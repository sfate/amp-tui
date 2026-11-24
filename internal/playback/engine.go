// Package playback is the engine contract the TUI plays tracks through.
package playback

import (
	"context"
	"errors"
	"time"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
)

// ErrSeekUnsupported is the Session.Seek result of an engine without in-session seek.
var ErrSeekUnsupported = errors.New("engine cannot seek inside a session")

// Engine plays one track per Session; a crossfade runs two sessions at once.
type Engine interface {
	Name() string
	Capabilities() EngineCaps
	Start(ctx context.Context, source provider.Source, opts StartOptions, handler EventHandler) (Session, error)
}

type EngineCaps struct {
	// Seek is true when Session.Seek moves the playing session; otherwise the caller starts a new one.
	Seek bool
	// Visualizer is true when the engine reports EventHandler.OnAudioBands.
	Visualizer bool
}

// Track identifies a track and quality for optional engine prefetching.
type Track struct {
	// ID is opaque; with Quality.ID it keys optional engine prebuffer caches.
	ID      string
	Quality core.Quality
	Resolve func(ctx context.Context) (provider.Source, error)
}

// TrackFrom binds a track ID to the provider stream that resolves it.
func TrackFrom(streams provider.StreamSource, id string, q core.Quality) Track {
	return Track{
		ID:      id,
		Quality: q,
		Resolve: func(ctx context.Context) (provider.Source, error) {
			return streams.Resolve(ctx, id, q)
		},
	}
}

type StartOptions struct {
	// StartMS is ignored when the resolved source is not seekable.
	StartMS        uint64
	Paused         bool
	PrebufferBytes int
}

// Session controls one playing track; Pause, SetVolume and Stop also apply before audio starts.
type Session interface {
	Pause()
	Resume()
	Stop()
	SetVolume(v float32)
	Seek(ms uint64) error
	FadeOutStop(d time.Duration)
	// Wait returns nil at the natural end and context.Canceled after Stop.
	Wait() error
}

type BufferingStage string

const (
	BufferingStageResolving   BufferingStage = "Resolving..."
	BufferingStageDownloading BufferingStage = "Downloading..."
	BufferingStagePreparing   BufferingStage = "Preparing..."
	BufferingStageReady       BufferingStage = "Ready"
)

// EventHandler callbacks run on engine goroutines; nil callbacks are skipped.
type EventHandler struct {
	OnTrackChanged      func(info core.TrackInfo, quality core.Quality, initialMS uint64)
	OnBufferingProgress func(percent uint8, stage BufferingStage)
	OnPlaybackProgress  func(currentMS, totalMS uint64)
	OnAudioBands        func([]uint8)
	OnPlaybackStopped   func()
	OnError             func(error)
}
