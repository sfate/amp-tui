package main

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/sfate/amp-tui/internal/provider"
)

func TestParseOptions(t *testing.T) {
	reg := newRegistry()
	o, err := parseOptions([]string{"--doctor"}, io.Discard, reg)
	if err != nil || !o.doctor {
		t.Fatalf("expected doctor option, got %+v, %v", o, err)
	}
	if _, err := parseOptions([]string{"extra"}, io.Discard, reg); err == nil {
		t.Fatal("expected error for positional arguments")
	}
	if _, err := parseOptions([]string{"-h"}, io.Discard, reg); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}
}

func TestParseOptionsProvider(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register("deezer", "Deezer", nil)
	reg.Register("plain", "Plain", nil)

	o, err := parseOptions([]string{"--provider", "plain"}, io.Discard, reg)
	if err != nil || o.provider != "plain" {
		t.Fatalf("expected provider plain, got %+v, %v", o, err)
	}

	_, err = parseOptions([]string{"--provider", "tidal"}, io.Discard, reg)
	var usage usageError
	if !errors.As(err, &usage) {
		t.Fatalf("expected a usage error, got %v", err)
	}
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "deezer, plain") {
		t.Fatalf("expected exit 2 with the known IDs, got %d %q", exitCode(err), err)
	}

	var help bytes.Buffer
	_, _ = parseOptions([]string{"-h"}, &help, reg)
	if !strings.Contains(help.String(), "-provider") || !strings.Contains(help.String(), "deezer, plain") {
		t.Fatalf("expected --help to list the provider flag and IDs:\n%s", help.String())
	}
}
