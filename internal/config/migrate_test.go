package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfate/amp-tui/internal/colorscheme"
	"github.com/sfate/amp-tui/internal/secrets"
)

const legacyJSON = `{
  "theme": "Gruvbox",
  "crossfade_enabled": true,
  "crossfade_duration_ms": 5000,
  "display_mode": "spectrum",
  "compact_view": false,
  "default_quality": "Flac",
  "arl": "legacy-arl-value"
}`

func writeLegacy(t *testing.T, home, data string) []byte {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, ".deezer-tui-config.json"), []byte(data), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	return []byte(data)
}

type failingSetStore struct{ secrets.Store }

func (failingSetStore) Set(string, string, string) error { return errors.New("keychain locked") }

func TestMigrateLegacyMovesSettingsAndSecret(t *testing.T) {
	home := isolate(t)
	before := writeLegacy(t, home, legacyJSON)
	store := secrets.NewMemory()

	migrated, err := MigrateLegacy(store)
	if err != nil || !migrated {
		t.Fatalf("migrated = %v, err = %v", migrated, err)
	}

	path, _ := ConfigFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read new config: %v", err)
	}
	if bytes.Contains(data, []byte("arl")) || bytes.Contains(data, []byte("legacy-arl-value")) {
		t.Fatalf("new config holds the ARL: %s", data)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load new config: %v", err)
	}
	if cfg.SchemaVersion != 2 || cfg.Provider != "deezer" {
		t.Fatalf("unexpected header %d %q", cfg.SchemaVersion, cfg.Provider)
	}
	if cfg.Theme != colorscheme.Gruvbox || !cfg.CrossfadeEnabled || cfg.CrossfadeDurationMS != 5000 || cfg.DisplayMode != DisplayModeSpectrum || cfg.CompactView {
		t.Fatalf("settings not carried over: %+v", cfg)
	}
	if got := cfg.ProviderSettings("deezer").DefaultQuality; got != "Flac" {
		t.Fatalf("deezer quality = %q, want Flac", got)
	}
	if got, err := store.Get("deezer", secrets.CredentialKey); err != nil || got != "legacy-arl-value" {
		t.Fatalf("stored credential = %q, err = %v", got, err)
	}
	after, err := os.ReadFile(filepath.Join(home, ".deezer-tui-config.json"))
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("legacy file changed: %q, err = %v", after, err)
	}
}

func TestMigrateLegacySkippedWhenNewFileExists(t *testing.T) {
	home := isolate(t)
	writeLegacy(t, home, legacyJSON)
	existing := writeConfigFile(t, `{"schema_version": 2, "provider": "deezer", "theme": "Winamp"}`)
	store := secrets.NewMemory()

	migrated, err := MigrateLegacy(store)
	if err != nil || migrated {
		t.Fatalf("migrated = %v, err = %v", migrated, err)
	}
	data, _ := os.ReadFile(existing)
	if !strings.Contains(string(data), "Winamp") {
		t.Fatalf("existing config changed: %s", data)
	}
	if _, err := store.Get("deezer", secrets.CredentialKey); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("expected no stored credential, got err %v", err)
	}
}

func TestMigrateLegacyWithoutOldFileDoesNothing(t *testing.T) {
	isolate(t)
	migrated, err := MigrateLegacy(secrets.NewMemory())
	if err != nil || migrated {
		t.Fatalf("migrated = %v, err = %v", migrated, err)
	}
	path, _ := ConfigFilePath()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no new config, stat err = %v", err)
	}
}

func TestMigrateLegacySecretFailureLeavesNoNewFile(t *testing.T) {
	home := isolate(t)
	writeLegacy(t, home, legacyJSON)

	migrated, err := MigrateLegacy(failingSetStore{secrets.NewMemory()})
	if err == nil || migrated || !strings.Contains(err.Error(), "keychain locked") {
		t.Fatalf("migrated = %v, err = %v", migrated, err)
	}
	path, _ := ConfigFilePath()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no new config, stat err = %v", err)
	}
}

func TestMigrateLegacyBrokenOldFileReturnsError(t *testing.T) {
	home := isolate(t)
	before := writeLegacy(t, home, `{"arl": "secret",`)

	migrated, err := MigrateLegacy(secrets.NewMemory())
	if err == nil || migrated {
		t.Fatalf("migrated = %v, err = %v", migrated, err)
	}
	path, _ := ConfigFilePath()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no new config, stat err = %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(home, ".deezer-tui-config.json"))
	if !bytes.Equal(after, before) {
		t.Fatalf("legacy file changed: %q", after)
	}
}
