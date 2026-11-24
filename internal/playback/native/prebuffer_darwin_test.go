//go:build darwin

package native

import (
	"context"
	"testing"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

func TestDarwinPrebufferTakeKeepsCachedTrackReusable(t *testing.T) {
	store := newDarwinPrebufferStore()
	track := playback.Track{ID: "1", Quality: testQuality320}
	key := darwinPrebufferKey(track)
	store.current[key] = &darwinPreparedTrack{
		TrackID: "1",
		Quality: testQuality320,
		Info:    core.TrackInfo{ID: "1", Title: "One", Artist: "A"},
		Format:  core.FormatMP3,
		File:    "/tmp/amp-tui-test.mp3",
	}
	store.order = []string{key}

	source := provider.Source{Info: core.TrackInfo{ID: track.ID}, Quality: track.Quality}
	prepared, err := store.TakeOrPrepareSource(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("take cached track: %v", err)
	}
	if !prepared.Cached {
		t.Fatal("expected cached prepared track")
	}
	if _, ok := store.current[key]; !ok {
		t.Fatal("expected cached track to remain reusable after take")
	}
}

func TestDarwinPrebufferEmptyWindowKeepsCachedTracks(t *testing.T) {
	store := newDarwinPrebufferStore()
	track := playback.Track{ID: "1", Quality: testQuality320}
	key := darwinPrebufferKey(track)
	store.current[key] = &darwinPreparedTrack{
		TrackID: "1",
		Quality: testQuality320,
		Info:    core.TrackInfo{ID: "1", Title: "One", Artist: "A"},
		Format:  core.FormatMP3,
		File:    "/tmp/amp-tui-test.mp3",
		Cached:  true,
	}
	store.order = []string{key}

	store.Prebuffer(context.Background(), nil, nil)
	if _, ok := store.current[key]; !ok {
		t.Fatal("expected cached track to survive empty prebuffer window")
	}
}

var testQuality320 = core.Quality{ID: "Kbps320", Label: "320 kbps", Seekable: true}
