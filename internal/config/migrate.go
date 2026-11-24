package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sfate/amp-tui/internal/secrets"
)

// LegacyFileName is the v1 config file in the home dir.
const LegacyFileName = ".deezer-tui-config.json"

func LegacyFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("could not resolve the user's home directory")
	}
	return filepath.Join(home, LegacyFileName), nil
}

// legacyFields are the v1 keys that have no place in the v2 schema.
type legacyFields struct {
	DefaultQuality string `json:"default_quality"`
	ARL            string `json:"arl"`
}

// MigrateLegacy writes a v2 config from the v1 file when no v2 file exists yet, and moves the ARL into store.
// It never changes the v1 file. On error it writes no v2 file, so the next start tries again.
func MigrateLegacy(store secrets.Store) (bool, error) {
	path, err := ConfigFilePath()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	legacyPath, err := LegacyFilePath()
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(legacyPath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("migrate %s: %w", legacyPath, err)
	}

	cfg, err := decode(data)
	if err != nil {
		return false, fmt.Errorf("migrate %s: %w", legacyPath, err)
	}
	var old legacyFields
	if err := json.Unmarshal(data, &old); err != nil {
		return false, fmt.Errorf("migrate %s: %w", legacyPath, err)
	}
	cfg.SchemaVersion = SchemaVersion
	cfg.Provider = DefaultProvider
	cfg.Providers = nil
	if old.DefaultQuality != "" {
		cfg.SetProviderSettings(DefaultProvider, ProviderSettings{DefaultQuality: old.DefaultQuality})
	}
	if old.ARL != "" {
		if err := store.Set(DefaultProvider, secrets.CredentialKey, old.ARL); err != nil {
			return false, fmt.Errorf("migrate %s: store the Deezer login: %w", legacyPath, err)
		}
	}
	if err := Save(cfg); err != nil {
		return false, fmt.Errorf("migrate %s: %w", legacyPath, err)
	}
	return true, nil
}
