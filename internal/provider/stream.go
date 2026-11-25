package provider

import (
	"context"
	"io"

	"github.com/sfate/amp-tui/internal/core"
)

// ByteSource gives the playable container bytes of one resolved track.
type ByteSource interface {
	// Size is the total byte count, or -1 until an OpenRange response tells it.
	Size() int64
	// OpenRange returns decrypted bytes from offset to the end of the track.
	OpenRange(ctx context.Context, offset int64) (io.ReadCloser, error)
}

// Source is a resolved track; it sets URL or Bytes, never both.
type Source struct {
	Info    core.TrackInfo
	Quality core.Quality
	Format  core.AudioFormat
	URL     string
	// HTTPHeaders accompany a direct URL when the player opens it.
	HTTPHeaders map[string]string
	Bytes       ByteSource
	// Seekable is false when playback must start at 0.
	Seekable bool
	// BytesPerSecond estimates the duration when Info has none.
	BytesPerSecond int
}

// StreamSource is implemented by a Provider whose tracks play through our engine.
type StreamSource interface {
	// Qualities is ordered from low to high.
	Qualities() []core.Quality
	DefaultQuality() core.Quality
	Resolve(ctx context.Context, trackID string, q core.Quality) (Source, error)
}
