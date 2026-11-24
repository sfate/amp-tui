package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/doctor"
	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/provider/deezer"
	"github.com/sfate/amp-tui/internal/secrets"
	"github.com/sfate/amp-tui/internal/tui"
)

type options struct {
	doctor   bool
	provider provider.ID
}

// usageError marks bad command-line input, which exits with status 2 like flag does.
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func exitCode(err error) int {
	var usage usageError
	if errors.As(err, &usage) {
		return 2
	}
	return 1
}

func parseOptions(args []string, out io.Writer, reg *provider.Registry) (options, error) {
	var o options
	var providerID string
	known := knownIDs(reg)
	flags := flag.NewFlagSet("amp-tui", flag.ContinueOnError)
	flags.SetOutput(out)
	flags.BoolVar(&o.doctor, "doctor", false, "check runtime dependencies and exit")
	flags.StringVar(&providerID, "provider", "", "music provider for this run only, not saved ("+known+")")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return o, err
		}
		return o, usageError{err.Error()}
	}
	if flags.NArg() > 0 {
		return o, usageError{fmt.Sprintf("unexpected arguments: %v", flags.Args())}
	}
	if providerID != "" {
		if _, ok := reg.Lookup(provider.ID(providerID)); !ok {
			return o, usageError{fmt.Sprintf("unknown provider %q; known providers: %s", providerID, known)}
		}
		o.provider = provider.ID(providerID)
	}
	return o, nil
}

func knownIDs(reg *provider.Registry) string {
	ids := reg.IDs()
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = string(id)
	}
	return strings.Join(names, ", ")
}

func run(args []string) error {
	reg := newRegistry()
	o, err := parseOptions(args, os.Stderr, reg)
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
	model := tui.New(tui.WithRegistry(reg), tui.WithSecrets(secrets.Default(secretsPath)), tui.WithProviderOverride(o.provider))
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
		os.Exit(exitCode(err))
	}
}
