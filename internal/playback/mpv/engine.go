// Package mpv implements the mpv-backed playback engine.
package mpv

import (
	"context"
	"os/exec"
	"runtime"

	"github.com/sfate/amp-tui/internal/mediactl"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

const engineName = "mpv"

// Engine starts one isolated mpv process for each playback session.
type Engine struct {
	binary string
	media  mediactl.Controller
}

var (
	_ playback.Engine             = (*Engine)(nil)
	_ playback.MediaController    = (*Engine)(nil)
	_ playback.MediaControlCloser = (*Engine)(nil)
)

func New() *Engine {
	binary, _ := exec.LookPath("mpv")
	return &Engine{binary: binary, media: mediactl.New()}
}

func newWithBinary(binary string) *Engine { return &Engine{binary: binary, media: mediactl.New()} }

func Available() bool {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return false
	}
	_, err := exec.LookPath("mpv")
	return err == nil
}

func (e *Engine) Name() string { return engineName }

func (e *Engine) Capabilities() playback.EngineCaps {
	return playback.EngineCaps{Seek: true, Visualizer: true}
}

func (e *Engine) Start(ctx context.Context, source provider.Source, opts playback.StartOptions, handler playback.EventHandler) (playback.Session, error) {
	return startSession(ctx, e.binary, source, opts, handler)
}

func (e *Engine) MediaCommands() <-chan playback.MediaCommand {
	return e.media.MediaCommands()
}

func (e *Engine) UpdateMediaState(state playback.MediaState) {
	e.media.UpdateMediaState(state)
}

func (e *Engine) CloseMediaControl() {
	e.media.Close()
}
