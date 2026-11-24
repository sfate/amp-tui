package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/doctor"
	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/provider/deezer"
	"github.com/sfate/amp-tui/internal/secrets"
	"github.com/sfate/amp-tui/internal/tui"
)

type options struct {
	doctor bool
}

func parseOptions(args []string, out io.Writer) (options, error) {
	var o options
	flags := flag.NewFlagSet("amp-tui", flag.ContinueOnError)
	flags.SetOutput(out)
	flags.BoolVar(&o.doctor, "doctor", false, "check runtime dependencies and exit")
	if err := flags.Parse(args); err != nil {
		return o, err
	}
	if flags.NArg() > 0 {
		return o, fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	return o, nil
}

func run(args []string) error {
	o, err := parseOptions(args, os.Stderr)
	if err != nil {
		return err
	}
	secretsPath, err := config.SecretsFilePath()
	if err != nil {
		return err
	}
	if o.doctor {
		checks := append(doctor.PlatformChecks(), doctor.SecretStoreCheck(secrets.NewKeyring(), secretsPath))
		return doctor.Run(context.Background(), os.Stdout, checks)
	}
	model := tui.New(tui.WithRegistry(newRegistry()), tui.WithSecrets(secrets.Default(secretsPath)))
	_, err = tea.NewProgram(model).Run()
	return err
}

func newRegistry() *provider.Registry {
	reg := provider.NewRegistry()
	deezer.Register(reg)
	return reg
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "amp-tui: %v\n", err)
		os.Exit(1)
	}
}
