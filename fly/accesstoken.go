package fly

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const flyConfigFile = "${HOME}/.fly/config.yml"

func LoadAccessToken() (string, error) {
	configContent, err := os.ReadFile(os.ExpandEnv(flyConfigFile))
	if err != nil {
		return "", fmt.Errorf("failed to read fly config file '%s': %w", flyConfigFile, err)
	}
	var temp map[string]interface{}
	if err := yaml.Unmarshal(configContent, &temp); err != nil {
		return "", fmt.Errorf("failed to decode fly config file '%s': %w", flyConfigFile, err)
	}
	accessToken, ok := temp["access_token"].(string)
	if !ok || accessToken == "" {
		return "", fmt.Errorf("fly config file is missing the 'access_token' string - please run fly auth login")
	}
	return accessToken, nil
}
