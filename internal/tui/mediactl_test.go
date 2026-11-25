package tui

import (
	"testing"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/playback"
)

func TestMediaControllerCanSetVolume(t *testing.T) {
	model := NewWithProvider(config.Default(), &fakeLoader{})
	model.handleMediaControlCommand(playback.MediaCommand{Kind: playback.MediaSetVolume, Volume: 0.758})
	if model.app.Volume != 76 {
		t.Fatalf("volume = %d, want 76", model.app.Volume)
	}
}

func TestMediaControllerCanSetRepeatMode(t *testing.T) {
	model := NewWithProvider(config.Default(), &fakeLoader{})
	model.handleMediaControlCommand(playback.MediaCommand{Kind: playback.MediaSetLoopStatus, LoopStatus: "Playlist"})
	if model.app.RepeatMode != app.RepeatModeAll {
		t.Fatalf("repeat mode = %v, want repeat all", model.app.RepeatMode)
	}
}
