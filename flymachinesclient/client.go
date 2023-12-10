package flymachinesclient

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/astromechza/score-flyio/fly"
)

//go:generate go run github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen --config=oapi-codegen.cfg.yml spec.yaml

func BuildScoreClient() (ClientWithResponsesInterface, error) {
	accessToken, err := fly.LoadAccessToken()
	if err != nil {
		return nil, err
	}
	client, err := NewClientWithResponses("https://api.machines.dev/v1", func(client *Client) error {
		client.RequestEditors = append(client.RequestEditors, func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+accessToken)
			slog.Info(fmt.Sprintf("%s %s", req.Method, req.URL.Path))
			return nil
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build fly machines client: %w", err)
	}
	return client, nil
}
