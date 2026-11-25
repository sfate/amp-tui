package mediactl

import "github.com/sfate/amp-tui/internal/playback"

// Controller connects OS Now Playing and media-key events to the TUI.
type Controller interface {
	MediaCommands() <-chan playback.MediaCommand
	UpdateMediaState(playback.MediaState)
	Close()
}

// New returns the platform media controller. On unsupported platforms its command
// channel is nil and state updates are ignored.
func New() Controller { return newController() }
