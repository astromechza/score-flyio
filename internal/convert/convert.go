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
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
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
	mUnit := int64(256 * 1_000_000)
	memory = mUnit
	for _, rl := range []*scoretypes.ResourcesLimits{cr.Requests, cr.Limits} {
		if rl != nil {
			if c, m, err := scoretypes.ParseResourceLimits(*rl); err != nil {
				return cpus, memory, fmt.Errorf("failed to parse resource: %w", err)
			} else {
				if c != nil {
					cpus = max(cpus, int(math.Ceil(float64(*c)/1000)))
				}
				if m != nil {
					memory = max(memory, int64(float64(mUnit)*math.Ceil(float64(*m)/float64(mUnit))))
				}
			}
		}
	}
	return
}

const annotationPrefix = "score-flyio.astromechza.github.com/"

var annotationReg = regexp.MustCompile(`^service-([^-]+)-(handlers|auto-stop|min-running|http-options|concurrency)$`)

func Workload(currentState *state.State, workloadName string) (*appconfig.AppConfig, map[string]string, error) {
	resOutputs, err := currentState.GetResourceOutputForWorkload(workloadName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate outputs: %w", err)
	}
	sf := framework.BuildSubstitutionFunction(currentState.Workloads[workloadName].Spec.Metadata, resOutputs)

	workload := currentState.Workloads[workloadName]
	workloadAnnotations, _ := workload.Spec.Metadata["annotations"].(map[string]interface{})
	for s := range workloadAnnotations {
		if strings.HasPrefix(s, annotationPrefix) {
			if !annotationReg.MatchString(s[len(annotationPrefix):]) {
				return nil, nil, fmt.Errorf("unrecognised %s annotation: '%s'", annotationPrefix, s)
			}
		}
	}
	output := &appconfig.AppConfig{
		AppName: currentState.Extras.AppPrefix + workloadName,
		Build:   &appconfig.Build{},
	}

	if len(workload.Spec.Containers) != 1 {
		return nil, nil, fmt.Errorf("containers: only 1 container per workload is supported until Fly multi-container support is released")
	}
	containerName, container, _ := anyFromMap(workload.Spec.Containers)
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
				return nil, nil, fmt.Errorf("container[%s].variables: %s: %w", containerName, key, err)
			}
			output.Env[key] = out
		}
	}
	if len(container.Files) > 0 {
		output.Files = make([]appconfig.File, 0, len(container.Files))
		for i, f := range container.Files {
			if f.Mode != nil {
				return nil, nil, fmt.Errorf("container[%s].files[%d]: mode not supported", containerName, i)
			}
			if f.Source != nil {
				if !filepath.IsAbs(*f.Source) && workload.File != nil {
					if lp, err := filepath.Rel(filepath.Dir(*workload.File), *f.Source); err != nil {
						return nil, nil, fmt.Errorf("container[%s].files[%d]: failed to find relative path: %w", containerName, i, err)
					} else {
						f.Source = &lp
					}
				}
			}
			if f.NoExpand == nil || !*f.NoExpand {
				if f.Content != nil {
					out, err := framework.SubstituteString(*f.Content, sf)
					if err != nil {
						return nil, nil, fmt.Errorf("container[%s].files[%d]: failed to interpolate in contents: %w", containerName, i, err)
					}
					f.Content = &out
				} else if f.Source != nil {
					raw, err := os.ReadFile(*f.Source)
					if err != nil {
						return nil, nil, fmt.Errorf("container[%s].files[%d]: failed to read file: %w", containerName, i, err)
					} else if !utf8.Valid(raw) {
						return nil, nil, fmt.Errorf("container[%s].files[%d]: cannot perform interpolation on non utf-8 file (did you mean to set noExpand?)", containerName, i)
					}
					out, err := framework.SubstituteString(string(raw), sf)
					if err != nil {
						return nil, nil, fmt.Errorf("container[%s].files[%d]: failed to interpolate in source file: %w", containerName, i, err)
					}
					newRaw := []byte(out)
					if !bytes.Equal(raw, newRaw) {
						tf, err := os.CreateTemp("", "*")
						if err != nil {
							return nil, nil, fmt.Errorf("container[%s].files[%d]: failed to create tempfile: %w", containerName, i, err)
						}
						if _, err := tf.Write(newRaw); err != nil {
							return nil, nil, fmt.Errorf("container[%s].files[%d]: failed to write tempfile: %w", containerName, i, err)
						}
						if err := tf.Close(); err != nil {
							return nil, nil, fmt.Errorf("container[%s].files[%d]: failed to close tempfile: %w", containerName, i, err)
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
			return nil, nil, fmt.Errorf("container[%s].files[%d]: content or source must be set", containerName, i)
		}
	}

	if len(container.Volumes) > 0 {
		output.Mounts = make([]appconfig.Mount, 0, len(container.Volumes))
		for i, volume := range container.Volumes {
			if volume.Path != nil && *volume.Path != "/" {
				return nil, nil, fmt.Errorf("container[%s].volumes[%d]: sub-path is not supported", containerName, i)
			} else if volume.ReadOnly != nil && *volume.ReadOnly {
				return nil, nil, fmt.Errorf("container[%s].volumes[%d]: read-only=true is not supported", containerName, i)
			}
			source := volume.Source
			if source, err = framework.SubstituteString(source, sf); err != nil {
				return nil, nil, fmt.Errorf("container[%s].volumes[%d]: failed to interpolate source: %w", containerName, i, err)
			}
			output.Mounts = append(output.Mounts, appconfig.Mount{Source: source, Destination: volume.Target})
		}
	}

	if workload.Spec.Service != nil && len(workload.Spec.Service.Ports) > 0 {
		output.Services = make([]appconfig.Service, 0, len(workload.Spec.Service.Ports))
		for name, def := range workload.Spec.Service.Ports {
			svc := appconfig.Service{
				InternalPort: def.Port,
				Protocol:     "tcp",
			}
			prt := appconfig.ServicePort{
				Port: def.Port,
			}
			if def.TargetPort != nil {
				svc.InternalPort = *def.TargetPort
			}
			if def.Protocol != nil && *def.Protocol == scoretypes.ServicePortProtocolUDP {
				svc.Protocol = "udp"
			} else {
				if v, _ := workloadAnnotations[fmt.Sprintf("%sservice-%s-handlers", annotationPrefix, name)].(string); v != "" {
					prt.Handlers = strings.Split(v, ",")
				}
				if v, _ := workloadAnnotations[fmt.Sprintf("%sservice-%s-http-options", annotationPrefix, name)].(string); v != "" {
					httpOpts := make(map[string]interface{})
					if err := json.Unmarshal([]byte(v), &httpOpts); err != nil {
						return nil, nil, fmt.Errorf("services.ports[%s]: failed to unmarshal fly annotation: %w", name, err)
					}
					prt.HttpOptions = httpOpts
				}
			}
			svc.Ports = []appconfig.ServicePort{prt}

			if v, _ := workloadAnnotations[fmt.Sprintf("%sservice-%s-auto-stop", annotationPrefix, name)].(string); v != "" {
				svc.AutoStopMachines = v
			}
			if v, _ := workloadAnnotations[fmt.Sprintf("%sservice-%s-min-running", annotationPrefix, name)].(string); v != "" {
				if iv, err := strconv.Atoi(v); err != nil {
					return nil, nil, fmt.Errorf("services[%s]: failed to parse min running '%s' as int: %w", name, v, err)
				} else {
					svc.MinMachinesRunning = iv
				}
			}
			if v, _ := workloadAnnotations[fmt.Sprintf("%sservice-%s-concurrency", annotationPrefix, name)].(string); v != "" {
				concurrency := make(map[string]interface{})
				if err := json.Unmarshal([]byte(v), &concurrency); err != nil {
					return nil, nil, fmt.Errorf("services.ports[%s]: failed to unmarshal fly concurrency annotation: %w", name, err)
				}
				svc.Concurrency = concurrency
			}
			output.Services = append(output.Services, svc)
		}
	}

	if container.LivenessProbe != nil {
		output.Checks = map[string]appconfig.TopLevelCheck{"liveness_probe": probeToTopLevelCheck(*container.LivenessProbe)}
	}

	if container.ReadinessProbe != nil {
		hg := container.ReadinessProbe.HttpGet
		foundSvcIndex := slices.IndexFunc(output.Services, func(service appconfig.Service) bool {
			return service.InternalPort == hg.Port
		})
		if foundSvcIndex == -1 {
			if output.Checks == nil {
				output.Checks = make(map[string]appconfig.TopLevelCheck)
			}
			output.Checks["readiness_probe"] = probeToTopLevelCheck(*container.ReadinessProbe)
		} else {
			svc := output.Services[foundSvcIndex]
			svc.HttpChecks = []appconfig.HttpCheck{probeToHttpCheck(container.ReadinessProbe.HttpGet)}
			output.Services[foundSvcIndex] = svc
		}
	}
	return output, nil, nil
}

func probeToTopLevelCheck(probe scoretypes.ContainerProbe) appconfig.TopLevelCheck {
	check := appconfig.TopLevelCheck{
		Type:   "http",
		Port:   probe.HttpGet.Port,
		Method: "get",
		Path:   probe.HttpGet.Path,
	}
	if probe.HttpGet.HttpHeaders != nil {
		headers := make(map[string]string, len(probe.HttpGet.HttpHeaders))
		for _, header := range probe.HttpGet.HttpHeaders {
			headers[header.Name] = header.Value
		}
		check.Headers = headers
	}
	return check
}

func probeToHttpCheck(probe scoretypes.HttpProbe) appconfig.HttpCheck {
	check := appconfig.HttpCheck{
		Method: "get",
		Path:   probe.Path,
	}
	if probe.Scheme != nil {
		check.Protocol = strings.ToLower(string(*probe.Scheme))
		if check.Protocol == "https" {
			check.TlsSkipVerify = true
			if probe.Host != nil {
				check.TlsServerName = *(probe.Host)
			}
		}
	}
	if probe.HttpHeaders != nil {
		headers := make(map[string]string, len(probe.HttpHeaders))
		for _, header := range probe.HttpHeaders {
			headers[header.Name] = header.Value
		}
		check.Headers = headers
	}
	return check
}
