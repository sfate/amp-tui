package tui

import (
	"context"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

// resolvingProvider records Resolve calls, so a test sees which provider stream a Track is bound to.
type resolvingProvider struct {
	registryProvider
	resolved []string
}

func (p *resolvingProvider) Resolve(_ context.Context, trackID string, q core.Quality) (provider.Source, error) {
	p.resolved = append(p.resolved, trackID+"@"+q.ID)
	return provider.Source{Info: core.TrackInfo{ID: trackID}, Quality: q}, nil
}

func TestPlaybackStartsThroughInjectedEngine(t *testing.T) {
	isolateConfig(t)
	prov := &resolvingProvider{registryProvider: registryProvider{withStream(&fakeLoader{})}}
	engine := newFakeEngine()
	model := NewWithProviderAndEngine(config.Default(), prov, engine)

	cmd := model.startTrackPlaybackAt("track-9", testQuality320, 7_000, true)
	if cmd == nil {
		t.Fatal("expected a playback command")
	}
	_, _ = model.Update(firstBatchMessage(cmd))

	starts := engine.Starts()
	if len(starts) != 1 {
		t.Fatalf("engine Start calls = %d, want 1", len(starts))
	}
	call := starts[0]
	if call.Source.Info.ID != "track-9" || call.Source.Quality != testQuality320 || call.Opts.StartMS != 7_000 {
		t.Fatalf("Start got source %q at %q from %dms", call.Source.Info.ID, call.Source.Quality.ID, call.Opts.StartMS)
	}
	if !slices.Equal(prov.resolved, []string{"track-9@" + testQuality320.ID}) {
		t.Fatalf("provider resolved %v, want the started track at its quality", prov.resolved)
	}
}

func seekingModel(engine *fakeEngine) Model {
	model := NewWithProviderAndEngine(config.Default(), withStream(&fakeLoader{}), engine)
	model.ready = true
	model.width = 120
	model.height = 40
	model.currentTrackID = "track-1"
	model.currentQuality = testQuality320
	model.app.IsPlaying = true
	model.app.NowPlaying = &app.NowPlaying{ID: "track-1", Quality: testQuality320, CurrentMS: 30_000, TotalMS: 120_000}
	return model
}

func TestSeekWithoutEngineSeekRestartsAtOffset(t *testing.T) {
	engine := newFakeEngine()
	model := seekingModel(engine)
	session, err := engine.Start(context.Background(), provider.Source{}, playback.StartOptions{}, playback.EventHandler{})
	if err != nil {
		t.Fatal(err)
	}
	model.session = session

	next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "."}))
	if cmd == nil {
		t.Fatal("expected a restart command")
	}
	_, _ = next.(Model).Update(firstBatchMessage(cmd))

	if got := engine.seeked(); !slices.Equal(got, []uint64{0, 40_000}) {
		t.Fatalf("Start offsets = %v, want a restart at 40000ms", got)
	}
	if seeks := engine.Sessions()[0].Seeks(); len(seeks) != 0 {
		t.Fatalf("Session.Seek calls = %v, want none", seeks)
	}
}

func TestSeekWithEngineSeekMovesSession(t *testing.T) {
	engine := newFakeEngine()
	engine.Caps.Seek = true
	model := seekingModel(engine)
	session, err := engine.Start(context.Background(), provider.Source{}, playback.StartOptions{}, playback.EventHandler{})
	if err != nil {
		t.Fatal(err)
	}
	model.session = session

	next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "."}))
	updated := next.(Model)
	if cmd != nil {
		t.Fatal("expected no restart when the engine seeks in-session")
	}
	if seeks := engine.Sessions()[0].Seeks(); !slices.Equal(seeks, []uint64{40_000}) {
		t.Fatalf("Session.Seek calls = %v, want [40000]", seeks)
	}
	if len(engine.Starts()) != 1 {
		t.Fatalf("Start calls = %d, want only the first", len(engine.Starts()))
	}
	if updated.app.NowPlaying.CurrentMS != 40_000 || updated.progress.baseMS != 40_000 {
		t.Fatalf("position = %d (base %d), want 40000", updated.app.NowPlaying.CurrentMS, updated.progress.baseMS)
	}
}

func TestEngineSettingSwitchesInjectedEngineAndStopsPlayback(t *testing.T) {
	isolateConfig(t)
	engine := newFakeEngine()
	model := NewWithProviderAndEngine(config.Default(), withStream(&fakeLoader{}), engine)
	var saved config.Config
	model.saveConfig = func(cfg config.Config) error { saved = cfg; return nil }
	var selected []config.PlaybackEngine
	model.engineFactory = func(choice config.PlaybackEngine, _ provider.ID) playback.Engine {
		selected = append(selected, choice)
		return newFakeEngine()
	}
	session, err := engine.Start(context.Background(), provider.Source{}, playback.StartOptions{}, playback.EventHandler{})
	if err != nil {
		t.Fatal(err)
	}
	model.session = session
	model.app.IsPlaying = true
	model.app.NowPlaying = &app.NowPlaying{ID: "playing"}
	model.app.SettingsState.Select(intPtr(slices.Index(model.settingsRows(), settingEngine)))

	model.adjustSelectedSetting(1)
	if model.app.Config.Engine != config.EngineMPV || saved.Engine != config.EngineMPV {
		t.Fatalf("engine config = %q, saved = %q; want mpv", model.app.Config.Engine, saved.Engine)
	}
	if !slices.Equal(selected, []config.PlaybackEngine{config.EngineMPV}) {
		t.Fatalf("engine factory calls = %v, want [mpv]", selected)
	}
	if !engine.Sessions()[0].Stopped() || model.session != nil || model.app.IsPlaying || model.app.NowPlaying != nil {
		t.Fatal("changing engines must stop and clear the active session")
	}
}
