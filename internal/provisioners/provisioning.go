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

	// provision in sorted order
	orderedResources, err := currentState.GetSortedResourceUids()
	if err != nil {
		return nil, fmt.Errorf("failed to determine sort order for provisioning: %w", err)
	}

	out.Resources = maps.Clone(out.Resources)
ResourceLoop:
	for _, resUid := range orderedResources {
		resState := out.Resources[resUid]

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
			if provisioner.ResourceType != resState.Type ||
				(provisioner.ResourceClass != "" && provisioner.ResourceClass != resState.Class) ||
				(provisioner.ResourceId != "" && provisioner.ResourceId != resState.Id) {
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
					"outputs": internal.Or(*provisioner.Static, map[string]interface{}{}),
				})
			} else {
				return out, fmt.Errorf("%s: provisioner is missing cmd or http section", resUid)
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
			resState.State = internal.Or(outputs.ResourceState, resState.State, map[string]interface{}{})
			resState.Outputs = internal.Or(outputs.ResourceOutputs, resState.Outputs, map[string]interface{}{})
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
	return out, nil
}

type ProvisionerInputs struct {
	ResourceUid      string                 `json:"resource_uid"`
	ResourceType     string                 `json:"resource_type"`
	ResourceClass    string                 `json:"resource_class"`
	ResourceId       string                 `json:"resource_id"`
	ResourceParams   map[string]interface{} `json:"resource_params"`
	ResourceMetadata map[string]interface{} `json:"resource_metadata"`
	ResourceState    map[string]interface{} `json:"state"`
	SharedState      map[string]interface{} `json:"shared"`
}

type ProvisionerOutputs struct {
	ResourceState   map[string]interface{} `json:"state,omitempty"`
	ResourceOutputs map[string]interface{} `json:"outputs,omitempty"`
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
	cmdArgs = append(cmdArgs, op)
	cmd := exec.CommandContext(context.Background(), bin, cmdArgs...)
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
