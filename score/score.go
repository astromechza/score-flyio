package score

//go:generate go run github.com/atombender/go-jsonschema@v0.14.1 -v --schema-output=https://score.dev/schemas/score=types.gen.go --schema-package=https://score.dev/schemas/score=score --schema-root-type=https://score.dev/schemas/score=WorkloadSpec score-v1b1.json.modified

import (
	_ "embed"
	"errors"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

//go:embed score-v1b1.json.modified
var rawEmbeddedSchema string

// embeddedSchema is the compiled version of rawEmbeddedSchema
var embeddedSchema *jsonschema.Schema

func init() {
	if s, err := jsonschema.CompileString("", rawEmbeddedSchema); err != nil {
		panic(err)
	} else {
		embeddedSchema = s
	}
}

func ParseAndValidate(content []byte) (*WorkloadSpec, error) {
	var temp map[string]interface{}
	if err := yaml.Unmarshal(content, &temp); err != nil {
		return nil, err
	}
	if v, ok := temp["apiVersion"].(string); !ok || v != "score.dev/v1b1" {
		return nil, errors.New("apiVersion: expected 'score.dev/v1b1'")
	}
	if err := embeddedSchema.Validate(temp); err != nil {
		return nil, err
	}
	final := new(WorkloadSpec)
	if err := yaml.Unmarshal(content, &final); err != nil {
		return nil, err
	}
	if v, ok := final.Metadata["name"].(string); !ok || v == "" {
		return nil, errors.New("metadata: name: is missing")
	}
	return final, nil
}
