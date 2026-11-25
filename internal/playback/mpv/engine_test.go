//go:build darwin || linux

package mpv

import (
	"slices"
	"testing"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

func TestMPVArgumentsKeepPlaybackHeadlessAndIsolated(t *testing.T) {
	args := mpvArguments("/tmp/private/mpv.sock")
	for _, want := range []string{
		"--idle=yes",
		"--no-video",
		"--no-config",
		"--no-terminal",
		"--input-media-keys=no",
		"--demuxer-lavf-o=fflags=+fastseek",
		"--ytdl=no",
		"--input-ipc-server=/tmp/private/mpv.sock",
		"--af=@astats:lavfi=[astats=metadata=1:reset=1]",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("mpv arguments %v do not include %q", args, want)
		}
	}
}

func TestPlaybackRestartReportsTrackOnce(t *testing.T) {
	var changes int
	var gotMS uint64
	session := &mpvSession{
		source:  provider.Source{Info: core.TrackInfo{ID: "one"}, Quality: core.Quality{ID: "q"}},
		startMS: 2_000, lastMS: 2_000,
		handler: playback.EventHandler{OnTrackChanged: func(_ core.TrackInfo, _ core.Quality, ms uint64) { changes++; gotMS = ms }},
	}
	_, _ = session.handleMessage(ipcMessage{Event: "playback-restart"})
	_, _ = session.handleMessage(ipcMessage{Event: "playback-restart"})
	if changes != 1 || gotMS != 2_000 {
		t.Fatalf("track change count/position = %d/%d, want 1/2000", changes, gotMS)
	}
}

func TestMPVPropertiesReportProgressAndVisualizer(t *testing.T) {
	var gotPosition, gotDuration uint64
	var gotBands []uint8
	session := &mpvSession{handler: playback.EventHandler{
		OnPlaybackProgress: func(position, duration uint64) { gotPosition, gotDuration = position, duration },
		OnAudioBands:       func(bands []uint8) { gotBands = bands },
	}}
	session.handleProperty("time-pos", 1.25)
	session.handleProperty("duration", 45.0)
	session.handleProperty("af-metadata/astats", map[string]any{"lavfi.astats.Overall.RMS_level": "-30.0"})
	if gotPosition != 1_250 || gotDuration != 45_000 {
		t.Fatalf("progress = %d/%d, want 1250/45000", gotPosition, gotDuration)
	}
	if len(gotBands) != 8 || gotBands[0] < 126 || gotBands[0] > 129 {
		t.Fatalf("visualizer bands = %v, want eight levels near 128", gotBands)
	}
}
