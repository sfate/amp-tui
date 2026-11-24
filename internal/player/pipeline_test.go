package player

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

// fakeByteSource serves plain bytes and records each requested offset.
type fakeByteSource struct {
	data    []byte
	mu      sync.Mutex
	offsets []int64
}

func (f *fakeByteSource) Size() int64 { return int64(len(f.data)) }

func (f *fakeByteSource) OpenRange(_ context.Context, offset int64) (io.ReadCloser, error) {
	f.mu.Lock()
	f.offsets = append(f.offsets, offset)
	f.mu.Unlock()
	return io.NopCloser(bytes.NewReader(f.data[offset:])), nil
}

type fakeStreamSource struct {
	source provider.Source
}

func (f *fakeStreamSource) Qualities() []core.Quality { return []core.Quality{f.source.Quality} }

func (f *fakeStreamSource) DefaultQuality() core.Quality { return f.source.Quality }

func (f *fakeStreamSource) Resolve(_ context.Context, _ string, q core.Quality) (provider.Source, error) {
	src := f.source
	src.Quality = q
	return src, nil
}

var (
	testQuality320  = core.Quality{ID: "Kbps320", Label: "320 kbps", Seekable: true}
	testQualityFlac = core.Quality{ID: "Flac", Label: "FLAC", Seekable: false}
)

func mp3Source(info core.TrackInfo, data []byte) *fakeStreamSource {
	return &fakeStreamSource{source: provider.Source{
		Info:           info,
		Quality:        testQuality320,
		Format:         core.FormatMP3,
		Bytes:          &fakeByteSource{data: data},
		Seekable:       true,
		BytesPerSecond: 40_000,
	}}
}

func resolveSource(t *testing.T, streams *fakeStreamSource, id string, quality core.Quality) provider.Source {
	t.Helper()
	source, err := streams.Resolve(context.Background(), id, quality)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

type fakeBackend struct {
	mu         sync.Mutex
	playedData []byte
	stopped    bool
	initialMS  uint64
	format     core.AudioFormat
}

func (b *fakeBackend) Start(stream io.ReadSeeker, format core.AudioFormat, handler playback.EventHandler, onFinished func(error)) (Controller, error) {
	return b.StartAt(stream, format, 0, handler, onFinished)
}

func (b *fakeBackend) StartAt(stream io.ReadSeeker, format core.AudioFormat, initialMS uint64, _ playback.EventHandler, onFinished func(error)) (Controller, error) {
	b.mu.Lock()
	b.initialMS = initialMS
	b.format = format
	b.mu.Unlock()
	go func() {
		data, err := io.ReadAll(stream)
		if err != nil {
			return
		}

		b.mu.Lock()
		b.playedData = append([]byte(nil), data...)
		b.mu.Unlock()

		onFinished(nil)
	}()
	return &fakeController{backend: b}, nil
}

type fakeController struct {
	backend *fakeBackend
}

func (c *fakeController) Pause()              {}
func (c *fakeController) Resume()             {}
func (c *fakeController) SetVolume(v float32) {}
func (c *fakeController) Stop() {
	c.backend.mu.Lock()
	defer c.backend.mu.Unlock()
	c.backend.stopped = true
}

func TestStartSourcePipelineStreamsAudio(t *testing.T) {
	plaintext := bytes.Repeat([]byte{0x5a}, chunkSizeForTest()*4)

	client := mp3Source(core.TrackInfo{ID: "42", Title: "Track", Artist: "Artist"}, plaintext)
	backend := &fakeBackend{}

	var changed atomic.Bool
	var stopped atomic.Bool
	var progressCurrent atomic.Uint64
	var progressTotal atomic.Uint64
	var bufferingMax atomic.Uint32

	session := StartSourcePipeline(
		context.Background(),
		backend,
		resolveSource(t, client, "42", testQuality320),
		0,
		playback.EventHandler{
			OnTrackChanged: func(info core.TrackInfo, quality core.Quality, initialMS uint64) {
				changed.Store(info.ID == "42" && quality == testQuality320 && initialMS == 0)
			},
			OnPlaybackProgress: func(currentMS, totalMS uint64) {
				progressCurrent.Store(currentMS)
				progressTotal.Store(totalMS)
			},
			OnBufferingProgress: func(percent uint8, _ playback.BufferingStage) {
				for {
					current := bufferingMax.Load()
					if uint32(percent) <= current || bufferingMax.CompareAndSwap(current, uint32(percent)) {
						return
					}
				}
			},
			OnPlaybackStopped: func() {
				stopped.Store(true)
			},
		},
		PipelineOptions{
			PrebufferBytes: 1024,
			ChunkSize:      chunkSizeForTest(),
		},
	)

	if err := session.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}

	for range 1000 {
		if stopped.Load() {
			break
		}
		runtime.Gosched()
	}

	if !changed.Load() {
		t.Fatalf("expected track-changed event")
	}
	if !stopped.Load() {
		t.Fatalf("expected playback-stopped event")
	}
	if progressCurrent.Load() != 0 || progressTotal.Load() == 0 {
		t.Fatalf("unexpected progress event current=%d total=%d", progressCurrent.Load(), progressTotal.Load())
	}
	if bufferingMax.Load() != 100 {
		t.Fatalf("expected buffering to reach 100%%, got %d", bufferingMax.Load())
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if !bytes.Equal(backend.playedData, plaintext) {
		t.Fatalf("backend received decrypted bytes mismatch")
	}
}

func TestStartSourcePipelinePassesInitialSeekTimeToBackend(t *testing.T) {
	plaintext := bytes.Repeat([]byte("abcd"), 1024)

	client := mp3Source(core.TrackInfo{ID: "42", Title: "Track", Artist: "Artist", DurationMS: 4000}, plaintext)
	backend := &fakeBackend{}

	session := StartSourcePipeline(
		context.Background(),
		backend,
		resolveSource(t, client, "42", testQuality320),
		2000,
		playback.EventHandler{},
		PipelineOptions{
			PrebufferBytes: 1,
			ChunkSize:      chunkSizeForTest(),
		},
	)

	if err := session.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.initialMS != 2000 {
		t.Fatalf("expected backend seek time 2000ms, got %dms", backend.initialMS)
	}
	if !bytes.Equal(backend.playedData, plaintext) {
		t.Fatal("pipeline should pass the full stream so the decoder can seek by samples")
	}
}

func TestStartSourcePipelineReadsPlainByteSource(t *testing.T) {
	mp3 := append([]byte("ID3\x04\x00\x00\x00\x00\x00\x00"), bytes.Repeat([]byte{0xff, 0xfb, 0x90, 0x64}, 1500)...)
	streams := mp3Source(core.TrackInfo{ID: "7", Title: "Plain", Artist: "Artist"}, mp3)
	bytesSrc := streams.source.Bytes.(*fakeByteSource)
	backend := &fakeBackend{}
	var gotInfo atomic.Value

	session := StartSourcePipeline(context.Background(), backend, resolveSource(t, streams, "7", testQuality320), 0, playback.EventHandler{
		OnTrackChanged: func(info core.TrackInfo, _ core.Quality, _ uint64) { gotInfo.Store(info) },
	}, PipelineOptions{PrebufferBytes: 1024})
	if err := session.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}

	if info, _ := gotInfo.Load().(core.TrackInfo); info.Title != "Plain" {
		t.Fatalf("expected track info from the source, got %+v", info)
	}
	bytesSrc.mu.Lock()
	offsets := append([]int64(nil), bytesSrc.offsets...)
	bytesSrc.mu.Unlock()
	if len(offsets) != 1 || offsets[0] != 0 {
		t.Fatalf("expected one OpenRange at offset 0, got %v", offsets)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.format != core.FormatMP3 {
		t.Fatalf("expected MP3 format at the backend, got %q", backend.format)
	}
	if !bytes.Equal(backend.playedData, mp3) {
		t.Fatal("backend must receive the source bytes unchanged")
	}
}

func TestStartSourcePipelineStartsUnseekableSourceAtZero(t *testing.T) {
	data := bytes.Repeat([]byte{0x01}, 4096)
	streams := &fakeStreamSource{source: provider.Source{
		Info:           core.TrackInfo{ID: "9"},
		Format:         core.FormatFLAC,
		Bytes:          &fakeByteSource{data: data},
		Seekable:       false,
		BytesPerSecond: 1000,
	}}
	backend := &fakeBackend{}
	var changedMS atomic.Uint64
	changedMS.Store(1)
	var totalMS atomic.Uint64

	source, err := streams.Resolve(context.Background(), "9", testQualityFlac)
	if err != nil {
		t.Fatal(err)
	}
	session := StartSourcePipeline(context.Background(), backend, source, 2000, playback.EventHandler{
		OnTrackChanged:     func(_ core.TrackInfo, _ core.Quality, initialMS uint64) { changedMS.Store(initialMS) },
		OnPlaybackProgress: func(_, total uint64) { totalMS.Store(total) },
	}, PipelineOptions{PrebufferBytes: 1})
	if err := session.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}

	if changedMS.Load() != 0 {
		t.Fatalf("expected unseekable source to report 0ms, got %d", changedMS.Load())
	}
	if totalMS.Load() != 4096 {
		t.Fatalf("expected duration estimate from BytesPerSecond (4096ms), got %d", totalMS.Load())
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.initialMS != 0 || backend.format != core.FormatFLAC {
		t.Fatalf("expected FLAC start at 0ms, got %q at %dms", backend.format, backend.initialMS)
	}
}

func TestEstimateTotalDurationUsesBytesPerSecond(t *testing.T) {
	for _, tc := range []struct {
		size        int64
		bytesPerSec int
		want        uint64
	}{
		{size: 40_000, bytesPerSec: 40_000, want: 1000},
		{size: 90_000, bytesPerSec: 90_000, want: 1000},
		{size: 40_000, bytesPerSec: 0, want: 1000},
		{size: 0, bytesPerSec: 16_000, want: 224_000},
	} {
		if got := estimateTotalDurationMS(tc.size, tc.bytesPerSec); got != tc.want {
			t.Fatalf("estimate(%d, %d) = %d, want %d", tc.size, tc.bytesPerSec, got, tc.want)
		}
	}
}

func chunkSizeForTest() int {
	return 2048
}
