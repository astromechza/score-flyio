package runcmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/sjson"
	"gopkg.in/yaml.v3"

	"github.com/astromechza/score-flyio/flygraphqlclient"
	"github.com/astromechza/score-flyio/flymachinesclient"
	"github.com/astromechza/score-flyio/score"
)

func Run(args Args) error {
	slog.Debug("Running deploy subcommand", "args", args)
	slog.Info("Validating Score input..")
	scoreSpec, err := score.ParseAndValidate(args.ScoreFileContent)
	if err != nil {
		return fmt.Errorf("score spec was not valid: %w", err)
	}
	slog.Info("Score input is valid.")

	// construct the clients first here
	machinesClient, err := flymachinesclient.BuildScoreClient()
	if err != nil {
		return err
	}
	graphClient, err := flygraphqlclient.BuildGraphQlClient()
	if err != nil {
		return err
	}

	// now load the app and extras information

	slog.Info(fmt.Sprintf("Looking up app %s/%s..", args.Org, args.App))
	var existingApp *flymachinesclient.App
	if getAppResp, err := machinesClient.AppsShowWithResponse(context.Background(), args.App); err != nil {
		return fmt.Errorf("failed to make show app request: %w", err)
	} else {
		switch getAppResp.StatusCode() {
		case http.StatusOK:
			existingApp = getAppResp.JSON200
		case http.StatusNotFound:
			if strings.Contains(string(getAppResp.Body), "Could not find App") {
				return fmt.Errorf("the Fly app '%s' does not exist - please create it", args.App)
			}
			fallthrough
		default:
			return fmt.Errorf("unexpected status code when showing app '%d': '%s'", getAppResp.StatusCode(), string(getAppResp.Body))
		}
	}

	slog.Info(fmt.Sprintf("Looking up hostname and shared ips"))
	appExtras, err := flygraphqlclient.GetAppExtras(context.Background(), graphClient, args.App)
	if err != nil {
		return fmt.Errorf("failed to load app extras: %w", err)
	}
	slog.Debug("App extras", "extras", appExtras)

	asMachine, err := convertScoreIntoMachine(args.App, scoreSpec, appExtras.App)
	if err != nil {
		return fmt.Errorf("failed to convert score to Fly machine: %w", err)
	}

	// in order to apply sjson modifications, we need to coerce through json
	rawMachineJson, _ := json.Marshal(asMachine)
	machineJson := string(rawMachineJson)

	if len(args.Extensions) > 0 {
		slog.Info(fmt.Sprintf("Applying %d extensions..", len(args.Extensions)))
		for _, extension := range args.Extensions {
			machineJson, err = sjson.Set(machineJson, extension.Path, extension.Set)
		}
		asMachine = flymachinesclient.ApiMachineConfig{}
		if err := json.Unmarshal([]byte(machineJson), &asMachine); err != nil {
			return fmt.Errorf("failed to convert machine json with extensions back to machine: %w", err)
		}
	}

	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		slog.Debug(machineJson)
	}

	if args.App == "" {
		args.App = scoreSpec.Metadata["name"].(string)
	}
	if args.Org == "" {
		return fmt.Errorf("no Fly.io org specified, please set --org")
	}
	if args.App == "" {
		return fmt.Errorf("no Fly.io app specified, please set --app or set a metadata.name in the Score specification")
	}
	slog.Info(fmt.Sprintf("This deployment applies to App '%s' in Organization '%s'..", args.Org, args.App))

	if args.DryRun {
		slog.Info("Stopping here and dumping yaml because --dry-run was provided.")
		var temp interface{}
		if err := json.Unmarshal([]byte(machineJson), &temp); err != nil {
			return fmt.Errorf("failed to unmarshal: %w", err)
		}
		_ = yaml.NewEncoder(os.Stdout).Encode(&temp)
		return nil
	}

	var existingMachines []flymachinesclient.Machine
	if existingApp != nil {
		slog.Info(fmt.Sprintf("Looking up machines for app %s/%s..", args.Org, args.App))
		if listMachinesResp, err := machinesClient.MachinesListWithResponse(context.Background(), args.App, &flymachinesclient.MachinesListParams{}); err != nil {
			return fmt.Errorf("failed to make list machines request: %w", err)
		} else {
			switch listMachinesResp.StatusCode() {
			case http.StatusOK:
				existingMachines = *listMachinesResp.JSON200
			default:
				return fmt.Errorf("unexpected status code when listing machines '%d': '%s'", listMachinesResp.StatusCode(), string(listMachinesResp.Body))
			}
		}
		slog.Info(fmt.Sprintf("Found %d machines.", len(existingMachines)))
	}

	if len(existingMachines) > 0 {
		slog.Info(fmt.Sprintf("Plan: update %d existing machines in the app", len(existingMachines)))
	} else {
		slog.Info(fmt.Sprintf("Plan: create 1 new machine in the app"))
	}

	if len(existingMachines) > 0 {
		for _, machine := range existingMachines {
			slog.Info(fmt.Sprintf("Updating machine '%s'..", *machine.Name))
			if updateMachineResp, err := machinesClient.MachinesUpdateWithResponse(context.Background(), args.App, *machine.Id, flymachinesclient.UpdateMachineRequest{
				Config:     ref(asMachine),
				SkipLaunch: ref(false),
			}); err != nil {
				return fmt.Errorf("failed to make update machine request: %w", err)
			} else {
				if updateMachineResp.StatusCode() >= 300 {
					return fmt.Errorf("unexpected status code when updating machine '%d': '%s'", updateMachineResp.StatusCode(), string(updateMachineResp.Body))
				}
			}
		}
	} else {
		slog.Info(fmt.Sprintf("Creating machine.."))
		if machineCreateResp, err := machinesClient.MachinesCreateWithResponse(context.Background(), args.App, flymachinesclient.CreateMachineRequest{
			Config:     ref(asMachine),
			SkipLaunch: ref(false),
		}); err != nil {
			return fmt.Errorf("failed to make create machine request: %w", err)
		} else {
			if machineCreateResp.StatusCode() != http.StatusOK {
				return fmt.Errorf("unexpected status code when creating machine '%d': '%s'", machineCreateResp.StatusCode(), string(machineCreateResp.Body))
			}
			slog.Info(fmt.Sprintf("Created machine '%s'", *(machineCreateResp.JSON200.Name)))
		}
	}

	return nil
}

func ref[k any](input k) *k {
	return &input
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

func convertScoreIntoMachine(appName string, spec *score.WorkloadSpec, appExtras flygraphqlclient.GetAppExtrasApp) (flymachinesclient.ApiMachineConfig, error) {
	templating := templatesContext{
		meta:               spec.Metadata,
		resourceProperties: map[string]map[string]interface{}{},
	}

	output := flymachinesclient.ApiMachineConfig{}

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
			templating.resourceProperties[resourceName] = currentEnvironment
		case "dns":
			if len(resource.Params) > 0 {
				return output, fmt.Errorf("resources: '%s': no params supported", resourceName)
			}
			if resource.Class == nil || *resource.Class == "default" {
				templating.resourceProperties[resourceName] = map[string]interface{}{
					"host": fmt.Sprintf("%s.internal", appName),
				}
			} else if *resource.Class == "external" {
				templating.resourceProperties[resourceName] = map[string]interface{}{
					"host": appExtras.Hostname,
				}
			} else {
				return output, fmt.Errorf("resources: '%s': dns.'%s' class not supported", resourceName, *resource.Class)
			}
		case "volume":
			if len(resource.Params) > 0 {
				return output, fmt.Errorf("resources: '%s': no params supported", resourceName)
			} else if resource.Class != nil && *resource.Class != "default" {
				return output, fmt.Errorf("resources: '%s': volume.'%s' class not supported", resourceName, *resource.Class)
			}
			if annotations, ok := resource.Metadata["annotations"].(score.ResourceMetadata); ok {
				if volumeId, ok := annotations["score-flyio/volume_id"].(string); ok {
					templating.resourceProperties[resourceName] = map[string]interface{}{"": volumeId}
					break
				}
			}
			return output, fmt.Errorf("resources: '%s': metadata.annotations.score-flyio/volume_id should be the Fly.io volume id", resourceName)
		case "":
			return output, fmt.Errorf("resources: '%s': missing resource type", resourceName)
		default:
			return output, fmt.Errorf("resources: '%s': unsupported resource type '%s'", resourceName, resource.Type)
		}
	}

	if len(spec.Containers) != 1 {
		return output, fmt.Errorf("score spec contains more than 1 container")
	}
	var containerName string
	var container score.Container
	for containerName, container = range spec.Containers {
		break
	}
	output.Image = ref(container.Image)
	process := flymachinesclient.ApiMachineProcess{}
	if len(container.Command) > 0 {
		process.Entrypoint = ref(container.Command)
	}
	if len(container.Args) > 0 {
		process.Cmd = ref(container.Args)
	}
	if container.Variables != nil {
		outputEnv := make(map[string]string, len(container.Variables))
		for k, v := range container.Variables {
			if v2, err := templating.Substitute(v); err != nil {
				return output, fmt.Errorf("containers.%s.variables.%s: failed to interpolate: %w", containerName, k, err)
			} else {
				outputEnv[k] = v2
			}
		}
		process.Env = ref(outputEnv)
	}
	if process.Cmd != nil || process.Entrypoint != nil || process.Env != nil {
		output.Processes = &[]flymachinesclient.ApiMachineProcess{process}
	}
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
			output.Guest = &flymachinesclient.ApiMachineGuest{CpuKind: ref("shared")}
		}
		if cpuReq != "" {
			if v, err := convertCpu(cpuReq); err != nil {
				return output, fmt.Errorf("invalid container cpu resource '%s': %w", cpuReq, err)
			} else {
				output.Guest.Cpus = ref(v)
			}
		}
		if memoryBytes != "" {
			if v, err := convertMemoryToMegabytes(memoryBytes); err != nil {
				return output, fmt.Errorf("invalid container memory resource '%s': %w", memoryBytes, err)
			} else {
				output.Guest.MemoryMb = ref(v)
			}
		}
	}
	if container.LivenessProbe != nil && container.LivenessProbe.HttpGet != nil {
		if output.Checks == nil {
			output.Checks = &map[string]flymachinesclient.ApiMachineCheck{}
		}
		(*output.Checks)["liveness"] = convertProbeToCheck(container.LivenessProbe.HttpGet)
	}
	if container.ReadinessProbe != nil && container.ReadinessProbe.HttpGet != nil {
		if output.Checks == nil {
			output.Checks = &map[string]flymachinesclient.ApiMachineCheck{}
		}
		(*output.Checks)["readiness"] = convertProbeToCheck(container.ReadinessProbe.HttpGet)
	}

	if spec.Service != nil && len(spec.Service.Ports) > 0 {
		flyServices := make([]flymachinesclient.ApiMachineService, 0)
		for portName, portSpec := range spec.Service.Ports {
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
			flyServices = append(flyServices, flymachinesclient.ApiMachineService{
				Protocol:     portSpec.Protocol,
				InternalPort: ref(portSpec.Port),
				Ports: &[]flymachinesclient.ApiMachinePort{
					{
						Port:     ref(portSpec.Port),
						Handlers: &[]string{},
					},
				},
			})
		}
		output.Services = &flyServices
	}

	if len(container.Files) > 0 {
		outputFiles := make([]flymachinesclient.ApiFile, 0)
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
				if substituted, err := templating.Substitute(rawContent); err != nil {
					return output, fmt.Errorf("containers.%s.files[%d]: failed to substitute content: %w", containerName, i, err)
				} else {
					rawContent = substituted
				}
			}
			rawContent = base64.StdEncoding.EncodeToString([]byte(rawContent))
			outputFiles = append(outputFiles, flymachinesclient.ApiFile{
				GuestPath: ref(containerFile.Target),
				RawValue:  &rawContent,
			})
		}
		output.Files = &outputFiles
	}

	if len(container.Volumes) > 0 {
		if len(container.Volumes) > 1 {
			return output, fmt.Errorf("containers.%s.volumes: Fly.io only supports 1 volume per machine", containerName)
		}
		mounts := make([]flymachinesclient.ApiMachineMount, 0)
		for i, volume := range container.Volumes {
			if volume.ReadOnly != nil && *volume.ReadOnly == true {
				return output, fmt.Errorf("containers.%s.volumes[%d]: read only not supported", containerName, i)
			}
			if volume.Path != nil && *volume.Path != "/" {
				return output, fmt.Errorf("containers.%s.volumes[%d]: subpath not supported", containerName, i)
			}
			volumeId, err := templating.Substitute(volume.Source)
			if err != nil {
				return output, fmt.Errorf("containers.%s.volumes[%d].source: failed to substitue: %w", containerName, i, err)
			}
			mounts = append(mounts, flymachinesclient.ApiMachineMount{
				Volume: ref(volumeId),
				Path:   ref(volume.Target),
			})
		}
		output.Mounts = &mounts
	}

	return output, nil
}

func convertProbeToCheck(hg *score.HttpProbe) flymachinesclient.ApiMachineCheck {
	probe := flymachinesclient.ApiMachineCheck{
		Type:        ref("http"),
		Port:        hg.Port,
		Method:      ref("GET"),
		Path:        ref(hg.Path),
		GracePeriod: ref("10s"),
		Interval:    ref("15s"),
		Timeout:     ref("3s"),
	}
	if hg.Scheme != nil {
		probe.Protocol = ref(string(*hg.Scheme))
	}
	if len(hg.HttpHeaders) > 0 {
		headers := make([]flymachinesclient.ApiMachineHTTPHeader, 0, len(hg.HttpHeaders))
		for _, header := range hg.HttpHeaders {
			if header.Name != nil && header.Value != nil {
				headers = append(headers, flymachinesclient.ApiMachineHTTPHeader{Name: header.Name, Values: ref([]string{*header.Value})})
			}
		}
		probe.Headers = &headers
	}
	return probe
}
