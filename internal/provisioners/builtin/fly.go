package builtin

import (
	"fmt"
	"os"

	"github.com/astromechza/score-flyio/internal/state"
)

func flyRegion() (string, error) {
	if v, ok := os.LookupEnv("FLY_REGION_NAME"); ok && v != "" {
		return v, nil
	}
	return "", fmt.Errorf("FLY_REGION_NAME not set")
}

// FlyAppPrefixFromState extracts the app prefix from the shared state which should have been inserted at score-flyio init time.
func FlyAppPrefixFromState(sharedState map[string]interface{}) string {
	v, _ := sharedState[state.SharedStateAppPrefixKey].(string)
	return v
}
