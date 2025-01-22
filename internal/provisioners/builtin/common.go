package builtin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/spf13/cobra"

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

func buildProvisionCommand(inner func(inputs provisioners.ProvisionerInputs, stderr io.Writer) (*provisioners.ProvisionerOutputs, error)) *cobra.Command {
	return &cobra.Command{
		Use:           "provision",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			inputs, err := ReadProvisionerInputs(cmd.InOrStdin())
			if err != nil {
				return err
			}
			slog.SetDefault(slog.Default().WithGroup(inputs.ResourceId))
			cmd.SilenceUsage = true
			out, err := inner(inputs, cmd.ErrOrStderr())
			if out != nil {
				err = errors.Join(err, json.NewEncoder(cmd.OutOrStdout()).Encode(out))
			}
			return err
		},
	}
}

func buildDeProvisionCommand(inner func(inputs provisioners.ProvisionerInputs, stderr io.Writer) (*provisioners.ProvisionerOutputs, error)) *cobra.Command {
	return &cobra.Command{
		Use:           "deprovision",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			inputs, err := ReadProvisionerInputs(cmd.InOrStdin())
			if err != nil {
				return err
			}
			slog.SetDefault(slog.Default().WithGroup(inputs.ResourceId))
			cmd.SilenceUsage = true
			out, err := inner(inputs, cmd.ErrOrStderr())
			if out != nil {
				if out.ResourceSecrets != nil || out.ResourceValues != nil || out.ResourceState != nil {
					return errors.Join(err, fmt.Errorf("deprovision output cannot include resource local state, values, or secrets"))
				}
				return errors.Join(err, json.NewEncoder(cmd.OutOrStdout()).Encode(out))
			}
			return err
		},
	}
}

func buildProvisionGroup(name string, provision, deprovision *cobra.Command) *cobra.Command {
	group := &cobra.Command{Use: name}
	group.AddCommand(provision, deprovision)
	return group
}
