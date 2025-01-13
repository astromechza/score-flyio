// Copyright 2024 Humanitec
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provisioners

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/score-spec/score-go/framework"

	"github.com/astromechza/score-flyio/internal"
	"github.com/astromechza/score-flyio/internal/state"
)

func ProvisionResources(currentState *state.State) (*state.State, error) {
	out := currentState

	orphanedResources := make(map[framework.ResourceUid]bool, len(currentState.Resources))
	for uid, _ := range currentState.Resources {
		orphanedResources[uid] = true
	}

	// provision in sorted order
	orderedResources, err := currentState.GetSortedResourceUids()
	if err != nil {
		return nil, fmt.Errorf("failed to determine sort order for provisioning: %w", err)
	}

	out.Resources = maps.Clone(out.Resources)
ResourceLoop:
	for _, resUid := range orderedResources {
		resState := out.Resources[resUid]
		delete(orphanedResources, resUid)

		var params map[string]interface{}
		if len(resState.Params) > 0 {
			resOutputs, err := out.GetResourceOutputForWorkload(resState.SourceWorkload)
			if err != nil {
				return out, fmt.Errorf("%s: failed to find resource params for resource: %w", resUid, err)
			}
			sf := framework.BuildSubstitutionFunction(out.Workloads[resState.SourceWorkload].Spec.Metadata, resOutputs)
			rawParams, err := framework.Substitute(resState.Params, sf)
			if err != nil {
				return out, fmt.Errorf("%s: failed to substitute params for resource: %w", resUid, err)
			}
			params = rawParams.(map[string]interface{})
		}
		resState.Params = params

		for _, provisioner := range currentState.Extras.Provisioners {
			if !provisioner.Matches(resUid) {
				continue
			}

			inputs := ProvisionerInputs{
				ResourceUid:      resState.Guid,
				ResourceType:     resState.Type,
				ResourceClass:    resState.Class,
				ResourceId:       resState.Id,
				ResourceParams:   resState.Params,
				ResourceMetadata: resState.Metadata,
				ResourceState:    resState.State,
				SharedState:      currentState.SharedState,
			}

			var rawOutputs []byte
			if provisioner.Http != nil {
				rawOutputs, err = doHttpRequest(provisioner.Http, http.MethodPost, inputs)
			} else if provisioner.Cmd != nil {
				rawOutputs, err = doCmdRequest(provisioner.Cmd, "provision", inputs)
			} else if provisioner.Static != nil {
				rawOutputs, _ = json.Marshal(map[string]interface{}{
					"values": internal.Or(*provisioner.Static, map[string]interface{}{}),
				})
			} else {
				return out, fmt.Errorf("%s: provisioner is missing cmd or http section", resUid)
			}
			if err != nil {
				return out, fmt.Errorf("%s: failed to call provisioner: %w", resUid, err)
			}
			if len(rawOutputs) == 0 {
				return out, fmt.Errorf("provision request returned no output")
			}
			var outputs ProvisionerOutputs
			dec := json.NewDecoder(bytes.NewReader(rawOutputs))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&outputs); err != nil {
				slog.Debug("invalid provisioner outputs", slog.String("raw", string(rawOutputs)))
				return out, fmt.Errorf("%s: failed to decode response from provisioner: %w", resUid, err)
			}
			resState.ProvisionerUri = provisioner.ProvisionerId
			resState.State = internal.Or(outputs.ResourceState, resState.State, map[string]interface{}{})
			resState.Outputs = internal.Or(outputs.ResourceValues, resState.Outputs, map[string]interface{}{})

			if outputs.ResourceSecrets != nil {
				secretLookup := MapOutputLookupFunc(outputs.ResourceSecrets)
				secretLookupWithMarker := func(keys ...string) (interface{}, error) {
					v, err := secretLookup(keys...)
					if err == nil {
						MarkSecretAccessed()
					}
					return v, err
				}
				resState.OutputLookupFunc = OrOutputLookupFunc(secretLookupWithMarker, MapOutputLookupFunc(outputs.ResourceValues))
			}
			out.Resources[resUid] = resState
			out.SharedState = internal.PatchMap(out.SharedState, internal.Or(outputs.SharedState, make(map[string]interface{})))
			if err != nil {
				return out, fmt.Errorf("%s: failed to provision: %w", resUid, err)
			}
			slog.Info("Provisioned resource", slog.String("uid", string(resUid)))
			continue ResourceLoop
		}
		return out, fmt.Errorf("failed to find a provisioner for '%s.%s#%s'", resState.Type, resState.Class, resState.Id)
	}

	if len(orphanedResources) > 0 {
		slog.Warn("Some resources are no longer attached to a workload, consider de-provisioning them", slog.Int("count", len(orphanedResources)))
		for uid := range orphanedResources {
			rt := out.Resources[uid]
			rt.SourceWorkload = ""
			out.Resources[uid] = rt
		}
	}

	return out, nil
}

func DeProvisionResource(currentState *state.State, uid framework.ResourceUid) (*state.State, error) {
	out := currentState

	rs, ok := currentState.Resources[uid]
	if !ok {
		return nil, fmt.Errorf("no such resource exists")
	}

	pId := slices.IndexFunc(out.Extras.Provisioners, func(provisioner state.Provisioner) bool {
		return provisioner.ProvisionerId == rs.ProvisionerUri
	})
	if pId < 0 {
		return nil, fmt.Errorf("failed to find provisioner '%s' cannot deprovision resource", rs.ProvisionerUri)
	}
	provisioner := out.Extras.Provisioners[pId]

	inputs := ProvisionerInputs{
		ResourceUid:      rs.Guid,
		ResourceType:     rs.Type,
		ResourceClass:    rs.Class,
		ResourceId:       rs.Id,
		ResourceParams:   rs.Params,
		ResourceMetadata: rs.Metadata,
		ResourceState:    rs.State,
		SharedState:      currentState.SharedState,
	}

	var err error
	if provisioner.Http != nil {
		_, err = doHttpRequest(provisioner.Http, http.MethodPost, inputs)
	} else if provisioner.Cmd != nil {
		_, err = doCmdRequest(provisioner.Cmd, "provision", inputs)
	} else if provisioner.Static != nil {
		// do nothing
	} else {
		return out, fmt.Errorf("%s: provisioner is missing cmd or http section", uid)
	}
	if err != nil {
		return out, fmt.Errorf("%s: failed to call provisioner: %w", uid, err)
	}
	delete(out.Resources, uid)
	slog.Info("Successfully de-provisioned resource and removed resource state from state file", slog.String("uid", string(uid)))
	return out, nil
}

type ProvisionerInputs struct {
	ResourceUid   string                 `json:"resource_uid"`
	ResourceType  string                 `json:"resource_type"`
	ResourceClass string                 `json:"resource_class"`
	ResourceId    string                 `json:"resource_id"`
	ResourceState map[string]interface{} `json:"state"`
	SharedState   map[string]interface{} `json:"shared"`

	// only included for provision requests

	ResourceParams   map[string]interface{} `json:"resource_params"`
	ResourceMetadata map[string]interface{} `json:"resource_metadata"`
}

type ProvisionerOutputs struct {
	ResourceState   map[string]interface{} `json:"state,omitempty"`
	ResourceValues  map[string]interface{} `json:"values,omitempty"`
	ResourceSecrets map[string]interface{} `json:"secrets,omitempty"`
	SharedState     map[string]interface{} `json:"shared,omitempty"`
}

func doCmdRequest(c *state.CmdProvisioner, op string, inputs ProvisionerInputs) ([]byte, error) {
	bin := c.Binary
	if !strings.HasPrefix(bin, "/") {
		if b, err := exec.LookPath(bin); err != nil {
			return nil, fmt.Errorf("failed to find '%s' on path", bin)
		} else {
			bin = b
		}
	}
	rawInput, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("failed to encode json input: %w", err)
	}
	outputBuffer := new(bytes.Buffer)

	cmdArgs := slices.Clone(c.Args)
	for i, arg := range cmdArgs {
		arg = strings.ReplaceAll(arg, "$SCORE_PROVISIONER_MODE", op)
		cmdArgs[i] = arg
	}
	cmdArgs = append(cmdArgs, op)
	cmd := exec.CommandContext(context.Background(), bin, cmdArgs...)
	cmd.Env = append(cmd.Environ(), "SCORE_PROVISIONER_MODE="+op)
	cmd.Stdin = bytes.NewReader(rawInput)
	cmd.Stdout = outputBuffer
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to execute cmd provisioner: %w", err)
	}
	return outputBuffer.Bytes(), nil
}

func doHttpRequest(h *state.HttpProvisioner, method string, inputs ProvisionerInputs) ([]byte, error) {
	raw, _ := json.Marshal(inputs)
	req, err := http.NewRequest(method, h.Url, io.NopCloser(bytes.NewReader(raw)))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if method != http.MethodDelete {
		req.Header.Set("Accept", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer res.Body.Close()
	bod, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("http provision request failed with status: %d %s: '%s'", res.StatusCode, res.Status, string(bod))
	}
	return bod, nil
}

func MapOutputLookupFunc(s map[string]interface{}) framework.OutputLookupFunc {
	return func(keys ...string) (interface{}, error) {
		var resolvedValue interface{}
		resolvedValue = s
		for _, k := range keys {
			ok := resolvedValue != nil
			if ok {
				var mapV map[string]interface{}
				mapV, ok = resolvedValue.(map[string]interface{})
				if !ok {
					return "", fmt.Errorf("cannot lookup key '%s', context is not a map", k)
				}
				resolvedValue, ok = mapV[k]
			}
			if !ok {
				return "", fmt.Errorf("key '%s' not found", k)
			}
		}
		return resolvedValue, nil
	}
}

func OrOutputLookupFunc(a framework.OutputLookupFunc, b framework.OutputLookupFunc) framework.OutputLookupFunc {
	return func(keys ...string) (interface{}, error) {
		if v, err := a(keys...); err == nil {
			return v, err
		}
		return b(keys...)
	}
}
