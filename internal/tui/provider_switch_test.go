package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback/playbacktest"
	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/secrets"
)

// switchRegistry holds the auth fake with a quality ladder and plainProvider without auth or ladder.
func switchRegistry() *provider.Registry {
	reg := testRegistry()
	reg.Register("plain", "Plain", func(provider.Config) (provider.Provider, error) {
		return bareProvider(), nil
	})
	return reg
}

func anonymousRegistry() *provider.Registry {
	reg := switchRegistry()
	reg.Register("anonymous", "Anonymous", func(provider.Config) (provider.Provider, error) {
		return anonymousAuthProvider{fakeLoader: &fakeLoader{}}, nil
	})
	return reg
}

type anonymousAuthProvider struct{ *fakeLoader }

func (anonymousAuthProvider) ID() provider.ID { return "anonymous" }

func (anonymousAuthProvider) Name() string { return "Anonymous" }

func (anonymousAuthProvider) SupportsAnonymousAccess() bool { return true }

func signedInStore(t *testing.T) secrets.Store {
	t.Helper()
	store := secrets.NewMemory()
	if err := store.Set("deezer", secrets.CredentialKey, "saved-token"); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	return store
}

func selectSettingsRow(t *testing.T, model *Model, id settingsRowID) {
	t.Helper()
	openSettings(model)
	idx := slices.Index(model.settingsRows(), id)
	if idx < 0 {
		t.Fatalf("row %q not in %v", id, model.settingsRows())
	}
	model.app.SettingsState.Select(intPtr(idx))
}

func pressKey(model Model, key string) (Model, tea.Cmd) {
	next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: key}))
	return next.(Model), cmd
}

func TestProviderSwitchRebootstrapsAndClearsQueue(t *testing.T) {
	isolateConfig(t)
	model := NewWithConfig(config.Default(), WithRegistry(switchRegistry()), WithSecrets(signedInStore(t)))
	var saved []config.Config
	model.saveConfig = func(cfg config.Config) error {
		saved = append(saved, cfg)
		return nil
	}
	session := playbacktest.NewSession()
	art := "https://example.test/cover.jpg"
	model.ready = true
	model.session = session
	model.currentTrackID = "1"
	model.app.IsPlaying = true
	model.app.NowPlaying = &app.NowPlaying{ID: "1", Title: "Song"}
	model.app.QueueTracks = []app.Track{{ID: "1"}, {ID: "2"}}
	model.app.QueueIndex = intPtr(0)
	model.app.CurrentTracks = []app.Track{{ID: "1"}}
	model.app.Playlists = []app.Playlist{{ID: "p", Title: "Mix"}}
	model.app.SearchQuery = "song"
	model.app.ShowingSearchResult = true
	model.app.SearchArtists = []app.Artist{{ID: "a", Name: "Artist"}}
	model.setPrebufferStatus("2", core.Quality{ID: "Kbps320"}, PrebufferStatusReady)
	model.artwork.url = art
	model.rememberArtwork(art, "ART")

	selectSettingsRow(t, &model, settingProvider)
	model, cmd := pressKey(model, "l")

	if model.providerID != "plain" || model.provider == nil || model.provider.ID() != "plain" {
		t.Fatalf("active provider = %q, %v", model.providerID, model.provider)
	}
	if !session.Stopped() {
		t.Fatal("expected the playing session to stop")
	}
	if len(model.app.QueueTracks) != 0 || model.app.QueueIndex != nil || model.app.NowPlaying != nil || model.app.IsPlaying || model.currentTrackID != "" {
		t.Fatalf("expected an empty queue and no now-playing, got queue %v index %v now %v", model.app.QueueTracks, model.app.QueueIndex, model.app.NowPlaying)
	}
	if len(model.prebuffer.statuses) != 0 || len(model.prebuffer.ready) != 0 {
		t.Fatalf("expected a clear prebuffer, got %v", model.prebuffer.statuses)
	}
	if len(model.app.CurrentTracks) != 0 || len(model.app.Playlists) != 0 || model.app.ShowingSearchResult || model.app.SearchQuery != "" || len(model.app.SearchArtists) != 0 {
		t.Fatal("expected library and search results of the old provider to be cleared")
	}
	if len(model.artwork.cache) != 0 || model.artwork.url != "" {
		t.Fatal("expected the artwork cache to be cleared")
	}
	if len(saved) != 1 || saved[0].Provider != "plain" {
		t.Fatalf("expected one save with the new provider, got %+v", saved)
	}
	if model.login.active || model.ready {
		t.Fatalf("expected the loading screen, login %v ready %v", model.login.active, model.ready)
	}
	if _, ok := firstNonTickMsg(cmd).(bootstrapLoadedMsg); !ok {
		t.Fatal("expected the switch to bootstrap the new provider")
	}
	if !slices.Equal(model.app.NavItems, []string{"Library", "Settings"}) {
		t.Fatalf("nav items = %v, want the new provider sections", model.app.NavItems)
	}
}

func TestSwitchToProviderWithoutAuthHidesAccountAndNeverLogsIn(t *testing.T) {
	isolateConfig(t)
	cfg := config.Default()
	cfg.Provider = "plain"
	model := NewWithConfig(cfg, WithRegistry(switchRegistry()), WithSecrets(secrets.NewMemory()))
	if model.login.active || model.provider == nil {
		t.Fatalf("expected a provider without auth to start signed in, status %q", model.app.StatusMessage)
	}
	if slices.Contains(model.settingsRows(), settingAccount) {
		t.Fatalf("rows = %v, want no Account row", model.settingsRows())
	}
	model.ready = true
	model, _ = pressKey(model, "O")
	if model.login.active {
		t.Fatal("expected O to keep the login screen closed")
	}
}

func TestSwitchToAuthProviderWithoutCredentialOpensLogin(t *testing.T) {
	isolateConfig(t)
	cfg := config.Default()
	cfg.Provider = "plain"
	model := NewWithConfig(cfg, WithRegistry(switchRegistry()), WithSecrets(secrets.NewMemory()))
	model.saveConfig = func(config.Config) error { return nil }
	model.ready = true

	selectSettingsRow(t, &model, settingProvider)
	model, cmd := pressKey(model, "l")

	if model.providerID != "deezer" || !model.login.active || model.provider != nil {
		t.Fatalf("expected the login screen for deezer, got id %q login %v", model.providerID, model.login.active)
	}
	if cmd != nil {
		t.Fatal("expected no bootstrap before login")
	}
}

func TestAuthProviderCanStartWithoutCredentialWhenAnonymousAccessIsSupported(t *testing.T) {
	isolateConfig(t)
	cfg := config.Default()
	cfg.Provider = "anonymous"
	model := NewWithConfig(cfg, WithRegistry(anonymousRegistry()), WithSecrets(secrets.NewMemory()))

	if model.provider == nil || model.login.active || model.ready {
		t.Fatalf("expected anonymous provider bootstrap, provider %v login %v ready %v", model.provider, model.login.active, model.ready)
	}
	if model.catalog.auth == nil {
		t.Fatal("expected optional authenticator metadata to remain available")
	}
}

func TestLogoutDeletesCredentialAndOpensLogin(t *testing.T) {
	isolateConfig(t)
	store := signedInStore(t)
	model := NewWithConfig(config.Default(), WithRegistry(switchRegistry()), WithSecrets(store))
	model.ready = true
	selectSettingsRow(t, &model, settingAccount)
	if _, value, _ := model.settingsRowView(settingAccount); value != "Signed in" {
		t.Fatalf("account row = %q, want Signed in", value)
	}

	model, _ = pressKey(model, "enter")

	if _, err := store.Get("deezer", secrets.CredentialKey); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("expected the credential to be deleted, err = %v", err)
	}
	if !model.login.active || model.provider != nil {
		t.Fatal("expected logout to open the login screen")
	}
}

func TestUnknownConfigProviderFallsBackWithMessage(t *testing.T) {
	isolateConfig(t)
	cfg := config.Default()
	cfg.Provider = "tidal"
	model := NewWithConfig(cfg, WithRegistry(switchRegistry()), WithSecrets(signedInStore(t)))

	if model.providerID != "deezer" || model.provider == nil {
		t.Fatalf("expected the registry default, got %q", model.providerID)
	}
	if !strings.Contains(model.app.StatusMessage, `Unknown provider "tidal"`) {
		t.Fatalf("expected a fallback message, got %q", model.app.StatusMessage)
	}
}

func TestProviderOverrideIsNotSaved(t *testing.T) {
	_, path := isolateConfig(t)
	writeTestFile(t, path, []byte(`{"schema_version": 2, "provider": "deezer"}`))

	model := New(WithRegistry(switchRegistry()), WithSecrets(signedInStore(t)), WithProviderOverride("plain"))
	if model.providerID != "plain" {
		t.Fatalf("expected the override provider, got %q", model.providerID)
	}
	changeThemeSetting(&model)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), `"provider": "deezer"`) {
		t.Fatalf("expected the saved provider to stay deezer: %s", data)
	}
}

func TestSettingsRowOrder(t *testing.T) {
	isolateConfig(t)
	model := NewWithConfig(config.Default(), WithRegistry(switchRegistry()), WithSecrets(signedInStore(t)))
	want := []settingsRowID{settingProvider, settingAccount, settingEngine, settingTheme, settingVolume, settingCrossfade, settingDuration, settingDisplay, settingCompactView, settingQuality}
	if got := model.settingsRows(); !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if label, value, _ := model.settingsRowView(settingProvider); label != "Provider" || value != "Deezer" {
		t.Fatalf("provider row = %q %q", label, value)
	}
	if model.app.SettingsItemCount != len(want) {
		t.Fatalf("settings item count = %d, want %d", model.app.SettingsItemCount, len(want))
	}

	cfg := config.Default()
	cfg.Provider = "plain"
	plain := NewWithConfig(cfg, WithRegistry(switchRegistry()), WithSecrets(secrets.NewMemory()))
	want = []settingsRowID{settingProvider, settingEngine, settingTheme, settingVolume, settingCrossfade, settingDuration, settingDisplay, settingCompactView}
	if got := plain.settingsRows(); !slices.Equal(got, want) {
		t.Fatalf("plain rows = %v, want %v", got, want)
	}
}

// stepsProvider declares its own login texts, so the view must show them as given.
type stepsProvider struct{ *plainProvider }

func (stepsProvider) LoginSteps() []provider.LoginStep {
	return []provider.LoginStep{
		{ID: "token", Kind: provider.LoginStepInput, Prompt: "Paste the Acme API token", Masked: true},
		{ID: "web", Kind: provider.LoginStepBrowser, Prompt: "Press Enter to open Acme in the browser", WaitText: "Waiting for Acme"},
	}
}

func (stepsProvider) Submit(_ context.Context, stepID, input string) (string, error) {
	return stepID + ":" + input, nil
}

func TestLoginViewShowsProviderStepPrompt(t *testing.T) {
	model := NewWithProvider(config.Default(), stepsProvider{bareProvider()})
	model.width = 120
	model.height = 40
	model.ready = true
	model, _ = pressKey(model, "O")

	view := model.View().Content
	if !strings.Contains(view, "Paste the Acme API token") {
		t.Fatalf("expected the first step prompt in the view:\n%s", view)
	}
	model, _ = pressKey(model, "secret-value")
	view = model.View().Content
	if strings.Contains(view, "secret-value") {
		t.Fatal("expected a masked step to hide the input")
	}
	model, _ = pressKey(model, "tab")
	model.login.input = ""
	if view = model.View().Content; !strings.Contains(view, "Press Enter to open Acme in the browser") {
		t.Fatalf("expected Tab to show the browser step:\n%s", view)
	}
	for _, word := range []string{"ARL", "Deezer", "Cookie"} {
		if strings.Contains(view, word) {
			t.Fatalf("login view holds %q:\n%s", word, view)
		}
	}
}

func TestTUISourceHasNoDeezerWords(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`(?i)\bdeezer\b|\bARL\b`)
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if loc := pattern.FindIndex(data); loc != nil {
			t.Errorf("%s holds provider word %q", name, data[loc[0]:loc[1]])
		}
	}
}

func TestLateLoadFromReplacedProviderIsDropped(t *testing.T) {
	isolateConfig(t)
	model := NewWithConfig(config.Default(), WithRegistry(switchRegistry()), WithSecrets(signedInStore(t)))
	model.saveConfig = func(config.Config) error { return nil }
	selectSettingsRow(t, &model, settingProvider)
	model, _ = pressKey(model, "l")

	next, _ := model.Update(bootstrapLoadedMsg{source: "deezer", playlists: []app.Playlist{{ID: "old"}}})
	model = next.(Model)
	next, _ = model.Update(collectionLoadedMsg{source: "deezer", id: collectionID("home"), title: "Home", tracks: []app.Track{{ID: "old"}}})
	model = next.(Model)
	if len(model.app.Playlists) != 0 || len(model.app.CurrentTracks) != 0 {
		t.Fatalf("expected late deezer results to be dropped, got %v %v", model.app.Playlists, model.app.CurrentTracks)
	}
}

func TestEnterOnProviderRowDoesNotSwitch(t *testing.T) {
	isolateConfig(t)
	model := NewWithConfig(config.Default(), WithRegistry(switchRegistry()), WithSecrets(signedInStore(t)))
	model.saveConfig = func(config.Config) error { return nil }
	model.ready = true
	selectSettingsRow(t, &model, settingProvider)

	next, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = next.(Model)

	if model.providerID != "deezer" {
		t.Fatalf("Enter switched provider to %q; only left/right may switch", model.providerID)
	}
	if !strings.Contains(model.app.StatusMessage, "←/→") {
		t.Fatalf("status %q, want a hint to use left/right", model.app.StatusMessage)
	}
}
