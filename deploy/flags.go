package deploy

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

const (
	FlagSubcommand = "deploy"
	FlagHelpPrefix = `
`
	FlagHelpSuffix = ``
)

type Args struct {
	Org              string
	App              string
	DryRun           bool
	ScoreFileContent []byte
}

func ParseFlagArgs(parent *flag.FlagSet) (Args, error) {
	fs := flag.NewFlagSet(parent.Name(), parent.ErrorHandling())
	fs.SetOutput(fs.Output())
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), FlagHelpPrefix)
		fs.PrintDefaults()
		_, _ = fmt.Fprintf(fs.Output(), FlagHelpSuffix)
	}
	receiver := new(Args)
	fs.BoolVar(&receiver.DryRun, "dry-run", false, "Validated inputs and remote state but don't change anything")
	fs.StringVar(&receiver.Org, "org", "personal", "The target Fly.io organization")
	fs.StringVar(&receiver.App, "app", "", "The target Fly.io app name otherwise the name of the Score workload will be used")
	if err := fs.Parse(parent.Args()[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return *receiver, flag.ErrHelp
		}
		return *receiver, err
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(fs.Output(), "Expected a file as the 1st and only positional argument.\n")
		fs.Usage()
		return *receiver, flag.ErrHelp
	}
	if fs.Arg(0) == "-" {
		if content, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<22)); err != nil {
			return *receiver, fmt.Errorf("failed to fully read stdin: %w", err)
		} else {
			receiver.ScoreFileContent = content
		}
	} else if content, err := os.ReadFile(fs.Arg(0)); err != nil {
		return *receiver, fmt.Errorf("failed to fully read score file '%s': %w", fs.Arg(0), err)
	} else {
		receiver.ScoreFileContent = content
	}
	return *receiver, nil
}
