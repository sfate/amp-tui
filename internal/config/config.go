package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sfate/amp-tui/internal/colorscheme"
)

type Config struct {
	Theme               colorscheme.Name `json:"theme"`
	CrossfadeEnabled    bool             `json:"crossfade_enabled"`
	CrossfadeDurationMS uint64           `json:"crossfade_duration_ms"`
	DisplayMode         DisplayMode      `json:"display_mode"`
	DisplayEnabled      bool             `json:"display_enabled,omitempty"`
	CompactView         bool             `json:"compact_view"`
	DefaultQuality      string           `json:"default_quality"` // provider quality ID
	ARL                 string           `json:"arl"`
}

type DisplayMode string

const (
	DisplayModeOff       DisplayMode = "off"
	DisplayModeEqualizer DisplayMode = "equalizer"
	DisplayModeSpectrum  DisplayMode = "spectrum"
	DisplayModeWaveform  DisplayMode = "waveform"
	DisplayModeScope     DisplayMode = "oscilloscope"
)

func Default() Config {
	return Config{
		Theme:               colorscheme.Aetheria,
		CrossfadeEnabled:    false,
		CrossfadeDurationMS: 0,
		DisplayMode:         DisplayModeEqualizer,
		DisplayEnabled:      true,
		CompactView:         true,
		DefaultQuality:      "Kbps320",
		ARL:                 "",
	}
}

func ConfigFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("could not resolve the user's home directory")
	}
	return filepath.Join(home, ".deezer-tui-config.json"), nil
}

// Load reads the config file. A missing file gives defaults and no error.
// A file that cannot be read or parsed gives defaults and an error; the caller must not save over it.
func Load() (Config, error) {
	path, err := ConfigFilePath()
	if err != nil {
		return Default(), err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), fmt.Errorf("read %s: %w", path, err)
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("parse %s: %w", path, err)
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)
	cfg.Theme = colorscheme.Normalize(cfg.Theme)
	if _, ok := raw["display_mode"]; !ok {
		_, hasLegacyDisplayEnabled := raw["display_enabled"]
		cfg.DisplayMode = NormalizeDisplayMode("", !hasLegacyDisplayEnabled || cfg.DisplayEnabled)
	} else {
		cfg.DisplayMode = NormalizeDisplayMode(cfg.DisplayMode, cfg.DisplayEnabled)
	}
	cfg.DisplayEnabled = cfg.DisplayMode != DisplayModeOff
	return cfg, nil
}

func Save(cfg Config) error {
	cfg.Theme = colorscheme.Normalize(cfg.Theme)
	cfg.DisplayMode = NormalizeDisplayMode(cfg.DisplayMode, cfg.DisplayEnabled)
	cfg.DisplayEnabled = cfg.DisplayMode != DisplayModeOff
	path, err := ConfigFilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func NormalizeDisplayMode(mode DisplayMode, legacyEnabled bool) DisplayMode {
	switch mode {
	case DisplayModeOff, DisplayModeEqualizer, DisplayModeSpectrum, DisplayModeWaveform, DisplayModeScope:
		return mode
	case "":
		if !legacyEnabled {
			return DisplayModeOff
		}
		return DisplayModeEqualizer
	default:
		return DisplayModeOff
	}
}
