package command

import (
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"maps"
	"slices"

	"github.com/score-spec/score-go/framework"
	"github.com/spf13/cobra"

	"github.com/astromechza/score-flyio/internal/provisioners"
	"github.com/astromechza/score-flyio/internal/state"
	"github.com/astromechza/score-flyio/internal/thingprinter"
)

const ()

var (
	resourcesGroup = &cobra.Command{
		Use:   "resources",
		Short: "inspect resources in the project",
	}

	listResources = &cobra.Command{
		Use:           "list",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			sd, ok, err := state.LoadStateDirectory(".")
			if err != nil {
				return fmt.Errorf("failed to load existing state directory: %w", err)
			} else if !ok {
				return fmt.Errorf("state directory does not exist, please run \"score-flyio init\" first")
			}
			things := slices.Collect(iterMap2To1(maps.All(sd.State.Resources), func(uid framework.ResourceUid, st framework.ScoreResourceState[state.ResourceExtras]) thingprinter.PrintableMap {
				return thingprinter.PrintableMap{
					"uid":             string(uid),
					"type":            st.Type,
					"class":           st.Class,
					"id":              st.Id,
					"source_workload": st.SourceWorkload,
					"provisioner":     st.ProvisionerUri,
					"state":           st.State,
					"outputs":         st.Outputs,
				}
			}))
			columns := []string{"uid", "source_workload", "provisioner", "outputs"}
			return thingprinter.PrintTable(cmd.OutOrStdout(), columns, things)
		},
	}

	deProvisionResource = &cobra.Command{
		Use:           "deprovision",
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			sd, ok, err := state.LoadStateDirectory(".")
			if err != nil {
				return fmt.Errorf("failed to load existing state directory: %w", err)
			} else if !ok {
				return fmt.Errorf("state directory does not exist, please run \"score-flyio init\" first")
			}
			out, err := provisioners.DeProvisionResource(&sd.State, framework.ResourceUid(args[0]))
			if err != nil {
				return fmt.Errorf("failed to deprovision: %w", err)
			}
			sd.State = *out
			if persistErr := sd.Persist(); persistErr != nil {
				return fmt.Errorf("failed to persist state file: %w", errors.Join(persistErr, err))
			}
			slog.Info("Persisted state file")
			return nil
		},
	}
)

func init() {
	resourcesGroup.AddCommand(listResources)
	resourcesGroup.AddCommand(deProvisionResource)
	rootCmd.AddCommand(resourcesGroup)
}

func iterMap2To1[a any, b any, c any](seq iter.Seq2[a, b], f func(a, b) c) iter.Seq[c] {
	return func(yield func(c) bool) {
		next, stop := iter.Pull2(seq)
		defer stop()
		for {
			if a, b, ok := next(); !ok {
				return
			} else if !yield(f(a, b)) {
				return
			}
		}
	}
}
