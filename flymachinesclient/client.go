package flymachinesclient

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
)

//go:generate go run github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@v2.0.0 --config=oapi-codegen.cfg.yml spec.yaml

const flyConfigFile = "${HOME}/.fly/config.yml"

func BuildScoreClient() (ClientWithResponsesInterface, error) {
	configContent, err := os.ReadFile(os.ExpandEnv(flyConfigFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read fly config file '%s': %w", flyConfigFile, err)
	}
	var temp map[string]interface{}
	if err := yaml.Unmarshal(configContent, &temp); err != nil {
		return nil, fmt.Errorf("failed to decode fly config file '%s': %w", flyConfigFile, err)
	}
	accessToken, ok := temp["access_token"].(string)
	if !ok || accessToken == "" {
		return nil, fmt.Errorf("fly config file is missing the 'access_token' string - please run fly auth login")
	}

	client, err := NewClientWithResponses("https://api.machines.dev/v1", func(client *Client) error {
		client.RequestEditors = append(client.RequestEditors, func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+accessToken)
			return nil
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build fly machines client: %w", err)
	}
	return client, nil
}
