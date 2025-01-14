package builtin

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/astromechza/score-flyio/internal/provisioners"
)

func ReadProvisionerInputs(r io.Reader) (provisioners.ProvisionerInputs, error) {
	var inputs provisioners.ProvisionerInputs
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&inputs); err != nil {
		return inputs, fmt.Errorf("failed to decode provisioner inputs: %w", err)
	}
	return inputs, nil
}
