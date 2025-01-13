package command

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astromechza/score-flyio/internal/state"
)

const (
	addProvResClassFlag   = "res-class"
	addProvResIdFlag      = "res-id"
	addProvCmdStaticFlag  = "static-json"
	addProvCmdBinFlag     = "cmd-binary"
	addProvCmdBinArgsFlag = "cmd-args"
	addProvHttpUrlFlag    = "http-url"
)

var (
	provisionersGroup = &cobra.Command{
		Use: "provisioners",
	}

	listProvisioners = &cobra.Command{
		Use:           "list",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			sd, ok, err := state.LoadStateDirectory(".")
			if err != nil {
				return fmt.Errorf("failed to load existing state directory: %w", err)
			} else if !ok {
				return fmt.Errorf("state directory does not exist, please run \"score-compose init\" first")
			}
			for i, provisioner := range sd.State.Extras.Provisioners {
				var t string
				if provisioner.Http != nil {
					t = fmt.Sprintf("HTTP %s", provisioner.Http.Url)
				} else if provisioner.Cmd != nil {
					t = fmt.Sprintf("CMD %s", provisioner.Cmd.Binary)
				} else if provisioner.Static != nil {
					t = fmt.Sprintf("Static #%d", len(*provisioner.Static))
				}
				fmt.Printf("[%d]: %s (%s.%s#%s) %s\n", i, provisioner.ProvisionerId, provisioner.ResourceType, provisioner.ResourceClass, provisioner.ResourceId, t)
			}
			return nil
		},
	}

	addProvisioner = &cobra.Command{
		Use:           fmt.Sprintf("add PROVISIONER_ID RESOURCE_TYPE (--%s|--%s|--%s)", addProvCmdBinFlag, addProvHttpUrlFlag, addProvCmdStaticFlag),
		Args:          cobra.ExactArgs(2),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			sd, ok, err := state.LoadStateDirectory(".")
			if err != nil {
				return fmt.Errorf("failed to load existing state directory: %w", err)
			} else if !ok {
				return fmt.Errorf("state directory does not exist, please run \"score-compose init\" first")
			}
			newProv := state.Provisioner{
				ProvisionerId: args[0],
				ResourceType:  args[1],
			}
			newProv.ResourceClass, _ = cmd.Flags().GetString(addProvResClassFlag)
			newProv.ResourceId, _ = cmd.Flags().GetString(addProvResIdFlag)

			if b, _ := cmd.Flags().GetString(addProvCmdBinFlag); b != "" {
				if !strings.HasPrefix(b, "/") {
					if strings.Contains(b, "/") {
						if b, err = filepath.Abs(b); err != nil {
							return fmt.Errorf("failed to resolve the binary as an absolute path: %w", err)
						}
					} else if b, err = exec.LookPath(b); err != nil {
						return fmt.Errorf("failed to find '%s' on path", b)
					}
				}
				args, _ := cmd.Flags().GetStringSlice(addProvCmdBinArgsFlag)
				newProv.Cmd = &state.CmdProvisioner{Binary: b, Args: args}
			} else if u, _ := cmd.Flags().GetString(addProvHttpUrlFlag); u != "" {
				if uu, err := url.Parse(u); err != nil || uu.Scheme == "" {
					return fmt.Errorf("invalid url '%s' for an http provisioner", u)
				}
				newProv.Http = &state.HttpProvisioner{Url: u}
			} else if r, _ := cmd.Flags().GetString(addProvCmdStaticFlag); r != "" {
				var o map[string]interface{}
				if err = json.Unmarshal([]byte(r), &o); err != nil {
					return fmt.Errorf("static json fails to decode as provisioner outputs: %w", err)
				}
				newProv.Static = &o
			} else {
				return fmt.Errorf("expected either --%s, --%s, or --%s", addProvHttpUrlFlag, addProvCmdBinFlag, addProvCmdStaticFlag)
			}

			slog.Info("Inserting new provisioner into state file", slog.String("res-type", newProv.ResourceType), slog.String("res-class", newProv.ResourceClass), slog.String("res-id", newProv.ResourceId))
			existingProvisioners := sd.State.Extras.Provisioners
			existingProvisioners = slices.DeleteFunc(existingProvisioners, func(provisioner state.Provisioner) bool {
				if provisioner.ProvisionerId == newProv.ProvisionerId {
					slog.Info("Removing existing provisioner with the same id", slog.String("id", provisioner.ProvisionerId))
					return true
				} else if provisioner.ResourceType == newProv.ResourceType && provisioner.ResourceClass == newProv.ResourceClass && provisioner.ResourceId == newProv.ResourceId {
					slog.Info("Removing existing provisioner with the same res type, class, and id", slog.String("id", provisioner.ProvisionerId))
					return true
				}
				return false
			})
			sd.State.Extras.Provisioners = append([]state.Provisioner{newProv}, existingProvisioners...)
			slog.Info("Writing new state directory", "dir", sd.Path)
			if err := sd.Persist(); err != nil {
				return fmt.Errorf("failed to persist new state directory: %w", err)
			}
			return nil
		},
	}

	removeProvisioner = &cobra.Command{
		Use:           "remove PROVISIONER_ID",
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			sd, ok, err := state.LoadStateDirectory(".")
			if err != nil {
				return fmt.Errorf("failed to load existing state directory: %w", err)
			} else if !ok {
				return fmt.Errorf("state directory does not exist, please run \"score-compose init\" first")
			}
			sd.State.Extras.Provisioners = slices.DeleteFunc(sd.State.Extras.Provisioners, func(provisioner state.Provisioner) bool {
				if provisioner.ProvisionerId == args[0] {
					slog.Info("Removing provisioner with id", slog.String("id", provisioner.ProvisionerId))
					return true
				}
				return false
			})
			slog.Info("Writing new state directory", "dir", sd.Path)
			if err := sd.Persist(); err != nil {
				return fmt.Errorf("failed to persist new state directory: %w", err)
			}
			return nil
		},
	}
)

func init() {
	addProvisioner.Flags().String(addProvResClassFlag, "", "The resource class to match")
	addProvisioner.Flags().String(addProvResIdFlag, "", "The resource id to match")

	addProvisioner.Flags().String(addProvCmdStaticFlag, "", "The static json to return for this provisioner")
	addProvisioner.Flags().String(addProvCmdBinFlag, "", "The binary to execute for a cmd provisioner")
	addProvisioner.Flags().StringSlice(addProvCmdBinArgsFlag, nil, "The arguments to the binary to execute")
	addProvisioner.Flags().String(addProvHttpUrlFlag, "", "The http url to request for an http provisioner")

	addProvisioner.MarkFlagsOneRequired(addProvCmdStaticFlag, addProvHttpUrlFlag, addProvCmdBinFlag)
	addProvisioner.MarkFlagsMutuallyExclusive(addProvCmdStaticFlag, addProvHttpUrlFlag, addProvCmdBinFlag)

	provisionersGroup.AddCommand(listProvisioners)
	provisionersGroup.AddCommand(addProvisioner)
	provisionersGroup.AddCommand(removeProvisioner)
	rootCmd.AddCommand(provisionersGroup)
}
