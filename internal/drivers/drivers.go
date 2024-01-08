package drivers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"

	"github.com/astromechza/score-flyio/score"
)

// Driver is a definition of a driver that we can use to provide a resource
type Driver struct {
	// Type is the resource type as seen in the score schema resource list
	Type string `yaml:"type"`
	// Class is the class as seen in the score schema - this is required and is usually `default`
	Class string `yaml:"class"`
	// ResourceId is the optional resource id to match.
	ResourceId *string `yaml:"resource_id"`
	// Uri is where we can find the driver. We support multiple types:
	// - echo://driver-inputs (will copy the driver inputs to the output)
	// - file:///some/path/.drivers/thing (will call the given script file with a json payload)
	// - http+unix://%2Fsocket.sock/my-driver (will perform a post request with a json payload)
	// - http://localhost:3030/my-driver (will perform a post request with a json payload)
	Uri string `yaml:"uri"`
	// DriverInputs is additional inputs that can be passed to the driver
	DriverInputs map[string]interface{} `yaml:"driver_inputs"`
}

type DriverProvisionRequest struct {
	Type             string                 `json:"type"`
	Class            string                 `json:"class"`
	ResourceId       string                 `json:"resource_id"`
	DriverInputs     map[string]interface{} `json:"driver_inputs"`
	ResourceMetadata map[string]interface{} `json:"resource_metadata"`
	ResourceParams   map[string]interface{} `json:"resource_params"`
}

type DriverProvisionResponse struct {
	ResourceValues map[string]interface{} `json:"resource_values"`
}

func (d *Driver) Provision(ctx context.Context, resourceId string, resource score.Resource) (map[string]interface{}, error) {

	reqBody := DriverProvisionRequest{
		Type:             resource.Type,
		Class:            *resource.Class,
		ResourceId:       resourceId,
		DriverInputs:     d.DriverInputs,
		ResourceMetadata: resource.Metadata,
		ResourceParams:   resource.Params,
	}

	var err error
	parsedUri, _ := url.Parse(d.Uri)
	switch parsedUri.Scheme {
	case "echo":
		if parsedUri.Host == "driver-inputs" {
			if d.DriverInputs == nil {
				return map[string]interface{}{}, nil
			}
			return d.DriverInputs, nil
		}
		return nil, fmt.Errorf("unsupported echo host+path")
	case "file":
		parsedUri.Host, err = url.PathUnescape(parsedUri.Host)
		if err != nil {
			return nil, fmt.Errorf("driver file path '%s' could not be unescaped", parsedUri.Host+parsedUri.Path)
		}
		target, err := exec.LookPath(parsedUri.Host + parsedUri.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup target executable '%s': %w", parsedUri.Host+parsedUri.Path, err)
		}
		rawBody, _ := json.Marshal(reqBody)
		cmd := exec.CommandContext(ctx, target, string(rawBody))
		cmd.Stderr = os.Stderr
		buff := new(bytes.Buffer)
		cmd.Stdout = buff
		slog.Info("Executing driver", "uri", d.Uri)
		if err := cmd.Run(); err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				if ee.ExitCode() != 0 {
					return nil, fmt.Errorf("driver exec failed with exit code %d and output: %s", ee.ExitCode(), buff.String())
				}
			} else {
				return nil, fmt.Errorf("failed to wait for exec '%s': %w", target, err)
			}
		}
		var out DriverProvisionResponse
		if err := json.Unmarshal(buff.Bytes(), &out); err != nil {
			return nil, fmt.Errorf("failed to decode response from driver: %w", err)
		}
		return out.ResourceValues, nil
	default:
		return nil, fmt.Errorf("unsupported driver scheme: '%s'", parsedUri.Scheme)
	}
}
