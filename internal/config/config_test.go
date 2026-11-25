package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// isolate points HOME and XDG_CONFIG_HOME at temp dirs, so tests never touch real files.
func isolate(t *testing.T) (home string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	return home
}

func TestSaveWritesConfigOwnerOnly(t *testing.T) {
	home := isolate(t)

	if err := Save(Default()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	dir := filepath.Join(home, ".config", "amp-tui")
	info, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected config mode 0600, got %04o", got)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("expected config dir mode 0700, got %04o", got)
	}
}

func TestConfigFilePathRespectsXDGConfigHome(t *testing.T) {
	isolate(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	path, err := ConfigFilePath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	if want := filepath.Join(xdg, "amp-tui", "config.json"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if err := Save(Default()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config under XDG_CONFIG_HOME: %v", err)
	}
}

func TestConfigFilePathIgnoresRelativeXDGConfigHome(t *testing.T) {
	home := isolate(t)
	t.Setenv("XDG_CONFIG_HOME", "relative/dir")

	path, err := ConfigFilePath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	if want := filepath.Join(home, ".config", "amp-tui", "config.json"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestSaveLoadRoundTripKeepsProviderSettings(t *testing.T) {
	isolate(t)
	cfg := Default()
	cfg.SetProviderSettings("deezer", ProviderSettings{DefaultQuality: "Flac", Options: map[string]string{"region": "fr"}})

	if err := Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got.SchemaVersion != SchemaVersion || got.Provider != "deezer" {
		t.Fatalf("unexpected header %d %q", got.SchemaVersion, got.Provider)
	}
	if !reflect.DeepEqual(got.ProviderSettings("deezer"), cfg.ProviderSettings("deezer")) {
		t.Fatalf("provider settings = %+v, want %+v", got.ProviderSettings("deezer"), cfg.ProviderSettings("deezer"))
	}
}

func TestSetProviderSettingsDoesNotChangeCopies(t *testing.T) {
	original := Default()
	original.SetProviderSettings("deezer", ProviderSettings{DefaultQuality: "Kbps128"})
	copied := original
	copied.SetProviderSettings("deezer", ProviderSettings{DefaultQuality: "Flac"})

	if got := original.ProviderSettings("deezer").DefaultQuality; got != "Kbps128" {
		t.Fatalf("original changed through a copy: %q", got)
	}
	if got := Default().ProviderSettings("missing"); !reflect.DeepEqual(got, ProviderSettings{}) {
		t.Fatalf("expected zero settings for unknown provider, got %+v", got)
	}
}

func TestNormalizeDisplayModeFallsBackToOffForUnknownMode(t *testing.T) {
	got := NormalizeDisplayMode(DisplayMode("unknown"), true)
	if got != DisplayModeOff {
		t.Fatalf("expected unknown display mode to fall back to off, got %q", got)
	}
}

func TestNormalizeDisplayModeAcceptsVisualizerModes(t *testing.T) {
	modes := []DisplayMode{
		DisplayModeOff,
		DisplayModeEqualizer,
		DisplayModeSpectrum,
		DisplayModeWaveform,
		DisplayModeScope,
	}
	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			if got := NormalizeDisplayMode(mode, true); got != mode {
				t.Fatalf("normalized mode = %q, want %q", got, mode)
			}
		})
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	isolate(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("expected defaults, got %+v", cfg)
	}
}

func TestEngineDefaultsToAutoAndRoundTrips(t *testing.T) {
	isolate(t)
	if got := Default().Engine; got != EngineAuto {
		t.Fatalf("default engine = %q, want %q", got, EngineAuto)
	}
	cfg := Default()
	cfg.Engine = EngineMPV
	if err := Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got.Engine != EngineMPV {
		t.Fatalf("loaded engine = %q, want %q", got.Engine, EngineMPV)
	}
}

func TestNormalizeEngineUsesAutoForUnknownValues(t *testing.T) {
	for _, tc := range []struct {
		input PlaybackEngine
		want  PlaybackEngine
	}{{EngineAuto, EngineAuto}, {EngineMPV, EngineMPV}, {EngineNative, EngineNative}, {"other", EngineAuto}, {"", EngineAuto}} {
		if got := NormalizeEngine(tc.input); got != tc.want {
			t.Errorf("NormalizeEngine(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func writeConfigFile(t *testing.T, data string) string {
	t.Helper()
	path, err := ConfigFilePath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadBrokenFileReturnsError(t *testing.T) {
	isolate(t)
	writeConfigFile(t, `{"theme": "Gruvbox",`)

	cfg, err := Load()
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("expected defaults with error, got %+v", cfg)
	}
}

func TestLoadUnknownSchemaVersionReturnsError(t *testing.T) {
	isolate(t)
	writeConfigFile(t, `{"schema_version": 99, "theme": "Gruvbox"}`)

	cfg, err := Load()
	if err == nil || !strings.Contains(err.Error(), "schema_version 99") {
		t.Fatalf("expected schema version error, got %v", err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("expected defaults with error, got %+v", cfg)
	}
}
