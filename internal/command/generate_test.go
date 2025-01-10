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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func changeToDir(t *testing.T, dir string) string {
	t.Helper()
	wd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	return dir
}

func changeToTempDir(t *testing.T) string {
	return changeToDir(t, t.TempDir())
}

func TestGenerateWithoutInit(t *testing.T) {
	_ = changeToTempDir(t)
	stdout, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"generate"})
	assert.EqualError(t, err, "state directory does not exist, please run \"init\" first")
	assert.Equal(t, "", stdout)
}

func TestGenerateWithoutScoreFiles(t *testing.T) {
	_ = changeToTempDir(t)
	stdout, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init", "--fly-app-prefix=example"})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)
	stdout, _, err = executeAndResetCommand(context.Background(), rootCmd, []string{"generate"})
	assert.EqualError(t, err, "project is empty, please add a score file")
	assert.Equal(t, "", stdout)
}

func TestInitAndGenerateWithBadFile(t *testing.T) {
	td := changeToTempDir(t)
	stdout, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init", "--fly-app-prefix=example"})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)

	assert.NoError(t, os.WriteFile(filepath.Join(td, "thing"), []byte(`"blah"`), 0644))

	stdout, _, err = executeAndResetCommand(context.Background(), rootCmd, []string{"generate", "thing"})
	assert.EqualError(t, err, "failed to decode input score file: thing: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `blah` into map[string]interface {}")
	assert.Equal(t, "", stdout)
}

func TestInitAndGenerateWithBadScore(t *testing.T) {
	td := changeToTempDir(t)
	stdout, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init", "--fly-app-prefix=example"})
	assert.NoError(t, err)
	assert.Equal(t, "", stdout)

	assert.NoError(t, os.WriteFile(filepath.Join(td, "thing"), []byte(`{}`), 0644))

	stdout, _, err = executeAndResetCommand(context.Background(), rootCmd, []string{"generate", "thing"})
	assert.EqualError(t, err, "invalid score file: thing: jsonschema: '' does not validate with https://score.dev/schemas/score#/required: missing properties: 'apiVersion', 'metadata', 'containers'")
	assert.Equal(t, "", stdout)
}

func TestSampleTests(t *testing.T) {
	ioTestsDir, err := filepath.Abs("../../samples")
	require.NoError(t, err)
	entries, err := os.ReadDir(ioTestsDir)
	require.NoError(t, err)
	for _, e := range entries {
		if e.IsDir() {
			t.Run(e.Name(), func(t *testing.T) {
				td := changeToTempDir(t)
				_, _, err := executeAndResetCommand(context.Background(), rootCmd, []string{"init", "--fly-app-prefix=iotest-", "--file="})
				require.NoError(t, err)
				_, _, err = executeAndResetCommand(context.Background(), rootCmd, []string{"generate", filepath.Join(ioTestsDir, e.Name(), "score.yaml")})
				require.NoError(t, err)
				expectedEntries, _ := os.ReadDir(filepath.Join(ioTestsDir, e.Name()))
				for _, ee := range expectedEntries {
					if !ee.IsDir() && strings.HasPrefix(ee.Name(), "fly_") && strings.HasSuffix(ee.Name(), ".toml") {
						expectedEntry, err := os.ReadFile(filepath.Join(ioTestsDir, e.Name(), ee.Name()))
						require.NoError(t, err)
						outputEntry, err := os.ReadFile(filepath.Join(td, ee.Name()))
						require.NoError(t, err)
						outputEntry = bytes.ReplaceAll(outputEntry, []byte(filepath.Join(ioTestsDir, e.Name())+"/"), []byte(""))
						assert.Equal(t, string(expectedEntry), string(outputEntry))
					}
				}
			})
		}
	}
}
