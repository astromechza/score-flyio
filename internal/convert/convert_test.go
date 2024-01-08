package convert

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/astromechza/score-flyio/flytoml"
	"github.com/astromechza/score-flyio/internal/drivers"
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
		output flytoml.Config
		error  string
	}{
		{
			name: "metadata substitutions",
			input: score.WorkloadSpec{
				Metadata: map[string]interface{}{"thing": 42},
				Containers: map[string]score.Container{
					"c": {
						Image: "my-image",
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
			output: flytoml.Config{
				AppName:       "my-app",
				PrimaryRegion: "lhr",
				Build:         &flytoml.Build{Image: "my-image"},
				Env: map[string]string{
					"A": "B", "B": "$C", "C": "42",
				},
				Processes: map[string]string{"c": ""},
				Files:     []flytoml.File{{Processes: []string{"c"}, GuestPath: "/path", RawValue: base64.StdEncoding.EncodeToString([]byte("hello 42"))}},
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
			output: flytoml.Config{
				AppName:       "my-app",
				PrimaryRegion: "lhr",
				Build:         &flytoml.Build{Image: ""},
				Env: map[string]string{
					"A": "SOME_VALUE",
				},
				Processes: map[string]string{"c": ""},
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
			error: "resources: 'env': failed to find a driver supporting environment.unknown#env",
		},
		{
			name: "default dns resource",
			input: score.WorkloadSpec{
				Containers: map[string]score.Container{"c": {Variables: map[string]string{"A": "${resources.d.host}"}}},
				Resources:  map[string]score.Resource{"d": {Type: "dns"}},
			},
			output: flytoml.Config{
				AppName:       "my-app",
				PrimaryRegion: "lhr",
				Build:         &flytoml.Build{Image: ""},
				Env: map[string]string{
					"A": "my-app.internal",
				},
				Processes: map[string]string{"c": ""},
			},
		},
		{
			name: "external dns resource",
			input: score.WorkloadSpec{
				Containers: map[string]score.Container{"c": {Variables: map[string]string{"A": "${resources.d.host}"}}},
				Resources:  map[string]score.Resource{"d": {Type: "dns", Class: ref("external")}},
			},
			output: flytoml.Config{
				AppName:       "my-app",
				PrimaryRegion: "lhr",
				Build:         &flytoml.Build{Image: ""},
				Env: map[string]string{
					"A": "my-app.fly.dev",
				},
				Processes: map[string]string{"c": ""},
			},
		},
		{
			name: "unsupported dns class",
			input: score.WorkloadSpec{
				Containers: map[string]score.Container{"c": {Variables: map[string]string{"A": "${resources.d.host}"}}},
				Resources:  map[string]score.Resource{"d": {Type: "dns", Class: ref("unknown")}},
			},
			error: "resources: 'd': failed to find a driver supporting dns.unknown#d",
		},
		{
			name: "service without port",
			input: score.WorkloadSpec{
				Containers: map[string]score.Container{"c": {}},
				Service: &score.WorkloadSpecService{
					Ports: map[string]score.ServicePort{"my-port": {}},
				},
			},
			error: "service: 'my-port' must have a port specified",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			defaultDrivers, _, _ := drivers.GenerateDefaultDrivers("my-app")
			o, err := ConvertScoreToFlyConfig("my-app", "lhr", &tc.input, defaultDrivers)
			if tc.error != "" {
				if err == nil {
					t.Errorf("no error, expected '%s'", tc.error)
				} else if err.Error() != tc.error {
					t.Errorf("expected error '%s' got '%s'", tc.error, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got '%s'", err.Error())
				} else {
					expected, _ := json.MarshalIndent(tc.output, "", "  ")
					actual, _ := json.MarshalIndent(o, "", "  ")
					if string(expected) != string(actual) {
						t.Errorf("expected:\n%s\n, got:\n %s", string(expected), string(actual))
					}
				}
			}
		})
	}
}
