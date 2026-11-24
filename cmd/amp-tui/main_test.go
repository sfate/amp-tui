package main

import (
	"errors"
	"flag"
	"io"
	"testing"
)

func TestParseOptions(t *testing.T) {
	o, err := parseOptions([]string{"--doctor"}, io.Discard)
	if err != nil || !o.doctor {
		t.Fatalf("expected doctor option, got %+v, %v", o, err)
	}
	if _, err := parseOptions([]string{"extra"}, io.Discard); err == nil {
		t.Fatal("expected error for positional arguments")
	}
	if _, err := parseOptions([]string{"-h"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}
}
