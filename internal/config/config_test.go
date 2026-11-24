package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveWritesConfigOwnerOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := Save(Config{ARL: "secret"}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	info, err := os.Stat(filepath.Join(home, ".deezer-tui-config.json"))
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected config mode 0600, got %04o", got)
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
	t.Setenv("HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != Default() {
		t.Fatalf("expected defaults, got %+v", cfg)
	}
}

func TestLoadBrokenFileReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".deezer-tui-config.json"), []byte(`{"arl": "secret",`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err == nil {
		t.Fatal("expected parse error")
	}
	if cfg != Default() {
		t.Fatalf("expected defaults with error, got %+v", cfg)
	}
}
