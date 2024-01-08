package drivers

import (
	"fmt"
	"os"
	"strings"
)

func GenerateDefaultDrivers(appName string) ([]Driver, func(), error) {
	out := make([]Driver, 0)

	currentEnvironment := map[string]interface{}{}
	for _, s := range os.Environ() {
		parts := strings.SplitN(s, "=", 2)
		currentEnvironment[parts[0]] = parts[1]
	}
	out = append(out, Driver{
		Type:         "environment",
		Class:        "default",
		Uri:          "echo://driver-inputs",
		DriverInputs: currentEnvironment,
	})

	out = append(out, Driver{
		Type:  "dns",
		Class: "default",
		Uri:   "echo://driver-inputs",
		DriverInputs: map[string]interface{}{
			"host": fmt.Sprintf("%s.internal", appName),
		},
	})

	out = append(out, Driver{
		Type:  "dns",
		Class: "external",
		Uri:   "echo://driver-inputs",
		DriverInputs: map[string]interface{}{
			"host": fmt.Sprintf("%s.fly.dev", appName),
		},
	})

	return out, func() {
	}, nil
}
