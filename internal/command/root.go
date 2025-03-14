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
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/astromechza/score-flyio/internal/logging"
	"github.com/astromechza/score-flyio/internal/provisioners/builtin"
)

var (
	rootCmd = &cobra.Command{
		Use:           "score-flyio",
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			d, _ := cmd.Flags().GetBool("debug")
			d = d || os.Getenv("SCORE_FLYIO_DEBUG") == "true"
			logging.Set(d, cmd.ErrOrStderr())
			return nil
		},
	}
)

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		rootCmd.Version = info.Main.Version
	}
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Increase log verbosity to debug level")
	builtin.Install(rootCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
