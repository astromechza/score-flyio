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

			outputs, err := provisioner.Provision(inputs)
			if outputs != nil {
				resState.State = internal.Or(outputs.ResourceState, resState.State, map[string]interface{}{})
				resState.Outputs = internal.Or(outputs.ResourceOutputs, resState.Outputs, map[string]interface{}{})
				out.Resources[resUid] = resState
				out.SharedState = internal.PatchMap(out.SharedState, internal.Or(outputs.SharedState, make(map[string]interface{})))
			}
			if err != nil {
				return out, fmt.Errorf("%s: failed to provision: %w", resUid, err)
			}
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
	ResourceState   map[string]interface{} `json:"state"`
	ResourceOutputs map[string]interface{} `json:"outputs"`
	SharedState     map[string]interface{} `json:"shared"`
}

var _ CanProvision = (*Provisioner)(nil)
var _ CanProvision = (*CmdProvisioner)(nil)
var _ CanProvision = (*HttpProvisioner)(nil)

func (p *Provisioner) Provision(inputs ProvisionerInputs) (*ProvisionerOutputs, error) {
	if p.Http != nil {
		return p.Http.Provision(inputs)
	} else if p.Cmd != nil {
		return p.Cmd.Provision(inputs)
	}
	return nil, fmt.Errorf("provisioner is missing cmd or http definition")
}

func (p *Provisioner) DeProvision(inputs ProvisionerInputs) error {
	if p.Http != nil {
		return p.Http.DeProvision(inputs)
	} else if p.Cmd != nil {
		return p.Cmd.DeProvision(inputs)
	}
	return fmt.Errorf("provisioner is missing cmd or http definition")
}

func (c *CmdProvisioner) doCmdRequest(op string, inputs ProvisionerInputs) ([]byte, error) {
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

func (c *CmdProvisioner) Provision(inputs ProvisionerInputs) (*ProvisionerOutputs, error) {
	bod, err := c.doCmdRequest("provision", inputs)
	if err != nil {
		return nil, err
	}
	if len(bod) == 0 {
		return nil, fmt.Errorf("http provision request returned no output")
	}
	var out ProvisionerOutputs
	dec := json.NewDecoder(bytes.NewReader(bod))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode response from http provisioner: %w", err)
	}
	return &out, nil
}

func (c *CmdProvisioner) DeProvision(inputs ProvisionerInputs) error {
	_, err := c.doCmdRequest("deprovision", inputs)
	return err
}

func (h *HttpProvisioner) doHttpRequest(method string, inputs ProvisionerInputs) ([]byte, error) {
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

func (h *HttpProvisioner) Provision(inputs ProvisionerInputs) (*ProvisionerOutputs, error) {
	bod, err := h.doHttpRequest(http.MethodPost, inputs)
	if err != nil {
		return nil, err
	}
	if len(bod) == 0 {
		return nil, fmt.Errorf("http provision request returned no output")
	}
	var out ProvisionerOutputs
	dec := json.NewDecoder(bytes.NewReader(bod))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode response from http provisioner: %w", err)
	}
	return &out, nil
}

func (h *HttpProvisioner) DeProvision(inputs ProvisionerInputs) error {
	_, err := h.doHttpRequest(http.MethodPost, inputs)
	return err
}
