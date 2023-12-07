package deploy

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
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
	Extensions       []Extension
}

type Extension struct {
	Path   string      `json:"path"`
	Set    interface{} `json:"set"`
	Delete bool        `json:"delete"`
}

type extensionsFlag struct {
	Extensions []Extension
}

func (of *extensionsFlag) String() string {
	return fmt.Sprintf("%v", of.Extensions)
}

func (of *extensionsFlag) Set(value string) error {
	parts := strings.Split(value, "=")
	if len(parts) < 2 {
		return fmt.Errorf("does not contain '='")
	}
	if parts[1] == "" {
		of.Extensions = append(of.Extensions, Extension{Path: parts[0], Delete: true})
	} else {
		var temp interface{}
		if err := json.Unmarshal([]byte(parts[1]), &temp); err != nil {
			return fmt.Errorf("could not json decode extension: %w", err)
		} else {
			of.Extensions = append(of.Extensions, Extension{Path: parts[0], Set: temp})
		}
	}
	return nil
}

func ParseFlagArgs(parent *flag.FlagSet) (Args, error) {
	fs := flag.NewFlagSet(parent.Name(), parent.ErrorHandling())
	fs.SetOutput(fs.Output())
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), FlagHelpPrefix)
		fs.PrintDefaults()
		_, _ = fmt.Fprintf(fs.Output(), FlagHelpSuffix)
	}
	receiver := &Args{Extensions: make([]Extension, 0)}
	fs.BoolVar(&receiver.DryRun, "dry-run", false, "Validated inputs and remote state but don't change anything")
	fs.StringVar(&receiver.Org, "org", "personal", "The target Fly.io organization")
	fs.StringVar(&receiver.App, "app", "", "The target Fly.io app name otherwise the name of the Score workload will be used")

	extensionsReceiver := extensionsFlag{Extensions: make([]Extension, 0)}
	fs.Var(&extensionsReceiver, "extension", "An extension in the outgoing Fly machine config to set, as json separated by a =")

	var extensionsFile string
	fs.StringVar(&extensionsFile, "extensions", "", "A file containing a list of extensions {path: string, set: any, delete: bool}")

	if err := fs.Parse(parent.Args()[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return *receiver, flag.ErrHelp
		}
		return *receiver, err
	}

	if extensionsFile != "" {
		if raw, err := os.ReadFile(extensionsFile); err != nil {
			return *receiver, fmt.Errorf("failed to read extensions file: %w", err)
		} else if err := yaml.Unmarshal(raw, &receiver.Extensions); err != nil {
			return *receiver, fmt.Errorf("failed to decode extensions file: %w", err)
		}
	}

	receiver.Extensions = append(receiver.Extensions, extensionsReceiver.Extensions...)

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
