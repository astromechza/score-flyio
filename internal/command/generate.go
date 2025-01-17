// Copyright 2024 Humanitec
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package command

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/imdario/mergo"
	"github.com/score-spec/score-go/framework"
	scoreloader "github.com/score-spec/score-go/loader"
	scoreschema "github.com/score-spec/score-go/schema"
	scoretypes "github.com/score-spec/score-go/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/astromechza/score-flyio/internal/convert"
	"github.com/astromechza/score-flyio/internal/provisioners"
	"github.com/astromechza/score-flyio/internal/state"
)

const (
	generateCmdOverridesFileFlag    = "overrides-file"
	generateCmdOverridePropertyFlag = "override-property"
	generateCmdImageFlag            = "image"
	generateCmdEnvSecretsFlag       = "secrets-file"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Run the conversion from score file to output manifests",
	Args:  cobra.ExactArgs(1),
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		sd, ok, err := state.LoadStateDirectory(".")
		if err != nil {
			return fmt.Errorf("failed to load existing state directory: %w", err)
		} else if !ok {
			return fmt.Errorf("state directory does not exist, please run \"init\" first")
		}
		currentState := &sd.State

		workloadFile := args[0]
		var rawWorkload map[string]interface{}
		if raw, err := os.ReadFile(args[0]); err != nil {
			return fmt.Errorf("failed to read input score file: %s: %w", workloadFile, err)
		} else if err = yaml.Unmarshal(raw, &rawWorkload); err != nil {
			return fmt.Errorf("failed to decode input score file: %s: %w", workloadFile, err)
		}

		// apply overrides

		if v, _ := cmd.Flags().GetString(generateCmdOverridesFileFlag); v != "" {
			if err := parseAndApplyOverrideFile(v, generateCmdOverridesFileFlag, rawWorkload); err != nil {
				return err
			}
		}

		// Now read, parse, and apply any override properties to the score files
		if v, _ := cmd.Flags().GetStringArray(generateCmdOverridePropertyFlag); len(v) > 0 {
			for _, overridePropertyEntry := range v {
				if rawWorkload, err = parseAndApplyOverrideProperty(overridePropertyEntry, generateCmdOverridePropertyFlag, rawWorkload); err != nil {
					return err
				}
			}
		}

		// Ensure transforms are applied (be a good citizen)
		if changes, err := scoreschema.ApplyCommonUpgradeTransforms(rawWorkload); err != nil {
			return fmt.Errorf("failed to upgrade spec: %w", err)
		} else if len(changes) > 0 {
			for _, change := range changes {
				slog.Info(fmt.Sprintf("Applying backwards compatible upgrade %s", change))
			}
		}

		var workload scoretypes.Workload
		if err = scoreschema.Validate(rawWorkload); err != nil {
			return fmt.Errorf("invalid score file: %s: %w", workloadFile, err)
		} else if err = scoreloader.MapSpec(&workload, rawWorkload); err != nil {
			return fmt.Errorf("failed to decode input score file: %s: %w", workloadFile, err)
		}
		workloadName := workload.Metadata["name"].(string)

		// Apply image override
		for containerName, container := range workload.Containers {
			if container.Image == "." {
				if v, _ := cmd.Flags().GetString(generateCmdImageFlag); v != "" {
					container.Image = v
					slog.Info(fmt.Sprintf("Set container image for container '%s' to %s from --%s", containerName, v, generateCmdImageFlag))
					workload.Containers[containerName] = container
				}
			}
		}

		if currentState, err = currentState.WithWorkload(&workload, &workloadFile, state.WorkloadExtras{}); err != nil {
			return fmt.Errorf("failed to add score file to project: %s: %w", workloadFile, err)
		}
		slog.Info("Added score file to project", "file", workloadFile)

		if currentState, err = currentState.WithPrimedResources(); err != nil {
			return fmt.Errorf("failed to prime resources: %w", err)
		}

		slog.Info("Primed resources", "#workloads", len(currentState.Workloads), "#resources", len(currentState.Resources))

		currentState, err = provisioners.ProvisionResources(currentState)
		if currentState != nil {
			sd.State = *currentState
			if persistErr := sd.Persist(); persistErr != nil {
				return fmt.Errorf("failed to persist state file: %w", errors.Join(persistErr, err))
			}
			slog.Info("Persisted state file")
		}
		if err != nil {
			return fmt.Errorf("failed to provision resources: %w", err)
		}

		if manifest, secrets, err := convert.Workload(currentState, workloadName); err != nil {
			return fmt.Errorf("failed to convert workloads: %w", err)
		} else {

			dest := fmt.Sprintf("fly_%s.toml", workloadName)
			f, err := os.CreateTemp("", "*")
			if err != nil {
				return fmt.Errorf("%s: failed to create tempfile: %w", workloadName, err)
			} else if err := toml.NewEncoder(f).Encode(manifest); err != nil {
				return fmt.Errorf("%s: failed to encode toml: %w", workloadName, err)
			} else if err := f.Close(); err != nil {
				return fmt.Errorf("%s: failed to close tempfile: %w", workloadName, err)
			} else if err := os.Rename(f.Name(), dest); err != nil {
				return fmt.Errorf("%s: failed to rename tempfile: %w", workloadName, err)
			}
			slog.Info(fmt.Sprintf("Wrote manifest to %s for workload '%s'", dest, workloadName))

			if x, _ := cmd.Flags().GetString(generateCmdEnvSecretsFlag); x == "" {
				if len(secrets) > 0 {
					return fmt.Errorf(
						"this workload uses application secrets at runtime, you must write these secrets to an output or stdout file using --secrets-file and then install them for app '%s'",
						currentState.Extras.AppPrefix+workloadName,
					)
				}
			} else if err := writeSecretsFile(secrets, x); err != nil {
				return fmt.Errorf("failed to write secrets env file: %w", err)
			} else {
				slog.Info(fmt.Sprintf("Wrote runtime secrets for workload '%s' to %s", workloadName, x))
			}
		}
		return nil
	},
}

func writeSecretsFile(s map[string]string, p string) error {
	content := new(strings.Builder)
	for s2, s3 := range s {
		if strings.Contains(s2, "=") || strings.Contains(s2, "\n") {
			return fmt.Errorf("key '%s' contains = or \\n", s2)
		}
		content.WriteString(s2)
		content.WriteRune('=')
		if strings.Contains(s3, "\n") {
			content.WriteString(`"""`)
			content.WriteString(s3)
			content.WriteString(`"""`)
		} else {
			content.WriteString(s3)
		}
		content.WriteRune('\n')
	}
	if p == "-" {
		_, _ = fmt.Fprint(os.Stdout, content.String())
		return nil
	} else {
		return os.WriteFile(p, []byte(content.String()), 0600)
	}
}

func parseAndApplyOverrideFile(entry string, flagName string, spec map[string]interface{}) error {
	if raw, err := os.ReadFile(entry); err != nil {
		return fmt.Errorf("--%s '%s' is invalid, failed to read file: %w", flagName, entry, err)
	} else {
		slog.Info(fmt.Sprintf("Applying overrides from %s to workload", entry))
		var out map[string]interface{}
		if err := yaml.Unmarshal(raw, &out); err != nil {
			return fmt.Errorf("--%s '%s' is invalid: failed to decode yaml: %w", flagName, entry, err)
		} else if err := mergo.Merge(&spec, out, mergo.WithOverride); err != nil {
			return fmt.Errorf("--%s '%s' failed to apply: %w", flagName, entry, err)
		}
	}
	return nil
}

func parseAndApplyOverrideProperty(entry string, flagName string, spec map[string]interface{}) (map[string]interface{}, error) {
	parts := strings.SplitN(entry, "=", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("--%s '%s' is invalid, expected a =-separated path and value", flagName, entry)
	}
	if parts[1] == "" {
		slog.Info(fmt.Sprintf("Overriding '%s' in workload", parts[0]))
		after, err := framework.OverridePathInMap(spec, framework.ParseDotPathParts(parts[0]), true, nil)
		if err != nil {
			return nil, fmt.Errorf("--%s '%s' could not be applied: %w", flagName, entry, err)
		}
		return after, nil
	} else {
		var value interface{}
		if err := yaml.Unmarshal([]byte(parts[1]), &value); err != nil {
			return nil, fmt.Errorf("--%s '%s' is invalid, failed to unmarshal value as json: %w", flagName, entry, err)
		}
		slog.Info(fmt.Sprintf("Overriding '%s' in workload", parts[0]))
		after, err := framework.OverridePathInMap(spec, framework.ParseDotPathParts(parts[0]), false, value)
		if err != nil {
			return nil, fmt.Errorf("--%s '%s' could not be applied: %w", flagName, entry, err)
		}
		return after, nil
	}
}

func init() {
	generateCmd.Flags().String(generateCmdOverridesFileFlag, "", "An optional file of Score overrides to merge in")
	generateCmd.Flags().StringArray(generateCmdOverridePropertyFlag, []string{}, "An optional set of path=key overrides to set or remove")
	generateCmd.Flags().String(generateCmdImageFlag, "", "An optional container image to use for any container with image == '.'")
	generateCmd.Flags().String(generateCmdEnvSecretsFlag, "", "An optional output file for the runtime secrets in KEY=VALUE format")
	rootCmd.AddCommand(generateCmd)
}
