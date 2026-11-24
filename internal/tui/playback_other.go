//go:build !darwin

package tui

import (
	"context"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/player"
	"github.com/sfate/amp-tui/internal/provider"
)

type defaultPlayerRuntime struct {
	streams provider.StreamSource
	backend player.Backend
}

func newPlayerRuntime(p provider.Provider) PlayerRuntime {
	streams, ok := p.(provider.StreamSource)
	if !ok {
		return nil
	}
	return &defaultPlayerRuntime{
		streams: streams,
		backend: player.NewProcessBackend(),
	}
}

func (r *defaultPlayerRuntime) Start(trackID string, quality core.Quality, seekMS uint64, handler player.EventHandler) (PlaybackSession, error) {
	session := player.StartTrackPipeline(
		context.Background(),
		r.streams,
		r.backend,
		trackID,
		quality,
		seekMS,
		handler,
		player.PipelineOptions{},
	)
	return session, nil
}
