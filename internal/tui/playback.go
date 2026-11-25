package tui

import (
	"context"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

// PrebufferStatus adds Scheduled, which the TUI sets before the engine reports on a track.
type PrebufferStatus int

const (
	PrebufferStatusScheduled PrebufferStatus = iota
	PrebufferStatusLoading
	PrebufferStatusReady
	PrebufferStatusFailed
)

func prebufferStatusFrom(status playback.PrebufferStatus) PrebufferStatus {
	switch status {
	case playback.PrebufferReady:
		return PrebufferStatusReady
	case playback.PrebufferFailed:
		return PrebufferStatusFailed
	default:
		return PrebufferStatusLoading
	}
}

// playerRuntime binds the injected engine to the active provider's streams.
type playerRuntime struct {
	engine  playback.Engine
	streams provider.StreamSource
}

// newPlayerRuntime is nil when nothing can play: no engine or a provider without streams.
func newPlayerRuntime(engine playback.Engine, p provider.Provider) *playerRuntime {
	streams, ok := p.(provider.StreamSource)
	if engine == nil || !ok {
		return nil
	}
	return &playerRuntime{engine: engine, streams: streams}
}

func (r *playerRuntime) track(trackID string, quality core.Quality) playback.Track {
	return playback.TrackFrom(r.streams, trackID, quality)
}

func (r *playerRuntime) start(trackID string, quality core.Quality, seekMS uint64, handler playback.EventHandler) (playback.Session, error) {
	ctx := context.Background()
	if handler.OnBufferingProgress != nil {
		handler.OnBufferingProgress(0, playback.BufferingStageResolving)
	}
	source, err := r.track(trackID, quality).Resolve(ctx)
	if err != nil {
		return nil, err
	}
	return r.engine.Start(ctx, source, playback.StartOptions{StartMS: seekMS}, handler)
}

func (r *playerRuntime) prebufferer() (playback.Prebufferer, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.engine.(playback.Prebufferer)
	return p, ok
}

func (r *playerRuntime) mediaController() (playback.MediaController, bool) {
	if r == nil {
		return nil, false
	}
	c, ok := r.engine.(playback.MediaController)
	return c, ok
}

func (r *playerRuntime) capabilities() playback.EngineCaps {
	if r == nil {
		return playback.EngineCaps{}
	}
	return r.engine.Capabilities()
}

// shutdown drops prebuffered tracks, because they belong to the provider being left.
func (r *playerRuntime) shutdown() {
	if p, ok := r.prebufferer(); ok {
		p.ClearPrebuffer()
	}
	if media, ok := r.mediaController(); ok {
		if closer, ok := media.(playback.MediaControlCloser); ok {
			closer.CloseMediaControl()
		}
	}
}
