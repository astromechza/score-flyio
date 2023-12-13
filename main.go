package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/astromechza/score-flyio/runcmd"
)

const (
	FlagHelpPrefix = `Usage: score-flyio [global options...] <subcommand> ...

Available subcommands:
  run	Convert the input Score file into a Fly.io toml file.

Global options:
`
	FlagHelpSuffix = `
Use "score-flyio" <subcommand> --help for more information about a given subcommand.
`
)

func main() {
	if err := mainInner(os.Args, os.Stdout); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(2)
		} else {
			log.Printf("Error: " + err.Error())
			os.Exit(1)
		}
	}
}

type argsStruct struct {
	Debug bool

	Subcommand string
	DeployArgs runcmd.Args
}

func parse(args []string, output io.Writer) (argsStruct, error) {
	fs := flag.NewFlagSet(filepath.Base(args[0]), flag.ContinueOnError)
	fs.SetOutput(output)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), FlagHelpPrefix)
		fs.PrintDefaults()
		_, _ = fmt.Fprintf(fs.Output(), FlagHelpSuffix)
	}
	receiver := new(argsStruct)
	fs.BoolVar(&receiver.Debug, "debug", false, "Enable debug logging")

	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return *receiver, flag.ErrHelp
		}
		return *receiver, err
	}
	if fs.NArg() < 1 {
		_, _ = fmt.Fprintf(fs.Output(), "Expected a subcommand as the first positional argument.\n")
		fs.Usage()
		return *receiver, flag.ErrHelp
	}
	receiver.Subcommand = fs.Arg(0)
	switch fs.Arg(0) {
	case runcmd.FlagSubcommand:
		subArgs, err := runcmd.ParseFlagArgs(fs)
		if err != nil {
			return *receiver, err
		}
		receiver.DeployArgs = subArgs
		return *receiver, nil
	default:
		_, _ = fmt.Fprintf(fs.Output(), "Unrecognised subcommand '%s'.\n", fs.Arg(0))
		fs.Usage()
		return *receiver, flag.ErrHelp
	}
}

func mainInner(args []string, output io.Writer) error {
	parsedArgs, err := parse(args, output)
	if err != nil {
		return err
	}
	if parsedArgs.Debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{AddSource: true, Level: slog.LevelDebug})))
	}
	switch parsedArgs.Subcommand {
	case runcmd.FlagSubcommand:
		return runcmd.Run(parsedArgs.DeployArgs)
	}
	return nil
}
