package runcmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/tidwall/sjson"

	"github.com/astromechza/score-flyio/flytoml"
	"github.com/astromechza/score-flyio/internal/convert"
	"github.com/astromechza/score-flyio/score"
)

func Run(args Args) error {
	slog.Debug("Running deploy subcommand", "args", args)
	slog.Info("Validating Score input..")
	scoreSpec, err := score.ParseAndValidate(args.ScoreFileContent)
	if err != nil {
		return fmt.Errorf("score spec was not valid: %w", err)
	}
	slog.Info("Score input is valid.")
	cfg, err := convert.ConvertScoreToFlyConfig(args.App, scoreSpec)
	if err != nil {
		return fmt.Errorf("failed to convert: %w", err)
	}

	// in order to apply sjson modifications, we need to coerce through json
	rawMachineJson, _ := json.Marshal(cfg)
	machineJson := string(rawMachineJson)

	if len(args.Extensions) > 0 {
		slog.Info(fmt.Sprintf("Applying %d extensions..", len(args.Extensions)))
		for _, extension := range args.Extensions {
			machineJson, err = sjson.Set(machineJson, extension.Path, extension.Set)
		}
		cfg = new(flytoml.Config)
		if err := json.Unmarshal([]byte(machineJson), &cfg); err != nil {
			return fmt.Errorf("failed to convert json with extensions back to toml: %w", err)
		}
	}

	if err := toml.NewEncoder(os.Stdout).Encode(cfg); err != nil {
		return fmt.Errorf("failed to encode: %w", err)
	}
	return nil
}
