package doctor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunReportsEachCheck(t *testing.T) {
	var out bytes.Buffer
	err := Run(context.Background(), &out, []Check{
		{Name: "good", Run: func(context.Context) error { return nil }},
		{Name: "bad", Hint: "install bad", Run: func(context.Context) error { return errors.New("missing") }},
	})
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("expected ErrFailed, got %v", err)
	}
	text := out.String()
	for _, want := range []string{"ok    good", "FAIL  bad: missing", "install bad"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output %q does not contain %q", text, want)
		}
	}
}

func TestRunWithoutChecks(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), &out, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "no runtime dependencies") {
		t.Fatalf("unexpected output %q", out.String())
	}
}

func TestCommandSucceedsReportsMissingBinary(t *testing.T) {
	if err := commandSucceeds(context.Background(), "amp-tui-no-such-binary"); err == nil {
		t.Fatal("expected error for missing binary")
	}
}
