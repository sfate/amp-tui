package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/provider"
)

const deezerControls = " space play/pause | n/p next prev | ,/. seek | u/i quality | r repeat | +/- volume "

func bareProvider() *plainProvider {
	return &plainProvider{sections: []provider.Section{{ID: "library", Label: "Library", Kind: provider.SectionTracks}}}
}

func playingModel(p provider.Provider) Model {
	model := NewWithProviderAndRuntime(config.Default(), p, &fakePlaybackRuntime{})
	model.ready = true
	model.width = 120
	model.height = 40
	model.currentTrackID = "track-1"
	model.currentQuality = testQuality128
	model.app.NowPlaying = &app.NowPlaying{ID: "track-1", Title: "Song", Artist: "Artist", Quality: testQuality128, TotalMS: 120_000}
	return model
}

func openSettings(model *Model) {
	model.app.ViewingSettings = true
	model.app.ActivePanel = app.ActivePanelMain
	model.app.SettingsState.Select(intPtr(0))
}

func TestBareProviderHidesFlowAndControls(t *testing.T) {
	model := playingModel(bareProvider())
	if slices.Contains(model.app.NavItems, "Flow") {
		t.Fatalf("nav items = %v, want no Flow", model.app.NavItems)
	}
	controls := model.controlsHint()
	if strings.Contains(controls, "seek") || strings.Contains(controls, "quality") {
		t.Fatalf("controls = %q, want no seek and no quality", controls)
	}
	if strings.Contains(model.renderPlaybar(), "seek") {
		t.Fatal("expected the playbar to omit seek")
	}
}

func TestBareProviderKeysAnswerNotAvailable(t *testing.T) {
	for _, key := range []string{",", ".", "u", "i", "s"} {
		model := playingModel(bareProvider())
		next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: key}))
		updated := next.(Model)
		if cmd != nil {
			t.Fatalf("key %q: expected no command", key)
		}
		if updated.app.StatusMessage != "Not available for Plain" {
			t.Fatalf("key %q: status = %q", key, updated.app.StatusMessage)
		}
	}
}

func TestBareProviderSettingsHaveNoQualityRow(t *testing.T) {
	model := NewWithProvider(config.Default(), bareProvider())
	openSettings(&model)
	want := []settingsRowID{settingTheme, settingVolume, settingCrossfade, settingDuration, settingDisplay, settingCompactView}
	if got := model.settingsRows(); !slices.Equal(got, want) {
		t.Fatalf("settings rows = %v, want %v", got, want)
	}
	for _, row := range model.renderSettingsRows() {
		if strings.Contains(row, "Quality") {
			t.Fatalf("unexpected quality row %q", row)
		}
	}
	for range 10 {
		model.app.HandleDown()
	}
	if got := derefOrZero(model.app.SettingsState.Selected()); got != len(want)-1 {
		t.Fatalf("HandleDown stopped at %d, want %d", got, len(want)-1)
	}
}

func TestBareProviderSettingsRowsStillAdjust(t *testing.T) {
	for i := range 6 {
		model := NewWithProvider(config.Default(), bareProvider())
		model.saveConfig = func(config.Config) error { return nil }
		openSettings(&model)
		model.app.SettingsState.Select(intPtr(i))
		before := fmt.Sprintf("%+v %d", model.app.Config, model.app.Volume)
		next, _ := model.Update(tea.KeyPressMsg(tea.Key{Text: "h"}))
		updated := next.(Model)
		if after := fmt.Sprintf("%+v %d", updated.app.Config, updated.app.Volume); after == before {
			t.Fatalf("row %d (%v): expected a change", i, model.settingsRows()[i])
		}
	}
}

func TestFullProviderKeepsDeezerControlsAndSettings(t *testing.T) {
	model := playingModel(withStream(&fakeLoader{}))
	if got := model.controlsHint(); got != deezerControls {
		t.Fatalf("controls = %q, want %q", got, deezerControls)
	}
	model.loadCollection(collectionID("favorites"), "Favorites", []app.Track{{ID: "1"}})
	wantFavorites := " space play/pause | n/p next prev | ,/. seek | u/i quality | r repeat | s sort | +/- volume "
	if got := model.controlsHint(); got != wantFavorites {
		t.Fatalf("favorites controls = %q, want %q", got, wantFavorites)
	}

	openSettings(&model)
	rows := model.settingsRows()
	if len(rows) != 7 || rows[2] != settingQuality {
		t.Fatalf("settings rows = %v, want 7 with quality third", rows)
	}
	for range 10 {
		model.app.HandleDown()
	}
	if got := derefOrZero(model.app.SettingsState.Selected()); got != 6 {
		t.Fatalf("HandleDown stopped at %d, want 6", got)
	}
}

func TestFullProviderStepsQualityAndKeepsSeekMessage(t *testing.T) {
	model := playingModel(withStream(&fakeLoader{}))
	next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "i"}))
	updated := next.(Model)
	if cmd == nil || updated.currentQuality != testQuality320 {
		t.Fatalf("expected i to step up to 320, got %+v", updated.currentQuality)
	}
	updated.app.NowPlaying = &app.NowPlaying{ID: "track-1", Quality: testQuality320, TotalMS: 120_000}
	next, cmd = updated.Update(tea.KeyPressMsg(tea.Key{Text: "u"}))
	updated = next.(Model)
	if cmd == nil || updated.currentQuality != testQuality128 {
		t.Fatalf("expected u to step down to 128, got %+v", updated.currentQuality)
	}

	model = playingModel(withStream(&fakeLoader{}))
	model.currentQuality = testQualityFlac
	next, cmd = model.Update(tea.KeyPressMsg(tea.Key{Text: "."}))
	updated = next.(Model)
	if cmd != nil || updated.app.StatusMessage != "Seeking is not supported for FLAC playback" {
		t.Fatalf("unexpected seek status %q", updated.app.StatusMessage)
	}
}

func TestLoginSuccessRecomputesCapabilities(t *testing.T) {
	model := NewWithProvider(config.Default(), bareProvider())
	model.saveConfig = func(config.Config) error { return nil }
	model.onLoginSucceeded(loginSucceededMsg{secret: "token", provider: withStream(&fakeLoader{})})
	if !model.caps.QualityLadder || len(model.settingsRows()) != 7 {
		t.Fatalf("caps = %+v, rows = %v; want ladder and 7 rows", model.caps, model.settingsRows())
	}
}
