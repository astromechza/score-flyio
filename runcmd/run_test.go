package runcmd

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/astromechza/score-flyio/flygraphqlclient"
	"github.com/astromechza/score-flyio/flymachinesclient"
	"github.com/astromechza/score-flyio/score"
)

func Test_convertCpu(t *testing.T) {
	for _, tc := range []struct {
		input  string
		output int
		error  string
	}{
		{input: "1", output: 1},
		{input: "2.0", output: 2},
		{input: "2e1", output: 20},
		{input: "100m", error: "Fly.io can only support integer numbers of cpus (0.1 != 0)"},
		{input: "0.5", error: "Fly.io can only support integer numbers of cpus (0.5 != 1)"},
		{input: "", error: "does not match regex pattern"},
		{input: "000", error: "value was not positive"},
	} {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			v, err := convertCpu(tc.input)
			if tc.error != "" {
				if err == nil {
					t.Errorf("no error, expected '%s'", tc.error)
				} else if err.Error() != tc.error {
					t.Errorf("expected error '%s' got '%s'", tc.error, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got '%s'", err.Error())
				} else if v != tc.output {
					t.Errorf("expected '%d', got '%d'", tc.output, v)
				}
			}
		})
	}
}

func Test_convertMemory(t *testing.T) {
	for _, tc := range []struct {
		input  string
		output int
		error  string
	}{
		{input: strconv.Itoa(256 * 1_000_000), output: 256},
		{input: "256M", output: 256},
		{input: "1024M", output: 1024},
		{input: "1G", error: "Fly.io can only support multiples of 256 Megabytes of memory (got 1000M)"},
		{input: "100m", error: "unsupported unit"},
		{input: "0.5", error: "Fly.io can only support multiples of 256 Megabytes of memory (got 0M)"},
		{input: "", error: "does not match regex pattern"},
		{input: "000", error: "value was not positive"},
	} {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			v, err := convertMemoryToMegabytes(tc.input)
			if tc.error != "" {
				if err == nil {
					t.Errorf("no error, expected '%s'", tc.error)
				} else if err.Error() != tc.error {
					t.Errorf("expected error '%s' got '%s'", tc.error, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got '%s'", err.Error())
				} else if v != tc.output {
					t.Errorf("expected '%d', got '%d'", tc.output, v)
				}
			}
		})
	}
}

func Test_convertSpecTests(t *testing.T) {
	_ = os.Setenv("SOME_KEY", "SOME_VALUE")
	for _, tc := range []struct {
		name   string
		input  score.WorkloadSpec
		output flymachinesclient.ApiMachineConfig
		error  string
	}{
		{
			name: "metadata substitutions",
			input: score.WorkloadSpec{
				Metadata: map[string]interface{}{"thing": 42},
				Containers: map[string]score.Container{
					"c": {
						Variables: map[string]string{
							"A": "B",
							"B": "$$C",
							"C": "${metadata.thing}",
						},
						Files: []score.ContainerFilesElem{{
							Target:  "/path",
							Content: "hello ${metadata.thing}",
						}},
					},
				},
			},
			output: flymachinesclient.ApiMachineConfig{
				Image:     ref(""),
				Files:     ref([]flymachinesclient.ApiFile{{GuestPath: ref("/path"), RawValue: ref(base64.StdEncoding.EncodeToString([]byte("hello 42")))}}),
				Processes: ref([]flymachinesclient.ApiMachineProcess{{Env: ref(map[string]string{"A": "B", "B": "$C", "C": "42"})}}),
			},
		},
		{
			name:  "bad var substitution",
			input: score.WorkloadSpec{Containers: map[string]score.Container{"c": {Variables: map[string]string{"A": "${thing}"}}}},
			error: "containers.c.variables.A: failed to interpolate: unsupported expression reference 'thing'",
		},
		{
			name:  "bad files substitution",
			input: score.WorkloadSpec{Containers: map[string]score.Container{"c": {Files: []score.ContainerFilesElem{{Target: "/", Content: "${thing}"}}}}},
			error: "containers.c.files[0]: failed to substitute content: unsupported expression reference 'thing'",
		},
		{
			name: "environment resource",
			input: score.WorkloadSpec{
				Containers: map[string]score.Container{"c": {Variables: map[string]string{"A": "${resources.env.SOME_KEY}"}}},
				Resources:  map[string]score.Resource{"env": {Type: "environment"}},
			},
			output: flymachinesclient.ApiMachineConfig{
				Image:     ref(""),
				Processes: ref([]flymachinesclient.ApiMachineProcess{{Env: ref(map[string]string{"A": "SOME_VALUE"})}}),
			},
		},
		{
			name: "environment resource with missing variable",
			input: score.WorkloadSpec{
				Containers: map[string]score.Container{"c": {Variables: map[string]string{"A": "${resources.env.SCOREFLYIORANDOMKEY}"}}},
				Resources:  map[string]score.Resource{"env": {Type: "environment"}},
			},
			error: "containers.c.variables.A: failed to interpolate: property SCOREFLYIORANDOMKEY not set on resource type",
		},
		{
			name: "unsupported environment class",
			input: score.WorkloadSpec{
				Containers: map[string]score.Container{"c": {Variables: map[string]string{"A": "${resources.env.SCOREFLYIORANDOMKEY}"}}},
				Resources:  map[string]score.Resource{"env": {Type: "environment", Class: ref("unknown")}},
			},
			error: "resources: 'env': environment.'unknown' class not supported",
		},
		{
			name: "default dns resource",
			input: score.WorkloadSpec{
				Containers: map[string]score.Container{"c": {Variables: map[string]string{"A": "${resources.d.host}"}}},
				Resources:  map[string]score.Resource{"d": {Type: "dns"}},
			},
			output: flymachinesclient.ApiMachineConfig{
				Image:     ref(""),
				Processes: ref([]flymachinesclient.ApiMachineProcess{{Env: ref(map[string]string{"A": "my-app.internal"})}}),
			},
		},
		{
			name: "external dns resource",
			input: score.WorkloadSpec{
				Containers: map[string]score.Container{"c": {Variables: map[string]string{"A": "${resources.d.host}"}}},
				Resources:  map[string]score.Resource{"d": {Type: "dns", Class: ref("external")}},
			},
			output: flymachinesclient.ApiMachineConfig{
				Image:     ref(""),
				Processes: ref([]flymachinesclient.ApiMachineProcess{{Env: ref(map[string]string{"A": "my-app.fly.dev"})}}),
			},
		},
		{
			name: "unsupported dns class",
			input: score.WorkloadSpec{
				Containers: map[string]score.Container{"c": {Variables: map[string]string{"A": "${resources.d.host}"}}},
				Resources:  map[string]score.Resource{"d": {Type: "dns", Class: ref("unknown")}},
			},
			error: "resources: 'd': dns.'unknown' class not supported",
		},
		{
			name: "existing volume id",
			input: score.WorkloadSpec{
				Containers: map[string]score.Container{"c": {Volumes: []score.ContainerVolumesElem{{Source: "${resources.v}", Target: "/path"}}}},
				Resources: map[string]score.Resource{"v": {Type: "volume", Metadata: map[string]interface{}{"annotations": score.ResourceMetadata{
					"score-flyio/volume_id": "vol_123456789",
				}}}},
			},
			output: flymachinesclient.ApiMachineConfig{
				Image:  ref(""),
				Mounts: &[]flymachinesclient.ApiMachineMount{{Path: ref("/path"), Volume: ref("vol_123456789")}},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			o, err := convertScoreIntoMachine("my-app", &tc.input, flygraphqlclient.GetAppExtrasApp{
				Hostname: "my-app.fly.dev",
			})
			if tc.error != "" {
				if err == nil {
					t.Errorf("no error, expected '%s'", tc.error)
				} else if err.Error() != tc.error {
					t.Errorf("expected error '%s' got '%s'", tc.error, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got '%s'", err.Error())
				} else if !reflect.DeepEqual(tc.output, o) {
					expected, _ := json.MarshalIndent(tc.output, "", "  ")
					actual, _ := json.MarshalIndent(o, "", "  ")
					t.Errorf("expected:\n%s\n, got:\n %s", string(expected), string(actual))
				}
			}
		})
	}
}
