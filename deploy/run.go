package deploy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/tidwall/sjson"
	"gopkg.in/yaml.v3"

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

	asMachine, err := convertScoreIntoMachine(scoreSpec)
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

	client, err := flymachinesclient.BuildScoreClient()
	if err != nil {
		return err
	}
	if args.App == "" {
		args.App = scoreSpec.Metadata["name"].(string)
	}

	slog.Info(fmt.Sprintf("Looking up app %s/%s..", args.Org, args.App))
	var existingApp *flymachinesclient.App
	if getAppResp, err := client.AppsShowWithResponse(context.Background(), args.App); err != nil {
		return fmt.Errorf("failed to make show app request: %w", err)
	} else {
		switch getAppResp.StatusCode() {
		case http.StatusOK:
			existingApp = getAppResp.JSON200
		case http.StatusNotFound:
			if strings.Contains(string(getAppResp.Body), "Could not find App") {
				slog.Info("Got a 404 for the App - assuming the App does not exist")
				break
			}
			fallthrough
		default:
			return fmt.Errorf("unexpected status code when showing app '%d': '%s'", getAppResp.StatusCode(), string(getAppResp.Body))
		}
	}

	var existingMachines []flymachinesclient.Machine
	if existingApp != nil {
		slog.Info(fmt.Sprintf("Looking up machines for app %s/%s..", args.Org, args.App))
		if listMachinesResp, err := client.MachinesListWithResponse(context.Background(), args.App, &flymachinesclient.MachinesListParams{}); err != nil {
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

	if existingApp == nil {
		slog.Info(fmt.Sprintf("Plan: create new app '%s' in org '%s'", args.App, args.Org))
	}
	if len(existingMachines) > 0 {
		slog.Info(fmt.Sprintf("Plan: update or destroy %d existing machines in the app", len(existingMachines)))
	} else {
		slog.Info(fmt.Sprintf("Plan: create 1 new machine in the app"))
	}
	if args.DryRun {
		slog.Info("Stopping here and dumping yaml because --dry-run was provided")
		var temp interface{}
		if err := json.Unmarshal([]byte(machineJson), &temp); err != nil {
			return fmt.Errorf("failed to unmarshal: %w", err)
		}
		_ = yaml.NewEncoder(os.Stdout).Encode(&temp)
		return nil
	}

	if existingApp == nil {
		slog.Info("Creating app..")
		if createAppResp, err := client.AppsCreateWithResponse(context.Background(), flymachinesclient.CreateAppRequest{
			OrgSlug: ref(args.Org),
			AppName: ref(args.App),
		}); err != nil {
			return fmt.Errorf("failed to make create app request: %w", err)
		} else {
			if createAppResp.StatusCode() >= 300 {
				return fmt.Errorf("unexpected status code when creating app '%d': '%s'", createAppResp.StatusCode(), string(createAppResp.Body))
			}
		}
		slog.Info("App created.")
	}

	if len(existingMachines) > 0 {
		for _, machine := range existingMachines {
			slog.Info(fmt.Sprintf("Updating machine '%s'..", *machine.Name))
			if updateMachineResp, err := client.MachinesUpdateWithResponse(context.Background(), args.App, *machine.Id, flymachinesclient.UpdateMachineRequest{
				Config:     ref(asMachine),
				SkipLaunch: ref(false),
			}); err != nil {
				return fmt.Errorf("failed to make update machine request: %w", err)
			} else {
				if updateMachineResp.StatusCode() >= 300 {
					return fmt.Errorf("unexpected status code when deleting machine '%d': '%s'", updateMachineResp.StatusCode(), string(updateMachineResp.Body))
				}
			}
		}
	} else {
		slog.Info(fmt.Sprintf("Creating machine.."))
		if machineCreateResp, err := client.MachinesCreateWithResponse(context.Background(), args.App, flymachinesclient.CreateMachineRequest{
			Config:     ref(asMachine),
			SkipLaunch: ref(false),
		}); err != nil {
			return fmt.Errorf("failed to make create machine request: %w", err)
		} else {
			if machineCreateResp.StatusCode() != http.StatusOK {
				return fmt.Errorf("unexpected status code when deleting machine '%d': '%s'", machineCreateResp.StatusCode(), string(machineCreateResp.Body))
			}
			slog.Info(fmt.Sprintf("Created machine '%s'", *(machineCreateResp.JSON200.Name)))
		}
	}

	return nil
}

func ref[k any](input k) *k {
	return &input
}

func convertScoreIntoMachine(spec *score.WorkloadSpec) (flymachinesclient.ApiMachineConfig, error) {
	output := flymachinesclient.ApiMachineConfig{}

	if len(spec.Containers) != 1 {
		return output, fmt.Errorf("score spec contains more than 1 container")
	}
	var container score.Container
	for _, container = range spec.Containers {
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
		var env map[string]string = container.Variables
		process.Env = ref(env)
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
			if v, err := strconv.ParseFloat(cpuReq, 32); err != nil {
				return output, fmt.Errorf("failed to parse container cpu resource '%s': %w", cpuReq, err)
			} else {
				output.Guest.Cpus = ref(int(math.Ceil(v)))
			}
		}
		if memoryBytes != "" {
			if v, err := strconv.ParseInt(memoryBytes, 10, 64); err != nil {
				return output, fmt.Errorf("failed to parse container memory resource '%s': %w", memoryBytes, err)
			} else {
				if d := float64(v) / float64(256*1024*1024); d < 1 || math.Round(d) != d {
					return output, fmt.Errorf("container memory resource must be a multiple of 256 MB but was '%s': %w", memoryBytes, err)
				} else {
					output.Guest.MemoryMb = ref(int(d * 256))
				}
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
		for _, portSpec := range spec.Service.Ports {
			flyServices = append(flyServices, flymachinesclient.ApiMachineService{
				Protocol:     portSpec.Protocol,
				InternalPort: portSpec.TargetPort,
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
				return output, fmt.Errorf("containers: files[%d]: mode is not supported", i)
			} else if containerFile.NoExpand != nil && *containerFile.NoExpand == false {
				return output, fmt.Errorf("containers: files[%d]: expand is not supported", i)
			}
			var rawContent string
			if v, ok := containerFile.Content.(string); ok && v != "" {
				rawContent = v
			} else if containerFile.Source != nil {
				if rawData, err := os.ReadFile(*containerFile.Source); err != nil {
					return output, fmt.Errorf("containers: files[%d]: failed to read source: %w", i, err)
				} else {
					rawContent = string(rawData)
				}
			} else {
				return output, fmt.Errorf("containers: files[%d]: is missing source or content", i)
			}
			rawContent = base64.StdEncoding.EncodeToString([]byte(rawContent))
			outputFiles = append(outputFiles, flymachinesclient.ApiFile{
				GuestPath: ref(containerFile.Target),
				RawValue:  &rawContent,
			})
		}
		output.Files = &outputFiles
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
