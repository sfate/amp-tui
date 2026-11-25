package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"

	"github.com/sfate/amp-tui/internal/colorscheme"
	"github.com/sfate/amp-tui/internal/fsutil"
)

// SchemaVersion is the config file format this build reads and writes.
const SchemaVersion = 2

// DefaultProvider is the provider ID of a new config.
const DefaultProvider = "deezer"

// Config holds no secrets; credentials live in the secrets store.
type Config struct {
	SchemaVersion       int                         `json:"schema_version"`
	Provider            string                      `json:"provider"`
	Engine              PlaybackEngine              `json:"engine"`
	Theme               colorscheme.Name            `json:"theme"`
	CrossfadeEnabled    bool                        `json:"crossfade_enabled"`
	CrossfadeDurationMS uint64                      `json:"crossfade_duration_ms"`
	DisplayMode         DisplayMode                 `json:"display_mode"`
	DisplayEnabled      bool                        `json:"display_enabled,omitempty"`
	CompactView         bool                        `json:"compact_view"`
	Providers           map[string]ProviderSettings `json:"providers,omitempty"`
}

type PlaybackEngine string

const (
	EngineAuto   PlaybackEngine = "auto"
	EngineMPV    PlaybackEngine = "mpv"
	EngineNative PlaybackEngine = "native"
)

// ProviderSettings holds the non-secret settings of one provider.
type ProviderSettings struct {
	// DefaultQuality is a provider quality ID; empty or unknown gives the provider default.
	DefaultQuality string            `json:"default_quality,omitempty"`
	Options        map[string]string `json:"options,omitempty"`
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
		SchemaVersion:       SchemaVersion,
		Provider:            DefaultProvider,
		Engine:              EngineAuto,
		Theme:               colorscheme.Aetheria,
		CrossfadeEnabled:    false,
		CrossfadeDurationMS: 0,
		DisplayMode:         DisplayModeEqualizer,
		DisplayEnabled:      true,
		CompactView:         true,
	}
}

// ProviderSettings returns the zero value for a provider without saved settings.
func (c Config) ProviderSettings(id string) ProviderSettings {
	return c.Providers[id]
}

// SetProviderSettings copies the map first, because Config values are copied across the TUI.
func (c *Config) SetProviderSettings(id string, s ProviderSettings) {
	providers := maps.Clone(c.Providers)
	if providers == nil {
		providers = map[string]ProviderSettings{}
	}
	providers[id] = s
	c.Providers = providers
}

// Dir is $XDG_CONFIG_HOME/amp-tui, else ~/.config/amp-tui, on every OS.
func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "amp-tui"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("could not resolve the user's home directory")
	}
	return filepath.Join(home, ".config", "amp-tui"), nil
}

func ConfigFilePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// SecretsFilePath is the secrets file used when no keychain is available.
func SecretsFilePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "secrets.json"), nil
}

// Load reads the config file. A missing file gives defaults and no error.
// A file that cannot be read, parsed or has a newer schema gives defaults and an error; the caller must not save over it.
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
	cfg, err := decode(data)
	if err != nil {
		return Default(), fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.SchemaVersion > SchemaVersion {
		return Default(), fmt.Errorf("%s has schema_version %d; this amp-tui reads up to %d", path, cfg.SchemaVersion, SchemaVersion)
	}
	cfg.SchemaVersion = SchemaVersion
	if cfg.Provider == "" {
		cfg.Provider = DefaultProvider
	}
	return cfg, nil
}

// decode applies the settings shared by the v2 file and the legacy file over the defaults.
func decode(data []byte) (Config, error) {
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), err
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)
	cfg.Theme = colorscheme.Normalize(cfg.Theme)
	cfg.Engine = NormalizeEngine(cfg.Engine)
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
	cfg.SchemaVersion = SchemaVersion
	cfg.Theme = colorscheme.Normalize(cfg.Theme)
	cfg.Engine = NormalizeEngine(cfg.Engine)
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
	return fsutil.WriteFileAtomic(path, data, 0o600)
}

func NormalizeEngine(engine PlaybackEngine) PlaybackEngine {
	switch engine {
	case EngineAuto, EngineMPV, EngineNative:
		return engine
	default:
		return EngineAuto
	}
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
