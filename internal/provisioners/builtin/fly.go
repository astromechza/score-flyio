package builtin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type ListedApp struct {
	Name   string `json:"Name"`
	Status string `json:"Status"`
}

func flyBin() (string, error) {
	return exec.LookPath("fly")
}

func flyOrg() (string, error) {
	if v, ok := os.LookupEnv("FLY_ORG_NAME"); ok && v != "" {
		return v, nil
	}
	return "", fmt.Errorf("FLY_ORG_NAME not set")
}

func flyRegion() (string, error) {
	if v, ok := os.LookupEnv("FLY_REGION_NAME"); ok && v != "" {
		return v, nil
	}
	return "", fmt.Errorf("FLY_REGION_NAME not set")
}

func ListApps(flyOrg string, stderr io.Writer) ([]ListedApp, error) {
	fly, err := flyBin()
	if err != nil {
		return nil, fmt.Errorf("could not find fly/flyctl on $PATH: %w", err)
	}
	c := exec.Command(fly, "apps", "list", "--org", flyOrg, "--json")
	c.Env = os.Environ()
	c.Stderr = stderr
	buf := new(bytes.Buffer)
	c.Stdout = buf
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("process failed: %w", err)
	}
	var out []ListedApp
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("process succeeded but failed to decode output: %w", err)
	}
	return out, nil
}
