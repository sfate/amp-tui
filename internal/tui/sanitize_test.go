package tui

import (
	"testing"

	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/core"
)

const hostileTitle = "Song\x1b[2J\x1b]0;pwned\x07\nB-side"

func TestTrackChangedSanitizesNowPlaying(t *testing.T) {
	model := NewWithProvider(config.Default(), &fakeLoader{})
	model.currentPlayID = 3

	next, _ := model.Update(playbackTrackChangedMsg{
		playID:  3,
		info:    core.TrackInfo{ID: "1", Title: hostileTitle, Artist: "\x1b]8;;x\x1b\\Artist"},
		quality: testQuality320,
	})
	now := next.(Model).app.NowPlaying
	if now == nil || now.Title != "Song B-side" || now.Artist != "Artist" {
		t.Fatalf("now playing not sanitized: %+v", now)
	}
}
