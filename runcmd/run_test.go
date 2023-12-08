package runcmd

import (
	"strconv"
	"testing"
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
