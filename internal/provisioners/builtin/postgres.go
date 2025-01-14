package builtin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/astromechza/score-flyio/internal/provisioners"
)

var (
	builtinPostgresProvision = &cobra.Command{
		Use:           "provision",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			inputs, err := ReadProvisionerInputs(cmd.InOrStdin())
			if err != nil {
				return err
			}
			pgApp, ok := inputs.ResourceState["app"].(string)
			if !ok {
				pgApp = "pg-" + time.Now().UTC().Format("20060102150405")
			}
			password, ok := inputs.ResourceState["password"].(string)
			if !ok {
				passwordBytes := make([]byte, 10)
				_, _ = rand.Read(passwordBytes)
				password = hex.EncodeToString(passwordBytes)
			}
			org, err := flyOrg()
			if err != nil {
				return err
			}
			cmd.SilenceUsage = true
			var createErr error
			if apps, err := ListApps(org, cmd.ErrOrStderr()); err != nil {
				return err
			} else if !slices.ContainsFunc(apps, func(app ListedApp) bool {
				log.Printf("Postgres app %s already exists in status '%s'", pgApp, app.Status)
				return app.Name == pgApp
			}) {
				region, err := flyRegion()
				if err != nil {
					return err
				}
				log.Printf("Provisioning new postgres app")
				c := exec.Command(
					"fly", "postgres", "create", "--org", org,
					"--name", pgApp, "--region", region, "--password", password, "--autostart",
					"--initial-cluster-size", "1", "--vm-size", "shared-cpu-1x", "--volume-size", "10",
				)
				c.Env = os.Environ()
				c.Stderr = cmd.ErrOrStderr()
				c.Stdout = cmd.ErrOrStderr()
				createErr = c.Run()
			}

			outputs := provisioners.ProvisionerOutputs{
				ResourceState: map[string]interface{}{
					"app":      pgApp,
					"password": password,
				},
				ResourceValues: map[string]interface{}{
					"host":     pgApp + ".flycast",
					"port":     "5432",
					"username": "postgres",
					// Although this is intended as a postgres-instance driver, we also output the default database name
					//	here so that it can technically be used as a postgres database directly.
					"name":     "postgres",
					"database": "postgres",
				},
				ResourceSecrets: map[string]interface{}{
					"password": password,
				},
			}
			_ = json.NewEncoder(cmd.OutOrStdout()).Encode(outputs)
			return createErr
		},
	}

	builtinPostgresDeProvision = &cobra.Command{
		Use:           "deprovision",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			inputs, err := ReadProvisionerInputs(cmd.InOrStdin())
			if err != nil {
				return err
			}
			pgApp, ok := inputs.ResourceState["app"].(string)
			if !ok {
				log.Printf("Resource never provisioned an app")
				return nil
			}
			org, err := flyOrg()
			if err != nil {
				return err
			}
			cmd.SilenceUsage = true
			if apps, err := ListApps(org, cmd.ErrOrStderr()); err != nil {
				return err
			} else if !slices.ContainsFunc(apps, func(app ListedApp) bool {
				return app.Name == pgApp
			}) {
				log.Printf("Postgres app %s does not exist", pgApp)
				return nil
			}
			c := exec.Command("fly", "app", "destroy", pgApp, "--yes")
			c.Env = os.Environ()
			c.Stderr = cmd.ErrOrStderr()
			c.Stdout = cmd.ErrOrStderr()
			return c.Run()
		},
	}

	builtinPostgresGroup = &cobra.Command{
		Use: "postgres-instance",
	}

	builtinGroup = &cobra.Command{
		Use: "builtin-provisioners",
	}
)

func Install(parent *cobra.Command) {
	parent.AddCommand(builtinGroup)
}

func init() {
	builtinPostgresGroup.AddCommand(builtinPostgresProvision, builtinPostgresDeProvision)
	builtinGroup.AddCommand(builtinPostgresGroup)
}
