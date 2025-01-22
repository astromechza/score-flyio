package builtin

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	rand2 "math/rand"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/astromechza/score-flyio/internal/flymachines"
	"github.com/astromechza/score-flyio/internal/provisioners"
)

const (
	SharedStateKey = "builtin-provisioners-postgres"
)

var (
	builtinPostgresInstanceProvision = buildProvisionCommand(func(inputs provisioners.ProvisionerInputs, stderr io.Writer) (*provisioners.ProvisionerOutputs, error) {
		pgApp, ok := inputs.ResourceState["app"].(string)
		password, _ := inputs.ResourceState["password"].(string)
		if !ok {
			pgApp = FlyAppPrefixFromState(inputs.SharedState) + "pg-" + time.Now().UTC().Format("20060102150405")
			passwordBytes := make([]byte, 10)
			_, _ = rand.Read(passwordBytes)
			password = hex.EncodeToString(passwordBytes)
		}
		fc, err := flymachines.NewFlyClient()
		if err != nil {
			return nil, fmt.Errorf("failed to setup fly api client: %w", err)
		}
		outputs := &provisioners.ProvisionerOutputs{
			ResourceState: map[string]interface{}{
				"app":      pgApp,
				"password": password,
			},
		}
		createErr := ensurePostgresInstance(fc, pgApp, password, stderr)
		if createErr == nil {
			outputs.ResourceValues = map[string]interface{}{
				"host":     pgApp + ".flycast",
				"port":     "5432",
				"username": "postgres",
				// Although this is intended as a postgres-instance driver, we also output the default database name
				//	here so that it can technically be used as a postgres database directly.
				"name":     "postgres",
				"database": "postgres",
			}
			outputs.ResourceSecrets = map[string]interface{}{
				"password": password,
			}
		}
		return outputs, createErr
	})

	builtinPostgresInstanceDeProvision = buildDeProvisionCommand(func(inputs provisioners.ProvisionerInputs, stderr io.Writer) (*provisioners.ProvisionerOutputs, error) {
		pgApp, ok := inputs.ResourceState["app"].(string)
		if !ok {
			return nil, nil
		}
		fc, err := flymachines.NewFlyClient()
		if err != nil {
			return nil, fmt.Errorf("failed to setup fly api client: %w", err)
		}
		return nil, flymachines.DeleteApp(fc, pgApp)
	})

	builtinPostgresProvision = buildProvisionCommand(func(inputs provisioners.ProvisionerInputs, stderr io.Writer) (*provisioners.ProvisionerOutputs, error) {
		sharedState, _ := inputs.SharedState[SharedStateKey].(map[string]interface{})
		pgApp, ok := sharedState["app"].(string)
		password, _ := sharedState["password"].(string)
		dbNames, _ := sharedState["dbNames"].([]interface{})
		if !ok {
			pgApp = FlyAppPrefixFromState(inputs.SharedState) + "pg-" + time.Now().UTC().Format("20060102150405")
			passwordBytes := make([]byte, 10)
			_, _ = rand.Read(passwordBytes)
			password = hex.EncodeToString(passwordBytes)
		}
		fc, err := flymachines.NewFlyClient()
		if err != nil {
			return nil, fmt.Errorf("failed to setup fly api client: %w", err)
		}
		outputs := &provisioners.ProvisionerOutputs{
			SharedState: map[string]interface{}{
				SharedStateKey: map[string]interface{}{
					"app":      pgApp,
					"password": password,
					"dbNames":  dbNames,
				},
			},
		}
		if err := ensurePostgresInstance(fc, pgApp, password, stderr); err != nil {
			return outputs, fmt.Errorf("failed to ensure postgres instance: %w", err)
		}
		dbName, ok := inputs.ResourceState["database"].(string)
		dbUser, _ := inputs.ResourceState["username"].(string)
		dbPassword, _ := inputs.ResourceState["password"].(string)
		if !ok {
			dbName = strings.Replace(inputs.ResourceId, ".", "-", -1) + strconv.Itoa(1000+rand2.Intn(9000))
			dbUser = dbName + "-user"
			passwordBytes := make([]byte, 10)
			_, _ = rand.Read(passwordBytes)
			dbPassword = hex.EncodeToString(passwordBytes)
			if err := flymachines.ExecAnyStartedMachine(fc, pgApp, []string{"/bin/bash", "-c", fmt.Sprintf(`psql "postgresql://postgres:${OPERATOR_PASSWORD}@localhost:5432/postgres" -c "CREATE DATABASE \"%[1]s\""`, dbName)}); err != nil {
				return nil, fmt.Errorf("failed to create database: %w", err)
			}
			if err := flymachines.ExecAnyStartedMachine(fc, pgApp, []string{"/bin/bash", "-c", fmt.Sprintf(`psql "postgresql://postgres:${OPERATOR_PASSWORD}@localhost:5432/postgres" -c "CREATE USER \"%[1]s\" WITH PASSWORD '%[2]s'"`, dbUser, dbPassword)}); err != nil {
				return nil, fmt.Errorf("failed to create user: %w", err)
			}
			if err := flymachines.ExecAnyStartedMachine(fc, pgApp, []string{"/bin/bash", "-c", fmt.Sprintf(`psql "postgresql://postgres:${OPERATOR_PASSWORD}@localhost:5432/postgres" -c "GRANT ALL PRIVILEGES ON DATABASE \"%s\" TO \"%s\""`, dbName, dbUser)}); err != nil {
				return nil, fmt.Errorf("failed to assign role to user: %w", err)
			}
			if err := flymachines.ExecAnyStartedMachine(fc, pgApp, []string{"/bin/bash", "-c", fmt.Sprintf(`psql "postgresql://postgres:${OPERATOR_PASSWORD}@localhost:5432/%s" -c "GRANT ALL ON SCHEMA public TO \"%s\""`, dbName, dbUser)}); err != nil {
				return nil, fmt.Errorf("failed to grant schema public user: %w", err)
			}
			dbNames = append(dbNames, dbName)
			{
				ss := outputs.SharedState[SharedStateKey].(map[string]interface{})
				ss["dbNames"] = dbNames
				outputs.SharedState[SharedStateKey] = ss
			}
			outputs.ResourceState = map[string]interface{}{
				"database": dbName,
				"username": dbUser,
				"password": dbPassword,
			}
		}
		outputs.ResourceValues = map[string]interface{}{
			"host":     pgApp + ".flycast",
			"port":     "5432",
			"username": dbUser,
			"name":     dbName,
			"database": dbName,
		}
		outputs.ResourceSecrets = map[string]interface{}{
			"password": dbPassword,
		}
		return outputs, nil
	})

	builtinPostgresDeProvision = buildDeProvisionCommand(func(inputs provisioners.ProvisionerInputs, stderr io.Writer) (*provisioners.ProvisionerOutputs, error) {
		dbName, ok := inputs.ResourceState["database"].(string)
		if !ok {
			return nil, nil
		}
		dbUser, _ := inputs.ResourceState["username"].(string)
		sharedState, ok := inputs.SharedState[SharedStateKey].(map[string]interface{})
		if !ok {
			return nil, nil
		}
		pgApp, _ := sharedState["app"].(string)
		dbNames, _ := sharedState["dbNames"].([]interface{})
		if !slices.ContainsFunc(dbNames, func(i interface{}) bool {
			x, _ := i.(string)
			return x == dbName
		}) {
			slog.Info("Postgres database is already de-provisioned", slog.String("database", dbName))
			return nil, nil
		}
		fc, err := flymachines.NewFlyClient()
		if err != nil {
			return nil, fmt.Errorf("failed to setup fly api client: %w", err)
		}
		dbNames = slices.DeleteFunc(dbNames, func(i interface{}) bool {
			x, _ := i.(string)
			return x == dbName
		})
		if len(dbNames) == 0 {
			slog.Info("Deprovisioning postgres app since this was the last database", slog.String("database", dbName), slog.String("app", pgApp))
			if err := flymachines.DeleteApp(fc, pgApp); err != nil {
				return nil, err
			}
			return &provisioners.ProvisionerOutputs{
				SharedState: map[string]interface{}{
					SharedStateKey: nil,
				},
			}, nil
		}
		slog.Info("Dropping postgres database from instance", slog.String("database", dbName), slog.String("app", pgApp))
		if err := flymachines.ExecAnyStartedMachine(fc, pgApp, []string{"/bin/bash", "-c", fmt.Sprintf(
			`set -e; psql "postgresql://postgres:${OPERATOR_PASSWORD}@localhost:5432/postgres" -c "DROP DATABASE IF EXISTS \"%s\" WITH (FORCE)"; psql "postgresql://postgres:${OPERATOR_PASSWORD}@localhost:5432/postgres" -c "DROP USER IF EXISTS \"%s\""`, dbName, dbUser)}); err != nil {
			return nil, fmt.Errorf("failed to delete database: %w", err)
		}
		sharedState["dbNames"] = dbNames
		return &provisioners.ProvisionerOutputs{
			SharedState: map[string]interface{}{
				SharedStateKey: sharedState,
			},
		}, nil
	})
)

func ensurePostgresInstance(c *flymachines.FlyClient, app, password string, stderr io.Writer) error {
	if flyApp, ok, err := flymachines.GetApp(c, app); err != nil {
		return err
	} else if ok {
		slog.Info("Postgres app already exists", slog.String("app", app), slog.String("status", *flyApp.Status))
	} else {
		region, err := flyRegion()
		if err != nil {
			return err
		}
		slog.Info("Provisioning new postgres app", slog.String("app", app), slog.String("region", region))
		c := exec.Command(
			"fly", "postgres", "create", "--access-token", c.ApiToken,
			"--name", app, "--region", region, "--password", password, "--autostart",
			"--initial-cluster-size", "1", "--vm-size", "shared-cpu-1x", "--volume-size", "10",
		)
		c.Env = os.Environ()
		c.Stderr = stderr
		c.Stdout = stderr
		return c.Run()
	}
	return nil
}

func Install(parent *cobra.Command) {
	group := &cobra.Command{Use: "builtin-provisioners"}
	group.AddCommand(buildProvisionGroup("postgres-instance", builtinPostgresInstanceProvision, builtinPostgresInstanceDeProvision))
	group.AddCommand(buildProvisionGroup("postgres", builtinPostgresProvision, builtinPostgresDeProvision))
	parent.AddCommand(group)
}
