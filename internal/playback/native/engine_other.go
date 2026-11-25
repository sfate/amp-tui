//go:build !darwin

package native

import (
	"context"

	"github.com/sfate/amp-tui/internal/mediactl"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/player"
	"github.com/sfate/amp-tui/internal/provider"
)

// Engine streams the track bytes through the player pipeline into beep.
type Engine struct {
	backend player.Backend
	opts    player.PipelineOptions
	media   mediactl.Controller
}

var (
	_ playback.Engine             = (*Engine)(nil)
	_ playback.MediaController    = (*Engine)(nil)
	_ playback.MediaControlCloser = (*Engine)(nil)
)

func New() *Engine {
	return &Engine{backend: player.NewBeepBackend(), media: mediactl.New()}
}

func newEngineWith(backend player.Backend, opts player.PipelineOptions) *Engine {
	return &Engine{backend: backend, opts: opts, media: mediactl.New()}
}

func (e *Engine) MediaCommands() <-chan playback.MediaCommand { return e.media.MediaCommands() }

func (e *Engine) UpdateMediaState(state playback.MediaState) { e.media.UpdateMediaState(state) }

func (e *Engine) CloseMediaControl() { e.media.Close() }

func (e *Engine) Name() string { return engineName }

func (e *Engine) Capabilities() playback.EngineCaps { return capabilities }

func (e *Engine) Start(ctx context.Context, source provider.Source, opts playback.StartOptions, handler playback.EventHandler) (playback.Session, error) {
	pipelineOpts := e.opts
	if opts.PrebufferBytes > 0 {
		pipelineOpts.PrebufferBytes = opts.PrebufferBytes
	}
	session := player.StartSourcePipeline(ctx, e.backend, source, opts.StartMS, handler, pipelineOpts)
	if opts.Paused {
		session.Pause()
	}
	return session, nil
}
