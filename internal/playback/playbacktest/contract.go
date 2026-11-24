package playbacktest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

// Harness is one Engine under test plus the output hook it needs.
type Harness struct {
	Engine playback.Engine
	// EndTrack waits until the latest session plays, then makes it reach its natural end.
	EndTrack func(t *testing.T)
}

const contractTimeout = 5 * time.Second

var contractQuality = core.Quality{ID: "contract", Label: "Contract", Seekable: true}

// RunEngineContract runs the behavior the TUI relies on against newHarness.
func RunEngineContract(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Run("reports the resolved track at the start offset", func(t *testing.T) {
		h := newHarness(t)
		events := newRecorder()
		session := start(t, h.Engine, memoryTrack(true), 5_000, events.handler())

		changed := events.waitChanged(t)
		if changed.info.ID != "contract-1" || changed.quality != contractQuality || changed.initialMS != 5_000 {
			t.Fatalf("track changed = %+v, want contract-1 at 5000ms", changed)
		}
		h.EndTrack(t)
		if err := wait(t, session); err != nil {
			t.Fatalf("Wait after the natural end = %v, want nil", err)
		}
		events.waitStopped(t)
	})

	t.Run("starts a non-seekable source at zero", func(t *testing.T) {
		h := newHarness(t)
		events := newRecorder()
		session := start(t, h.Engine, memoryTrack(false), 5_000, events.handler())

		if changed := events.waitChanged(t); changed.initialMS != 0 {
			t.Fatalf("initialMS = %d, want 0 for a non-seekable source", changed.initialMS)
		}
		h.EndTrack(t)
		if err := wait(t, session); err != nil {
			t.Fatalf("Wait = %v, want nil", err)
		}
	})

	t.Run("stop ends the session as canceled", func(t *testing.T) {
		h := newHarness(t)
		events := newRecorder()
		session := start(t, h.Engine, memoryTrack(true), 0, events.handler())
		events.waitChanged(t)

		session.Stop()
		if err := wait(t, session); !errors.Is(err, context.Canceled) {
			t.Fatalf("Wait after Stop = %v, want context.Canceled", err)
		}
		if events.stopped.Load() != 0 {
			t.Fatal("OnPlaybackStopped must not run after Stop")
		}
	})

	t.Run("seek follows the capabilities", func(t *testing.T) {
		h := newHarness(t)
		events := newRecorder()
		session := start(t, h.Engine, memoryTrack(true), 0, events.handler())
		events.waitChanged(t)

		err := session.Seek(1_000)
		if h.Engine.Capabilities().Seek && err != nil {
			t.Fatalf("Seek = %v on an engine with in-session seek", err)
		}
		if !h.Engine.Capabilities().Seek && !errors.Is(err, playback.ErrSeekUnsupported) {
			t.Fatalf("Seek = %v, want ErrSeekUnsupported", err)
		}
		session.Stop()
		_ = wait(t, session)
	})
}

// memorySource serves 8 KiB of filler bytes; engine outputs in contract tests never decode them.
func memorySource(seekable bool) provider.Source {
	return provider.Source{
		Info:           core.TrackInfo{ID: "contract-1", Title: "Contract", Artist: "Engine", DurationMS: 60_000},
		Quality:        contractQuality,
		Format:         core.FormatMP3,
		Bytes:          memoryBytes(bytes.Repeat([]byte{0x5a}, 8*1024)),
		Seekable:       seekable,
		BytesPerSecond: 40_000,
	}
}

func memoryTrack(seekable bool) playback.Track {
	return playback.Track{ID: "contract-1", Quality: contractQuality, Resolve: func(context.Context) (provider.Source, error) {
		return memorySource(seekable), nil
	}}
}

type memoryBytes []byte

func (b memoryBytes) Size() int64 { return int64(len(b)) }

func (b memoryBytes) OpenRange(_ context.Context, offset int64) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(b[offset:])), nil
}

func start(t *testing.T, engine playback.Engine, track playback.Track, startMS uint64, handler playback.EventHandler) playback.Session {
	t.Helper()
	source, err := track.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	session, err := engine.Start(context.Background(), source, playback.StartOptions{StartMS: startMS}, handler)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return session
}

func wait(t *testing.T, session playback.Session) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(contractTimeout):
		t.Fatal("Wait did not return")
		return nil
	}
}

type trackChange struct {
	info      core.TrackInfo
	quality   core.Quality
	initialMS uint64
}

type recorder struct {
	changed chan trackChange
	stopped atomic.Int32
}

func newRecorder() *recorder {
	return &recorder{changed: make(chan trackChange, 1)}
}

func (r *recorder) handler() playback.EventHandler {
	return playback.EventHandler{
		OnTrackChanged: func(info core.TrackInfo, quality core.Quality, initialMS uint64) {
			r.changed <- trackChange{info: info, quality: quality, initialMS: initialMS}
		},
		OnPlaybackStopped: func() { r.stopped.Add(1) },
	}
}

func (r *recorder) waitChanged(t *testing.T) trackChange {
	t.Helper()
	select {
	case change := <-r.changed:
		return change
	case <-time.After(contractTimeout):
		t.Fatal("OnTrackChanged did not run")
		return trackChange{}
	}
}

// waitStopped polls, because an engine may report the end after Wait returns.
func (r *recorder) waitStopped(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(contractTimeout)
	for r.stopped.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("OnPlaybackStopped did not run")
		}
		time.Sleep(time.Millisecond)
	}
	if n := r.stopped.Load(); n != 1 {
		t.Fatalf("OnPlaybackStopped ran %d times, want 1", n)
	}
}
