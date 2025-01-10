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

package convert

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/score-spec/score-go/framework"
	scoretypes "github.com/score-spec/score-go/types"

	"github.com/astromechza/score-flyio/internal/appconfig"
	"github.com/astromechza/score-flyio/internal/state"
)

func anyFromMap[k string, v any](in map[k]v) (k, v, bool) {
	for kk, vv := range in {
		return kk, vv, true
	}
	var kk k
	var vv v
	return kk, vv, false
}

func collateVmResources(cr scoretypes.ContainerResources) (cpus int, memory int64, err error) {
	cpus = 1
	memory = int64(256 * 1_000_000)
	for _, rl := range []*scoretypes.ResourcesLimits{cr.Requests, cr.Limits} {
		if rl != nil {
			if c, m, err := scoretypes.ParseResourceLimits(*rl); err != nil {
				return cpus, memory, fmt.Errorf("failed to parse resource: %w", err)
			} else {
				if c != nil && *c > cpus {
					cpus = *c
				}
				if m != nil && *m > memory {
					memory = *m
				}
			}
		}
	}
	return
}

func Workload(currentState *state.State, workloadName string) (*appconfig.AppConfig, map[string]string, error) {
	resOutputs, err := currentState.GetResourceOutputForWorkload(workloadName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate outputs: %w", err)
	}
	sf := framework.BuildSubstitutionFunction(currentState.Workloads[workloadName].Spec.Metadata, resOutputs)

	workload := currentState.Workloads[workloadName]
	output := &appconfig.AppConfig{
		AppName: currentState.Extras.AppPrefix + workloadName,
		Build:   &appconfig.Build{},
	}

	if len(workload.Spec.Containers) != 1 {
		return nil, nil, fmt.Errorf("containers: only 1 container per workload is supported until Fly multi-container support is released")
	}
	_, container, _ := anyFromMap(workload.Spec.Containers)
	if container.Image == "." {
		if f := currentState.Workloads[workloadName].File; f != nil {
			output.Build.Dockerfile = filepath.Join(filepath.Dir(*f), "Dockerfile")
			output.Build.Dockerfile = filepath.Join(filepath.Dir(*f), ".dockerignore")
		}
	} else {
		output.Build.Image = container.Image
	}

	if len(container.Command) > 0 {
		if output.Experimental == nil {
			output.Experimental = &appconfig.Experimental{}
		}
		output.Experimental.Entrypoint = container.Command
	}
	if len(container.Args) > 0 {
		if output.Experimental == nil {
			output.Experimental = &appconfig.Experimental{}
		}
		output.Experimental.Cmd = container.Args
	}
	if container.Resources != nil {
		c, m, err := collateVmResources(*container.Resources)
		if err != nil {
			return nil, nil, fmt.Errorf("resources: %w", err)
		}
		output.Vm = &appconfig.Vm{Cpus: c, Memory: fmt.Sprintf("%dMB", int(m/1000000))}
	}

	if len(container.Variables) > 0 {
		output.Env = make(map[string]string, len(container.Variables))
		for key, value := range container.Variables {
			out, err := framework.SubstituteString(value, sf)
			if err != nil {
				return nil, nil, fmt.Errorf("variables: %s: %w", key, err)
			}
			output.Env[key] = out
		}
	}
	if len(container.Files) > 0 {
		output.Files = make([]appconfig.File, 0, len(container.Files))
		for i, f := range container.Files {
			if f.Mode != nil {
				return nil, nil, fmt.Errorf("files[%d]: mode not supported", i)
			}
			if f.Source != nil {
				if !filepath.IsAbs(*f.Source) && workload.File != nil {
					if lp, err := filepath.Rel(filepath.Dir(*workload.File), *f.Source); err != nil {
						return nil, nil, fmt.Errorf("files[%d]: failed to find relative path: %w", i, err)
					} else {
						f.Source = &lp
					}
				}
			}
			if f.NoExpand == nil || !*f.NoExpand {
				if f.Content != nil {
					out, err := framework.SubstituteString(*f.Content, sf)
					if err != nil {
						return nil, nil, fmt.Errorf("files[%d]: failed to interpolate in contents: %w", i, err)
					}
					f.Content = &out
				} else if f.Source != nil {
					raw, err := os.ReadFile(*f.Source)
					if err != nil {
						return nil, nil, fmt.Errorf("files[%d]: failed to read file: %w", i, err)
					} else if !utf8.Valid(raw) {
						return nil, nil, fmt.Errorf("files[%d]: cannot perform interpolation on non utf-8 file (did you mean to set noExpand?)", i)
					}
					out, err := framework.SubstituteString(string(raw), sf)
					if err != nil {
						return nil, nil, fmt.Errorf("files[%d]: failed to interpolate in source file: %w", i, err)
					}
					newRaw := []byte(out)
					if !bytes.Equal(raw, newRaw) {
						tf, err := os.CreateTemp("", "*")
						if err != nil {
							return nil, nil, fmt.Errorf("files[%d]: failed to create tempfile: %w", i, err)
						}
						if _, err := tf.Write(newRaw); err != nil {
							return nil, nil, fmt.Errorf("files[%d]: failed to write tempfile: %w", i, err)
						}
						if err := tf.Close(); err != nil {
							return nil, nil, fmt.Errorf("files[%d]: failed to close tempfile: %w", i, err)
						}
						n := tf.Name()
						f.Source = &n
					}
				}
			}
			if f.Content != nil {
				encoded := base64.StdEncoding.EncodeToString([]byte(*f.Content))
				output.Files = append(output.Files, appconfig.File{GuestPath: f.Target, RawValue: &encoded})
				continue
			} else if f.Source != nil {
				output.Files = append(output.Files, appconfig.File{GuestPath: f.Target, LocalPath: f.Source})
				continue
			}
			return nil, nil, fmt.Errorf("files[%d]: content or source must be set", i)
		}
	}

	if len(container.Volumes) > 0 {
		output.Mounts = make([]appconfig.Mount, 0, len(container.Volumes))
		for i, volume := range container.Volumes {
			if volume.Path != nil && *volume.Path != "/" {
				return nil, nil, fmt.Errorf("volumes[%d]: sub-path is not supported", i)
			} else if volume.ReadOnly != nil && *volume.ReadOnly {
				return nil, nil, fmt.Errorf("volumes[%d]: read-only=true is not supported", i)
			}
			source := volume.Source
			if source, err = framework.SubstituteString(source, sf); err != nil {
				return nil, nil, fmt.Errorf("volumes[%d]: failed to interpolate source: %w", i, err)
			}
			output.Mounts = append(output.Mounts, appconfig.Mount{Source: source, Destination: volume.Target})
		}
	}

	return output, nil, nil
}
