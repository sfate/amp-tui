//go:build !darwin

package tui

import (
	"context"

	"github.com/sfate/amp-tui/internal/player"
	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/provider/deezer"
)

type defaultPlayerRuntime struct {
	client  player.MediaClient
	backend player.Backend
}

func newPlayerRuntime(p provider.Provider) PlayerRuntime {
	capable, ok := p.(playbackCapableProvider)
	if !ok || capable.DeezerClient() == nil {
		return nil
	}
	return &defaultPlayerRuntime{
		client:  capable.DeezerClient(),
		backend: player.NewProcessBackend(),
	}
}

func (r *defaultPlayerRuntime) Start(trackID string, quality deezer.AudioQuality, seekMS uint64, handler player.EventHandler) (PlaybackSession, error) {
	session := player.StartTrackPipeline(
		context.Background(),
		r.client,
		r.backend,
		trackID,
		quality,
		seekMS,
		handler,
		player.PipelineOptions{},
	)
	return session, nil
}
