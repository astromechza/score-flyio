package flygraphqlclient

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Khan/genqlient/graphql"

	"github.com/astromechza/score-flyio/fly"
)

//go:generate go run github.com/Khan/genqlient genqlient.yaml

type customDoer struct {
	client      *http.Client
	accessToken string
}

func (c *customDoer) Do(request *http.Request) (*http.Response, error) {
	request.Header.Set("Authorization", "Bearer "+c.accessToken)
	slog.Info(fmt.Sprintf("%s %s", request.Method, request.URL.Path))
	return c.client.Do(request)
}

func BuildGraphQlClient() (graphql.Client, error) {
	accessToken, err := fly.LoadAccessToken()
	if err != nil {
		return nil, err
	}
	return graphql.NewClient(`https://api.fly.io/graphql`, &customDoer{client: http.DefaultClient, accessToken: accessToken}), nil
}
