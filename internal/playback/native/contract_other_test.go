//go:build !darwin

package native

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/playback/playbacktest"
	"github.com/sfate/amp-tui/internal/player"
	"github.com/sfate/amp-tui/internal/provider"
)

// fakeBackend reads the whole stream, then plays until the test ends the track or Stop runs.
type fakeBackend struct {
	mu        sync.Mutex
	data      []byte
	initialMS uint64
	format    core.AudioFormat
	finish    func(error)
	playing   chan struct{}
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{playing: make(chan struct{}, 8)}
}

func (b *fakeBackend) Start(stream io.ReadSeeker, format core.AudioFormat, handler playback.EventHandler, onFinished func(error)) (player.Controller, error) {
	return b.StartAt(stream, format, 0, handler, onFinished)
}

func (b *fakeBackend) StartAt(stream io.ReadSeeker, format core.AudioFormat, initialMS uint64, _ playback.EventHandler, onFinished func(error)) (player.Controller, error) {
	var once sync.Once
	finish := func(err error) { once.Do(func() { onFinished(err) }) }
	go func() {
		data, _ := io.ReadAll(stream)
		b.mu.Lock()
		b.data, b.initialMS, b.format, b.finish = data, initialMS, format, finish
		b.mu.Unlock()
		b.playing <- struct{}{}
	}()
	return fakeController{stop: func() { finish(context.Canceled) }}, nil
}

func (b *fakeBackend) waitPlaying(t *testing.T) {
	t.Helper()
	select {
	case <-b.playing:
	case <-time.After(5 * time.Second):
		t.Fatal("backend did not start")
	}
}

func (b *fakeBackend) end() {
	b.mu.Lock()
	finish := b.finish
	b.mu.Unlock()
	finish(nil)
}

type fakeController struct{ stop func() }

func (fakeController) Pause()            {}
func (fakeController) Resume()           {}
func (fakeController) SetVolume(float32) {}
func (c fakeController) Stop()           { c.stop() }

func newTestEngine() (*Engine, *fakeBackend) {
	backend := newFakeBackend()
	return newEngineWith(backend, player.PipelineOptions{PrebufferBytes: 1024}), backend
}

func TestNativeEngineMeetsContract(t *testing.T) {
	playbacktest.RunEngineContract(t, func(*testing.T) playbacktest.Harness {
		engine, backend := newTestEngine()
		return playbacktest.Harness{Engine: engine, EndTrack: func(t *testing.T) {
			backend.waitPlaying(t)
			backend.end()
		}}
	})
}

func TestNativeEnginePlaysResolvedBytesAtStartOffset(t *testing.T) {
	engine, backend := newTestEngine()
	data := bytes.Repeat([]byte("amp"), 1000)
	track := playback.Track{ID: "7", Quality: core.Quality{ID: "q", Seekable: true}, Resolve: func(context.Context) (provider.Source, error) {
		return provider.Source{
			Info:     core.TrackInfo{ID: "7", DurationMS: 60_000},
			Format:   core.FormatFLAC,
			Bytes:    memoryBytes(data),
			Seekable: true,
		}, nil
	}}

	source, err := track.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	session, err := engine.Start(context.Background(), source, playback.StartOptions{StartMS: 4_000}, playback.EventHandler{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	backend.waitPlaying(t)
	backend.mu.Lock()
	gotData, gotMS, gotFormat := backend.data, backend.initialMS, backend.format
	backend.mu.Unlock()
	if !bytes.Equal(gotData, data) || gotMS != 4_000 || gotFormat != core.FormatFLAC {
		t.Fatalf("backend got %d bytes of %q at %dms, want %d bytes of flac at 4000ms", len(gotData), gotFormat, gotMS, len(data))
	}
	backend.end()
	if err := session.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}
