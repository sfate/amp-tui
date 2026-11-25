package applemusic

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

func TestMusicEngineControlsExternalPlaybackAndStopsWithoutAudioSource(t *testing.T) {
	state, _ := json.Marshal(playbackState{State: "playing", Track: trackRecord{ID: "TRK789", Title: "One", Artist: "A", Duration: 12.5}, Position: 1.5})
	runner := &fakeScriptRunner{fn: func(_ context.Context, operation string, _ ...string) ([]byte, error) {
		if operation == "state" {
			return state, nil
		}
		return []byte("ok"), nil
	}}
	engine := newEngine(runner, 10*time.Millisecond)
	quality := core.Quality{ID: "music-app", Label: "Music.app", Seekable: true}
	source := provider.Source{Info: core.TrackInfo{ID: "TRK789", Title: "One", DurationMS: 12_500}, Quality: quality, Seekable: true}
	changed := make(chan core.TrackInfo, 1)
	session, err := engine.Start(context.Background(), source, playback.StartOptions{StartMS: 1_500}, playback.EventHandler{
		OnTrackChanged: func(info core.TrackInfo, _ core.Quality, _ uint64) { changed <- info },
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case info := <-changed:
		if info.ID != "TRK789" || info.Title != "One" {
			t.Fatalf("track changed info = %+v", info)
		}
	case <-time.After(time.Second):
		t.Fatal("Start did not report the selected track")
	}
	session.Pause()
	session.Resume()
	session.SetVolume(0.4)
	if err := session.Seek(4_000); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	session.Stop()
	if err := session.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait after Stop = %v, want context.Canceled", err)
	}
	ops := make([]string, 0, len(runner.calls))
	for _, call := range runner.calls {
		ops = append(ops, call.operation)
	}
	for _, op := range []string{"play", "pause", "resume", "volume", "seek", "stop"} {
		if !slices.Contains(ops, op) {
			t.Errorf("Music.app operations %v missing %q", ops, op)
		}
	}
	if slices.Contains(ops, "audio") || slices.Contains(ops, "resolve") {
		t.Fatalf("remote playback unexpectedly requested audio bytes: %v", ops)
	}
}

func TestMusicEngineNaturalEndAndSingleTrackDoesNotAutoplay(t *testing.T) {
	state, _ := json.Marshal(playbackState{State: "stopped", Track: trackRecord{ID: "TRK789", Title: "One", Duration: 12}, Position: 12})
	runner := &fakeScriptRunner{fn: func(_ context.Context, operation string, _ ...string) ([]byte, error) {
		if operation == "state" {
			return state, nil
		}
		return []byte("ok"), nil
	}}
	engine := newEngine(runner, 5*time.Millisecond)
	session, err := engine.Start(context.Background(), provider.Source{
		Info:    core.TrackInfo{ID: "TRK789", Title: "One", DurationMS: 12_000},
		Quality: core.Quality{ID: "music-app", Seekable: true}, Seekable: true,
	}, playback.StartOptions{}, playback.EventHandler{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case err := <-waitSession(session):
		if err != nil {
			t.Fatalf("natural Wait = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("session did not finish after Music.app stopped at the track end")
	}
	if len(runner.calls) == 0 || runner.calls[0].operation != "play" || !strings.Contains(scripts["play"].source, "{once: true}") {
		t.Fatalf("play call = %+v, script does not request single-track playback", runner.calls)
	}
}

func waitSession(session playback.Session) <-chan error {
	result := make(chan error, 1)
	go func() { result <- session.Wait() }()
	return result
}
