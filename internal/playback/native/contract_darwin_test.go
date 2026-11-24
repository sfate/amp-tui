//go:build darwin

package native

import (
	"bytes"
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/playback/playbacktest"
	"github.com/sfate/amp-tui/internal/provider"
)

// fakeFilePlayer stands in for the Swift helper process.
type fakeFilePlayer struct {
	mu      sync.Mutex
	token   int
	current chan error
	data    []byte
	seekMS  uint64
	playing chan struct{}
}

func newFakeFilePlayer() *fakeFilePlayer {
	return &fakeFilePlayer{playing: make(chan struct{}, 8)}
}

func (p *fakeFilePlayer) play(path string, _ float32, seekMS uint64) (int, <-chan error, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, nil, err
	}
	p.mu.Lock()
	p.token++
	p.current = make(chan error, 1)
	p.data = data
	p.seekMS = seekMS
	token, done := p.token, p.current
	p.mu.Unlock()
	p.playing <- struct{}{}
	return token, done, nil
}

func (p *fakeFilePlayer) finish(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case p.current <- err:
	default:
	}
}

func (p *fakeFilePlayer) pause(int) error              { return nil }
func (p *fakeFilePlayer) resume(int) error             { return nil }
func (p *fakeFilePlayer) stop(int) error               { p.finish(context.Canceled); return nil }
func (p *fakeFilePlayer) setVolume(int, float32) error { return nil }

func (p *fakeFilePlayer) waitPlaying(t *testing.T) {
	t.Helper()
	select {
	case <-p.playing:
	case <-time.After(5 * time.Second):
		t.Fatal("helper play was not called")
	}
}

func newTestEngine(t *testing.T) (*Engine, *fakeFilePlayer) {
	fake := newFakeFilePlayer()
	engine := newEngineWith(fake)
	t.Cleanup(engine.ClearPrebuffer)
	return engine, fake
}

func TestNativeEngineMeetsContract(t *testing.T) {
	playbacktest.RunEngineContract(t, func(t *testing.T) playbacktest.Harness {
		engine, fake := newTestEngine(t)
		return playbacktest.Harness{Engine: engine, EndTrack: func(t *testing.T) {
			fake.waitPlaying(t)
			fake.finish(nil)
		}}
	})
}

func TestNativeEnginePlaysResolvedBytesAtStartOffset(t *testing.T) {
	engine, fake := newTestEngine(t)
	data := bytes.Repeat([]byte("amp"), 1000)
	track := playback.Track{ID: "7", Quality: core.Quality{ID: "q", Seekable: true}, Resolve: func(context.Context) (provider.Source, error) {
		return provider.Source{
			Info:     core.TrackInfo{ID: "7", DurationMS: 60_000},
			Format:   core.FormatMP3,
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
	fake.waitPlaying(t)
	fake.mu.Lock()
	gotData, gotSeek := fake.data, fake.seekMS
	fake.mu.Unlock()
	if !bytes.Equal(gotData, data) || gotSeek != 4_000 {
		t.Fatalf("helper played %d bytes at %dms, want %d bytes at 4000ms", len(gotData), gotSeek, len(data))
	}
	fake.finish(nil)
	if err := session.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}
