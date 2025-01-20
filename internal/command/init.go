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

	"github.com/score-spec/score-go/framework"
	scoretypes "github.com/score-spec/score-go/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/astromechza/score-flyio/internal/state"
)

const (
	initCmdFileFlag      = "file"
	initCmdAppPrefixFlag = "fly-app-prefix"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialise the local state directory and sample score file",
	Args:  cobra.NoArgs,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		sd, ok, err := state.LoadStateDirectory(".")
		if err != nil {
			return fmt.Errorf("failed to load existing state directory: %w", err)
		} else if ok {
			slog.Info("Found existing state directory", "dir", sd.Path)
			pref, _ := cmd.Flags().GetString(initCmdAppPrefixFlag)
			if pref != "" && pref != sd.State.Extras.AppPrefix {
				return fmt.Errorf("--%s cannot be changed after first init ('%s' != '%s')", initCmdAppPrefixFlag, pref, sd.State.Extras.AppPrefix)
			}
		} else {
			if !cmd.Flags().Lookup(initCmdAppPrefixFlag).Changed {
				return fmt.Errorf("--%s must be set on first init", initCmdAppPrefixFlag)
			}
			pref, _ := cmd.Flags().GetString(initCmdAppPrefixFlag)
			sd = &state.StateDirectory{
				Path: state.DefaultRelativeStateDirectory,
				State: state.State{
					Extras:      state.StateExtras{AppPrefix: pref},
					Workloads:   map[string]framework.ScoreWorkloadState[state.WorkloadExtras]{},
					Resources:   map[framework.ResourceUid]framework.ScoreResourceState[state.ResourceExtras]{},
					SharedState: map[string]interface{}{state.SharedStateAppPrefixKey: pref},
				},
			}
			slog.Info("Writing new state directory", "dir", sd.Path)
			if err := sd.Persist(); err != nil {
				return fmt.Errorf("failed to persist new state directory: %w", err)
			}
		}

		initCmdScoreFile, _ := cmd.Flags().GetString(initCmdFileFlag)
		if initCmdScoreFile != "" {
			if _, err := os.Stat(initCmdScoreFile); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("failed to check for existing Score file: %w", err)
				}
				workload := &scoretypes.Workload{
					ApiVersion: "score.dev/v1b1",
					Metadata: map[string]interface{}{
						"name": "example",
					},
					Containers: map[string]scoretypes.Container{
						"main": {
							Image: "stefanprodan/podinfo",
						},
					},
					Service: &scoretypes.WorkloadService{
						Ports: map[string]scoretypes.ServicePort{
							"web": {Port: 8080},
						},
					},
				}
				rawScore, _ := yaml.Marshal(workload)
				if err := os.WriteFile(initCmdScoreFile, rawScore, 0755); err != nil {
					return fmt.Errorf("failed to write Score file: %w", err)
				}
				slog.Info("Created initial Score file", "file", initCmdScoreFile)
			} else {
				slog.Info("Skipping creation of initial Score file since it already exists", "file", initCmdScoreFile)
			}
		} else {
			slog.Info("Skipping creation of initial Score file since the file was set to an empty string", "file", initCmdScoreFile)
		}

		return nil
	},
}

func init() {
	initCmd.Flags().StringP(initCmdFileFlag, "f", "score.yaml", "The score file to initialize")
	initCmd.Flags().String(initCmdAppPrefixFlag, "", "A prefix to add to Workload names to determine final Fly.io app names")
	rootCmd.AddCommand(initCmd)
}
