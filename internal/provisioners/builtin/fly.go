package builtin

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/astromechza/score-flyio/internal"
	"github.com/astromechza/score-flyio/internal/flymachines"
)

type FlyClient struct {
	flymachines.ClientWithResponsesInterface
	ApiToken string
}

func NewFlyClient() (*FlyClient, error) {
	token, ok := os.LookupEnv("FLY_API_TOKEN")
	if !ok || token == "" {
		return nil, fmt.Errorf("FLY_API_TOKEN must be set")
	}
	c, err := flymachines.NewClientWithResponses("https://api.machines.dev/v1", flymachines.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		log.Printf("Making API request %s %s", req.Method, req.URL)
		return nil
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to setup client: %w", err)
	}
	return &FlyClient{ClientWithResponsesInterface: c, ApiToken: token}, nil
}

type ListedApp struct {
	Name   string `json:"Name"`
	Status string `json:"Status"`
}

func flyRegion() (string, error) {
	if v, ok := os.LookupEnv("FLY_REGION_NAME"); ok && v != "" {
		return v, nil
	}
	return "", fmt.Errorf("FLY_REGION_NAME not set")
}

func GetApp(c flymachines.ClientWithResponsesInterface, app string) (*flymachines.App, bool, error) {
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

func DeleteApp(c flymachines.ClientWithResponsesInterface, app string) error {
	resp, err := c.AppsDeleteWithResponse(context.Background(), app)
	if err != nil {
		return fmt.Errorf("failed to make delete-app request: %w", err)
	} else if resp.StatusCode() != http.StatusAccepted {
		return fmt.Errorf("failed to delete-app: %s %s", resp.Status(), string(resp.Body))
	}
	return nil
}

func ListMachines(c flymachines.ClientWithResponsesInterface, app string, state *string) ([]flymachines.Machine, error) {
	resp, err := c.MachinesListWithResponse(context.Background(), app, &flymachines.MachinesListParams{State: state})
	if err != nil {
		return nil, fmt.Errorf("failed to make list-machines request: %w", err)
	} else if resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list-machines: %s: %s", resp.Status(), string(resp.Body))
	}
	return *(resp.JSON200), nil
}

func ExecMachine(c flymachines.ClientWithResponsesInterface, app, machine string, command []string) error {
	resp, err := c.MachinesExecWithResponse(context.Background(), app, machine, flymachines.MachineExecRequest{
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

func ExecAnyStartedMachine(c flymachines.ClientWithResponsesInterface, app string, command []string) error {
	machines, err := ListMachines(c, app, internal.Ref("started"))
	if err != nil {
		return err
	} else if len(machines) == 0 {
		return fmt.Errorf("no machines to exec on")
	}
	return ExecMachine(c, app, *(machines[0].Id), command)
}
