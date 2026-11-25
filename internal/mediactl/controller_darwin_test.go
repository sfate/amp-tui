//go:build darwin

package mediactl

import (
	"strings"
	"testing"

	"github.com/sfate/amp-tui/internal/playback"
)

func TestParseRemoteCommands(t *testing.T) {
	tests := []struct {
		line string
		want playback.MediaCommand
		ok   bool
	}{
		{"remote\tplay", playback.MediaCommand{Kind: playback.MediaPlay}, true},
		{"remote\tpause", playback.MediaCommand{Kind: playback.MediaPause}, true},
		{"remote\ttoggle", playback.MediaCommand{Kind: playback.MediaToggle}, true},
		{"remote\tnext", playback.MediaCommand{Kind: playback.MediaNext}, true},
		{"remote\tprevious", playback.MediaCommand{Kind: playback.MediaPrevious}, true},
		{"remote\tsetPosition\t12345", playback.MediaCommand{Kind: playback.MediaSetPosition, PositionMS: 12345}, true},
		{"remote\tsetPosition\tbad", playback.MediaCommand{}, false},
		{"remote\tunknown", playback.MediaCommand{}, false},
	}
	for _, test := range tests {
		t.Run(test.line, func(t *testing.T) {
			parts := strings.Split(test.line, "\t")
			got, ok := parseRemote(parts)
			if ok != test.ok || got != test.want {
				t.Fatalf("parseRemote(%q) = (%+v, %t), want (%+v, %t)", test.line, got, ok, test.want, test.ok)
			}
		})
	}
}
