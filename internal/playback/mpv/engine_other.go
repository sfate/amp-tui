//go:build !darwin && !linux

package mpv

import (
	"context"
	"errors"

	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

func startSession(context.Context, string, provider.Source, playback.StartOptions, playback.EventHandler) (playback.Session, error) {
	return nil, errors.New("mpv playback is supported on macOS and Linux")
}
