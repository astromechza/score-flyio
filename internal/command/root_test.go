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
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

// executeAndResetCommand is a test helper that runs and then resets a command for executing in another test.
func executeAndResetCommand(ctx context.Context, cmd *cobra.Command, args []string) (string, string, error) {
	beforeOut, beforeErr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	defer func() {
		cmd.SetOut(beforeOut)
		cmd.SetErr(beforeErr)
		// also have to remove completion commands which get auto added and bound to an output buffer
		for _, command := range cmd.Commands() {
			if command.Name() == "completion" {
				cmd.RemoveCommand(command)
				break
			}
		}
	}()

	nowOut, nowErr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetOut(nowOut)
	cmd.SetErr(nowErr)
	cmd.SetArgs(args)
	subCmd, err := cmd.ExecuteContextC(ctx)
	if subCmd != nil {
		subCmd.SetOut(nil)
		subCmd.SetErr(nil)
		subCmd.SetContext(context.TODO())
		subCmd.SilenceUsage = false
		subCmd.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Value.Type() == "stringArray" {
				_ = f.Value.(pflag.SliceValue).Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
		})
	}
	return nowOut.String(), nowErr.String(), err
}

func TestRootUnknown(t *testing.T) {
	stdout, stderr, err := executeAndResetCommand(context.Background(), rootCmd, []string{"unknown"})
	assert.EqualError(t, err, "unknown command \"unknown\" for \"score-implementation-sample\"")
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}
