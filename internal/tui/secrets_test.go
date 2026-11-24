package tui

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/secrets"
)

func TestLoginStoresCredentialInSecretStoreNotConfig(t *testing.T) {
	_, path := isolateConfig(t)
	store := secrets.NewMemory()
	model := New(WithRegistry(testRegistry()), WithSecrets(store))
	model.login.step = 1

	next, _ := model.Update(tea.KeyPressMsg(tea.Key{Text: "Cookie: sid=session; arl=token-value"}))
	model = next.(Model)
	next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "enter"}))
	model = next.(Model)
	next, _ = model.Update(cmd())
	model = next.(Model)

	if model.login.active {
		t.Fatalf("expected login to finish, status %q", model.app.StatusMessage)
	}
	if got, err := store.Get("deezer", secrets.CredentialKey); err != nil || got != "token-value" {
		t.Fatalf("stored credential = %q, err = %v", got, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if bytes.Contains(data, []byte("token-value")) || bytes.Contains(data, []byte("arl")) {
		t.Fatalf("config file holds the secret: %s", data)
	}
	if !bytes.Contains(data, []byte(`"provider": "deezer"`)) {
		t.Fatalf("config file lacks the provider: %s", data)
	}
}

func TestNewWithConfigSignsInWithStoredCredential(t *testing.T) {
	store := secrets.NewMemory()
	_ = store.Set("deezer", secrets.CredentialKey, "saved-token")
	var got provider.Config
	reg := provider.NewRegistry()
	reg.Register("deezer", "Deezer", func(cfg provider.Config) (provider.Provider, error) {
		got = cfg
		return registryProvider{withStream(&fakeLoader{})}, nil
	})
	cfg := config.Default()
	cfg.SetProviderSettings("deezer", config.ProviderSettings{DefaultQuality: "Flac"})

	model := NewWithConfig(cfg, WithRegistry(reg), WithSecrets(store))

	if model.login.active || model.provider == nil {
		t.Fatalf("expected a signed-in session, status %q", model.app.StatusMessage)
	}
	if got.Credential != "saved-token" || got.Settings.DefaultQuality != "Flac" {
		t.Fatalf("factory got %+v", got)
	}
	if err := got.SaveCredential("refreshed"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	if v, _ := store.Get("deezer", secrets.CredentialKey); v != "refreshed" {
		t.Fatalf("SaveCredential stored %q", v)
	}
	if model.configuredQuality() != testQualityFlac {
		t.Fatalf("expected provider settings quality, got %+v", model.configuredQuality())
	}
}

func TestNewMigratesLegacyConfigOnce(t *testing.T) {
	home, path := isolateConfig(t)
	legacy := []byte(`{"theme": "Winamp", "default_quality": "Flac", "arl": "legacy-token"}`)
	writeTestFile(t, filepath.Join(home, ".deezer-tui-config.json"), legacy)
	store := secrets.NewMemory()

	model := New(WithRegistry(testRegistry()), WithSecrets(store))

	if !strings.Contains(model.app.StatusMessage, "Migrated settings from ~/.deezer-tui-config.json") {
		t.Fatalf("expected migration notice, got %q", model.app.StatusMessage)
	}
	if model.login.active {
		t.Fatal("expected the migrated credential to sign in")
	}
	if v, _ := store.Get("deezer", secrets.CredentialKey); v != "legacy-token" {
		t.Fatalf("stored credential = %q", v)
	}
	if data, err := os.ReadFile(path); err != nil || bytes.Contains(data, []byte("legacy-token")) {
		t.Fatalf("new config = %s, err = %v", data, err)
	}
	model.app.StatusMessage = "Loaded 0 playlists"
	model.onCollectionLoaded(collectionLoadedMsg{id: collectionID("home"), title: "Home"})
	if !strings.Contains(model.app.StatusMessage, "Migrated settings") {
		t.Fatalf("expected the notice again after the first section, got %q", model.app.StatusMessage)
	}
	model.onCollectionLoaded(collectionLoadedMsg{id: collectionID("home"), title: "Home"})
	if strings.Contains(model.app.StatusMessage, "Migrated settings") {
		t.Fatal("expected the notice only once")
	}

	again := New(WithRegistry(testRegistry()), WithSecrets(store))
	if strings.Contains(again.app.StatusMessage, "Migrated") {
		t.Fatalf("expected no second migration, got %q", again.app.StatusMessage)
	}
}

type failingSetStore struct{ *secrets.Memory }

func (failingSetStore) Set(string, string, string) error { return errors.New("keychain locked") }

func TestMigrationSecretFailureBlocksWrites(t *testing.T) {
	home, path := isolateConfig(t)
	writeTestFile(t, filepath.Join(home, ".deezer-tui-config.json"), []byte(`{"arl": "legacy-token"}`))

	model := New(WithRegistry(testRegistry()), WithSecrets(failingSetStore{secrets.NewMemory()}))
	if !strings.Contains(model.app.StatusMessage, "keychain locked") {
		t.Fatalf("expected the store error in status, got %q", model.app.StatusMessage)
	}
	changeThemeSetting(&model)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no new config so migration retries, stat err = %v", err)
	}
}

func TestBrokenLegacyConfigIsNotMigrated(t *testing.T) {
	home, path := isolateConfig(t)
	legacyPath := filepath.Join(home, ".deezer-tui-config.json")
	broken := []byte(`{"arl": "secret",`)
	writeTestFile(t, legacyPath, broken)

	model := New(WithRegistry(testRegistry()), WithSecrets(secrets.NewMemory()))
	if !strings.Contains(model.app.StatusMessage, "Config error") {
		t.Fatalf("expected config error, got %q", model.app.StatusMessage)
	}
	changeThemeSetting(&model)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no new config, stat err = %v", err)
	}
	if got, _ := os.ReadFile(legacyPath); !bytes.Equal(got, broken) {
		t.Fatalf("legacy file changed: %q", got)
	}
}
