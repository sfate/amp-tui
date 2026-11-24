package doctor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sfate/amp-tui/internal/secrets"
)

type secretProbe struct {
	err  error
	sets int
}

func (p *secretProbe) Get(string, string) (string, error) { return "", p.err }
func (p *secretProbe) Set(string, string, string) error   { p.sets++; return nil }
func (p *secretProbe) Delete(string, string) error        { return nil }

func TestSecretStoreCheckAcceptsNotFound(t *testing.T) {
	probe := &secretProbe{err: secrets.ErrNotFound}
	check := SecretStoreCheck(probe, "/tmp/secrets.json")
	if err := check.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if probe.sets != 0 {
		t.Fatal("doctor must not write to the secret store")
	}
}

func TestSecretStoreCheckReportsBackendError(t *testing.T) {
	check := SecretStoreCheck(&secretProbe{err: errors.New("no secret service")}, "/tmp/secrets.json")
	if err := check.Run(context.Background()); err == nil {
		t.Fatal("expected backend error")
	}
	if !strings.Contains(check.Hint, "/tmp/secrets.json") {
		t.Fatalf("hint %q does not name the fallback file", check.Hint)
	}
}

func TestMissingKeychainWarnsWithoutFailingRun(t *testing.T) {
	var out strings.Builder
	check := SecretStoreCheck(&secretProbe{err: errors.New("no secret service")}, "/tmp/secrets.json")
	if err := Run(context.Background(), &out, []Check{check}); err != nil {
		t.Fatalf("Run() = %v, want nil because the file fallback works", err)
	}
	if !strings.HasPrefix(out.String(), "warn  ") {
		t.Fatalf("output %q, want a warn line", out.String())
	}
}
