package convert

import (
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alessio/shellescape"

	"github.com/astromechza/score-flyio/flytoml"
	"github.com/astromechza/score-flyio/internal/templating"
	"github.com/astromechza/score-flyio/score"
)

const annotationNamespace = "score-flyio/"

func ConvertScoreToFlyConfig(appName string, region string, spec *score.WorkloadSpec) (*flytoml.Config, error) {
	output := &flytoml.Config{}

	output.AppName = appName
	output.PrimaryRegion = region
	if v, ok := spec.Metadata["name"].(string); ok && output.AppName == "" && v != "" {
		output.AppName = v
	}
	if r, ok := spec.Metadata[annotationNamespace+"primary_region"].(string); ok && output.PrimaryRegion == "" && r != "" {
		output.PrimaryRegion = r
	}

	templateCtx := templating.Context{
		Meta:               spec.Metadata,
		ResourceProperties: map[string]map[string]interface{}{},
	}
	for resourceName, resource := range spec.Resources {
		switch resource.Type {
		case "environment":
			if len(resource.Params) > 0 {
				return output, fmt.Errorf("resources: '%s': no params supported", resourceName)
			} else if resource.Class != nil && *resource.Class != "default" {
				return output, fmt.Errorf("resources: '%s': environment.'%s' class not supported", resourceName, *resource.Class)
			}
			// TODO: should we require this to come from an env file
			currentEnvironment := map[string]interface{}{}
			for _, s := range os.Environ() {
				parts := strings.SplitN(s, "=", 2)
				currentEnvironment[parts[0]] = parts[1]
			}
			templateCtx.ResourceProperties[resourceName] = currentEnvironment
		case "dns":
			if len(resource.Params) > 0 {
				return output, fmt.Errorf("resources: '%s': no params supported", resourceName)
			}
			if resource.Class == nil || *resource.Class == "default" {
				templateCtx.ResourceProperties[resourceName] = map[string]interface{}{
					"host": fmt.Sprintf("%s.internal", output.AppName),
				}
			} else if *resource.Class == "external" {
				templateCtx.ResourceProperties[resourceName] = map[string]interface{}{
					"host": fmt.Sprintf("%s.fly.dev", output.AppName),
				}
			} else {
				return output, fmt.Errorf("resources.%s: dns.'%s' class not supported", resourceName, *resource.Class)
			}
		case "volume":
			if len(resource.Params) > 0 {
				return output, fmt.Errorf("resources: '%s': no params supported", resourceName)
			} else if resource.Class != nil && *resource.Class != "default" {
				return output, fmt.Errorf("resources.%s: volume.'%s' class not supported", resourceName, *resource.Class)
			}
			volNameAnnotation := annotationNamespace + "volume_name"
			if annotations, ok := resource.Metadata["annotations"].(score.ResourceMetadata); ok {
				if volumeId, ok := annotations[volNameAnnotation].(string); ok {
					templateCtx.ResourceProperties[resourceName] = map[string]interface{}{"": volumeId}
					break
				}
			}
			return output, fmt.Errorf("resources.%s.metadata.annotations.%s should be the Fly.io volume id", resourceName, volNameAnnotation)
		case "":
			return output, fmt.Errorf("resources.%s.type: not specified", resourceName)
		default:
			return output, fmt.Errorf("resources.%s: unsupported resource type '%s'", resourceName, resource.Type)
		}
	}

	output.Processes = map[string]string{}
	output.Build = &flytoml.Build{}
	output.Env = make(map[string]string)

	for containerName, container := range spec.Containers {
		containerName := containerName
		container := container

		if output.Build.Image == "" {
			output.Build.Image = container.Image
		} else if output.Build.Image != container.Image {
			return nil, fmt.Errorf("containers.%s.image: all processes must use the same container image", containerName)
		}

		if container.Variables != nil {
			for k, v := range container.Variables {
				if v2, err := templateCtx.Substitute(v); err != nil {
					return output, fmt.Errorf("containers.%s.variables.%s: failed to interpolate: %w", containerName, k, err)
				} else if v, ok := output.Env[k]; ok && v != v2 {
					return output, fmt.Errorf("containers.%s.variables.%s: containers cannot have different values of the same variable", containerName, k)
				} else {
					output.Env[k] = v2
				}
			}
		}

		argString := make([]string, 0)
		argString = append(argString, container.Command...)
		argString = append(argString, container.Args...)
		output.Processes[containerName] = shellescape.QuoteCommand(argString)

		if container.Resources != nil {
			cpuReq := ""
			if container.Resources.Limits != nil && container.Resources.Limits.Cpu != nil {
				cpuReq = *container.Resources.Limits.Cpu
			} else if container.Resources.Requests != nil && container.Resources.Requests.Cpu != nil {
				cpuReq = *container.Resources.Requests.Cpu
			}
			memoryBytes := ""
			if container.Resources.Limits != nil && container.Resources.Limits.Memory != nil {
				memoryBytes = *container.Resources.Limits.Memory
			} else if container.Resources.Requests != nil && container.Resources.Requests.Memory != nil {
				memoryBytes = *container.Resources.Requests.Memory
			}
			if cpuReq != "" || memoryBytes != "" {
				vm := flytoml.Compute{CPUKind: "shared", Processes: []string{containerName}}
				if cpuReq != "" {
					if v, err := convertCpu(cpuReq); err != nil {
						return output, fmt.Errorf("invalid container cpu resource '%s': %w", cpuReq, err)
					} else {
						vm.CPUs = v
					}
				}
				if memoryBytes != "" {
					if v, err := convertMemoryToMegabytes(memoryBytes); err != nil {
						return output, fmt.Errorf("invalid container memory resource '%s': %w", memoryBytes, err)
					} else {
						vm.MemoryMB = v
					}
				}
				if output.Compute == nil {
					output.Compute = make([]*flytoml.Compute, 0)
				}
				output.Compute = append(output.Compute, &vm)
			}
		}

		if container.LivenessProbe != nil && container.LivenessProbe.HttpGet != nil {
			if output.Checks == nil {
				output.Checks = map[string]*flytoml.ToplevelCheck{}
			}
			checkName := fmt.Sprintf("%s-%s", containerName, "liveness")
			output.Checks[checkName] = ref(convertProbeToCheck(container.LivenessProbe.HttpGet))
			output.Checks[checkName].Processes = []string{containerName}
		}
		if container.ReadinessProbe != nil && container.ReadinessProbe.HttpGet != nil {
			if output.Checks == nil {
				output.Checks = map[string]*flytoml.ToplevelCheck{}
			}
			checkName := fmt.Sprintf("%s-%s", containerName, "readiness")
			output.Checks[checkName] = ref(convertProbeToCheck(container.ReadinessProbe.HttpGet))
			output.Checks[checkName].Processes = []string{containerName}
		}

		if len(container.Volumes) > 0 {
			if len(container.Volumes) > 1 {
				return output, fmt.Errorf("containers.%s.volumes: Fly.io only supports 1 volume per machine", containerName)
			}
			if output.Mounts == nil {
				output.Mounts = make([]flytoml.Mount, 0)
			}
			for i, volume := range container.Volumes {
				if volume.ReadOnly != nil && *volume.ReadOnly == true {
					return output, fmt.Errorf("containers.%s.volumes[%d]: read only not supported", containerName, i)
				}
				if volume.Path != nil && *volume.Path != "/" {
					return output, fmt.Errorf("containers.%s.volumes[%d]: subpath not supported", containerName, i)
				}
				volumeId, err := templateCtx.Substitute(volume.Source)
				if err != nil {
					return output, fmt.Errorf("containers.%s.volumes[%d].source: failed to substitue: %w", containerName, i, err)
				}
				output.Mounts = append(output.Mounts, flytoml.Mount{
					Source:      volumeId,
					Destination: volume.Target,
					Processes:   []string{containerName},
				})
			}
			if output.Mounts == nil {
				output.Mounts = make([]flytoml.Mount, 0)
			}
		}

		if len(container.Files) > 0 {
			if output.Files == nil {
				output.Files = make([]flytoml.File, 0)
			}
			for i, containerFile := range container.Files {
				if containerFile.Mode != nil {
					return output, fmt.Errorf("containers.%s.files[%d]]: mode is not supported", containerName, i)
				}
				var rawContent string
				if v, ok := containerFile.Content.(string); ok && v != "" {
					rawContent = v
				} else if containerFile.Source != nil {
					if rawData, err := os.ReadFile(*containerFile.Source); err != nil {
						return output, fmt.Errorf("containers.%s.files[%d]: failed to read source: %w", containerName, i, err)
					} else {
						rawContent = string(rawData)
					}
				} else {
					return output, fmt.Errorf("containers.%s.files[%d]: is missing source or content", containerName, i)
				}
				if containerFile.NoExpand == nil || !*containerFile.NoExpand {
					if substituted, err := templateCtx.Substitute(rawContent); err != nil {
						return output, fmt.Errorf("containers.%s.files[%d]: failed to substitute content: %w", containerName, i, err)
					} else {
						rawContent = substituted
					}
				}
				rawContent = base64.StdEncoding.EncodeToString([]byte(rawContent))
				output.Files = append(output.Files, flytoml.File{
					GuestPath: containerFile.Target,
					RawValue:  rawContent,
					Processes: []string{containerName},
				})
			}
		}
	}

	if spec.Service != nil && len(spec.Service.Ports) > 0 {
		flyServices := make([]flytoml.Service, 0)
		for portName, portSpec := range spec.Service.Ports {
			protocol := "tcp"
			if portSpec.Protocol != nil {
				protocol = *portSpec.Protocol
			}
			internalPort := portSpec.Port
			if internalPort == 0 && portSpec.TargetPort == nil {
				return output, fmt.Errorf("service: '%s' must have a port specified", portName)
			}
			targetPort := portSpec.Port
			if portSpec.TargetPort != nil && *portSpec.TargetPort > 0 {
				targetPort = *portSpec.TargetPort
				if internalPort == 0 {
					internalPort = targetPort
				}
			}
			var process string
			for p := range output.Processes {
				process = p
				break
			}
			flyServices = append(flyServices, flytoml.Service{
				Protocol:     protocol,
				InternalPort: portSpec.Port,
				Processes:    []string{process},
				Ports: []flytoml.MachinePort{
					{
						Port:     ref(portSpec.Port),
						Handlers: []string{},
					},
				},
			})
		}
		output.Services = flyServices
	}

	return output, nil
}

func convertCpu(input string) (int, error) {
	m := regexp.MustCompile(`^(\d+(?:[e.]\d+)?)(m?)$`).FindStringSubmatch(input)
	if m == nil {
		return 0, fmt.Errorf("does not match regex pattern")
	}
	value, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse '%s' as float", m[1])
	} else if math.IsNaN(value) || value <= 0 {
		return 0, fmt.Errorf("value was not positive")
	}
	if m[2] == "m" {
		value = value / 1000
	}
	value = math.Trunc(value*1000) / 1000
	if value != math.Round(value) {
		return 0, fmt.Errorf("Fly.io can only support integer numbers of cpus (%v != %v)", value, math.Round(value))
	}
	return int(value), nil
}

func convertMemoryToMegabytes(input string) (int, error) {
	m := regexp.MustCompile(`^(\d+(?:[e.]\d+)?)([A-Za-z]*)$`).FindStringSubmatch(input)
	if m == nil {
		return 0, fmt.Errorf("does not match regex pattern")
	}
	value, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse '%s' as float", m[1])
	} else if math.IsNaN(value) || value <= 0 {
		return 0, fmt.Errorf("value was not positive")
	}
	switch m[2] {
	case "":
	case "k":
		value = value * 1_000
	case "M":
		value = value * 1_000_000
	case "G":
		value = value * 1_000_000_000
	case "Ki":
		value = value * (1 << 10)
	case "Mi":
		value = value * (1 << 20)
	case "Gi":
		value = value * (1 << 30)
	default:
		return 0, fmt.Errorf("unsupported unit")
	}
	// We assume Fly is interested in Megabytes here, not Mebibytes
	value = math.Round(value / 1_000_000)
	if multiple := math.Round(value / 256); multiple == 0 || value != (256*multiple) {
		return 0, fmt.Errorf("Fly.io can only support multiples of 256 Megabytes of memory (got %vM)", value)
	}
	return int(value), nil
}

func convertProbeToCheck(hg *score.HttpProbe) flytoml.ToplevelCheck {
	probe := flytoml.ToplevelCheck{
		Type:        ref("http"),
		Port:        hg.Port,
		HTTPMethod:  ref("GET"),
		HTTPPath:    ref(hg.Path),
		GracePeriod: &flytoml.Duration{Duration: time.Second * 10},
		Interval:    &flytoml.Duration{Duration: time.Second * 2},
		Timeout:     &flytoml.Duration{Duration: time.Second * 4},
	}
	if hg.Scheme != nil {
		probe.HTTPProtocol = ref(string(*hg.Scheme))
	}
	if len(hg.HttpHeaders) > 0 {
		headers := make(map[string]string)
		for _, header := range hg.HttpHeaders {
			if header.Name != nil && header.Value != nil {
				headers[*header.Name] = *header.Value
			}
		}
		probe.HTTPHeaders = headers
	}
	return probe
}

func ref[k any](input k) *k {
	return &input
}
