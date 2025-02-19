package flymachines

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/astromechza/score-flyio/internal"
)

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.3.0 --config=oapi-codegen.cfg.yaml spec.json

type FlyClient struct {
	ClientWithResponsesInterface
	ApiToken string
}

func NewFlyClient() (*FlyClient, error) {
	token, ok := os.LookupEnv("FLY_API_TOKEN")
	if !ok || token == "" {
		return nil, fmt.Errorf("FLY_API_TOKEN must be set")
	}
	token = strings.TrimPrefix(token, "FlyV1 ")
	c, err := NewClientWithResponses("https://api.machines.dev/v1", WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		slog.Debug("Making API request %s %s", req.Method, req.URL)
		return nil
	}))
	if err != nil {
		return nil, fmt.Errorf("Failed to setup client: %w", err)
	}
	return &FlyClient{ClientWithResponsesInterface: c, ApiToken: token}, nil
}

func GetApp(c ClientWithResponsesInterface, app string) (*App, bool, error) {
	resp, err := c.AppsShowWithResponse(context.Background(), app)
	if err != nil {
		return nil, false, fmt.Errorf("failed to make get-app request: %w", err)
	} else if resp.StatusCode() == http.StatusNotFound {
		return nil, false, nil
	} else if resp.JSON200 == nil {
		return nil, false, fmt.Errorf("failed to get-app: %s %s", resp.Status(), string(resp.Body))
	}
	return resp.JSON200, true, nil
}

func DeleteApp(c ClientWithResponsesInterface, app string) error {
	resp, err := c.AppsDeleteWithResponse(context.Background(), app)
	if err != nil {
		return fmt.Errorf("failed to make delete-app request: %w", err)
	} else if resp.StatusCode() != http.StatusAccepted {
		return fmt.Errorf("failed to delete-app: %s %s", resp.Status(), string(resp.Body))
	}
	return nil
}

func ListMachines(c ClientWithResponsesInterface, app string, state *string) ([]Machine, error) {
	resp, err := c.MachinesListWithResponse(context.Background(), app, &MachinesListParams{State: state})
	if err != nil {
		return nil, fmt.Errorf("failed to make list-machines request: %w", err)
	} else if resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list-machines: %s: %s", resp.Status(), string(resp.Body))
	}
	return *(resp.JSON200), nil
}

func ExecMachine(c ClientWithResponsesInterface, app, machine string, command []string) error {
	resp, err := c.MachinesExecWithResponse(context.Background(), app, machine, MachineExecRequest{
		Command: &command,
		Timeout: internal.Ref(60),
	})
	if err != nil {
		return fmt.Errorf("failed to make exec-machine request: %w", err)
	} else if resp.JSON200 == nil {
		return fmt.Errorf("failed to exec-machines: %s: %s", resp.Status(), string(resp.Body))
	}
	if *(resp.JSON200.ExitCode) != 0 {
		return fmt.Errorf("exec failed: code %d:\n%s\n%s", *(resp.JSON200.ExitCode), *(resp.JSON200.Stdout), *(resp.JSON200.Stderr))
	}
	log.Printf("stdout: %s", *(resp.JSON200.Stdout))
	return nil
}

func ExecAnyStartedMachine(c ClientWithResponsesInterface, app string, command []string) error {
	machines, err := ListMachines(c, app, internal.Ref("started"))
	if err != nil {
		return err
	} else if len(machines) == 0 {
		return fmt.Errorf("no machines to exec on")
	}
	return ExecMachine(c, app, *(machines[0].Id), command)
}
