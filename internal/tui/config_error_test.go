package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfate/amp-tui/internal/app"
)

func TestBrokenConfigFileIsNotOverwrittenBySettingsChange(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".deezer-tui-config.json")
	broken := []byte(`{"arl": "secret", "theme": `)
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	model := New()
	if !strings.Contains(model.app.StatusMessage, "Config error") {
		t.Fatalf("expected config error in status, got %q", model.app.StatusMessage)
	}

	model.app.ActivePanel = app.ActivePanelMain
	model.app.ViewingSettings = true
	model.app.SettingsState.Select(intPtr(0))
	model.adjustSelectedSetting(1)

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
