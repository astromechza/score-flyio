package deploy

import (
	"fmt"
	"log/slog"

	"github.com/astromechza/score-flyio/score"
)

func Run(args Args) error {
	slog.Debug("Running deploy subcommand", "args", args)
	slog.Info("Validating Score input..")
	_, err := score.ParseAndValidate(args.ScoreFileContent)
	if err != nil {
		return fmt.Errorf("score spec was not valid: %w", err)
	}
	slog.Info("Score input is valid.")
	return nil
}
