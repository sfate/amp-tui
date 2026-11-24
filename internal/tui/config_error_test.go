package tui

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/secrets"
)

// isolateConfig points HOME and XDG_CONFIG_HOME at a temp dir and returns the v2 config path.
func isolateConfig(t *testing.T) (home, path string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path, err := config.ConfigFilePath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	return home, path
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func changeThemeSetting(model *Model) {
	model.app.ActivePanel = app.ActivePanelMain
	model.app.ViewingSettings = true
	model.app.SettingsState.Select(intPtr(slices.Index(model.settingsRows(), settingTheme)))
	model.adjustSelectedSetting(1)
}

func TestBrokenConfigFileIsNotOverwrittenBySettingsChange(t *testing.T) {
	_, path := isolateConfig(t)
	broken := []byte(`{"provider": "deezer", "theme": `)
	writeTestFile(t, path, broken)

	model := New(WithSecrets(secrets.NewMemory()))
	if !strings.Contains(model.app.StatusMessage, "Config error") {
		t.Fatalf("expected config error in status, got %q", model.app.StatusMessage)
	}

	changeThemeSetting(&model)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != string(broken) {
		t.Fatalf("config file changed: %q", got)
	}
	if !strings.Contains(model.app.StatusMessage, "fix or remove the config file") {
		t.Fatalf("expected save refusal in status, got %q", model.app.StatusMessage)
	}
}

func TestNewerSchemaConfigBlocksConfigAndSecretWrites(t *testing.T) {
	_, path := isolateConfig(t)
	future := []byte(`{"schema_version": 99, "provider": "deezer"}`)
	writeTestFile(t, path, future)
	store := secrets.NewMemory()

	model := New(WithRegistry(testRegistry()), WithSecrets(store))
	if !strings.Contains(model.app.StatusMessage, "schema_version 99") {
		t.Fatalf("expected schema error in status, got %q", model.app.StatusMessage)
	}
	changeThemeSetting(&model)
	model.onLoginSucceeded(loginSucceededMsg{secret: "new-token", provider: withStream(&fakeLoader{})})

	if got, _ := os.ReadFile(path); !bytes.Equal(got, future) {
		t.Fatalf("config file changed: %q", got)
	}
	if _, err := store.Get("deezer", secrets.CredentialKey); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("expected no stored credential, got err %v", err)
	}
	if !strings.Contains(model.app.StatusMessage, "fix or remove the config file") {
		t.Fatalf("expected save refusal in status, got %q", model.app.StatusMessage)
	}
}
