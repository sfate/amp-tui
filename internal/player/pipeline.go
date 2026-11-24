package player

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

const (
	DefaultChunkSize      = 2048
	DefaultPrebufferBytes = 512 * 1024
)

type Backend interface {
	Start(stream io.ReadSeeker, format core.AudioFormat, handler playback.EventHandler, onFinished func(error)) (Controller, error)
}

type SeekBackend interface {
	StartAt(stream io.ReadSeeker, format core.AudioFormat, initialMS uint64, handler playback.EventHandler, onFinished func(error)) (Controller, error)
}

type Controller interface {
	Pause()
	Resume()
	Stop()
	SetVolume(v float32)
}

type PipelineOptions struct {
	PrebufferBytes int
	ChunkSize      int
}

// Session keeps pause, volume and stop until the backend starts, so calls during buffering apply.
type Session struct {
	mu         sync.Mutex
	controller Controller
	paused     bool
	stopped    bool
	volume     *float32
	cancel     context.CancelFunc
	done       chan error
}

var _ playback.Session = (*Session)(nil)

// StartSourcePipeline plays an already resolved source on backend.
func StartSourcePipeline(
	ctx context.Context,
	backend Backend,
	src provider.Source,
	seekMS uint64,
	handler playback.EventHandler,
	opts PipelineOptions,
) *Session {
	if opts.PrebufferBytes <= 0 {
		opts.PrebufferBytes = DefaultPrebufferBytes
	}
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = DefaultChunkSize
	}

	ctx, cancel := context.WithCancel(ctx)
	session := &Session{done: make(chan error, 1), cancel: cancel}
	go func() {
		defer cancel()
		session.done <- runSourcePipeline(ctx, backend, session, src, seekMS, handler, opts)
		close(session.done)
	}()
	return session
}

func (s *Session) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = true
	if s.controller != nil {
		s.controller.Pause()
	}
}

func (s *Session) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = false
	if s.controller != nil {
		s.controller.Resume()
	}
}

func (s *Session) SetVolume(v float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.volume = &v
	if s.controller != nil {
		s.controller.SetVolume(v)
	}
}

func (s *Session) Stop() {
	s.mu.Lock()
	s.stopped = true
	controller := s.controller
	s.mu.Unlock()
	if controller != nil {
		controller.Stop()
	}
	s.cancel()
}

func (s *Session) Seek(uint64) error { return playback.ErrSeekUnsupported }

func (s *Session) FadeOutStop(duration time.Duration) {
	s.mu.Lock()
	controller := s.controller
	s.mu.Unlock()
	if controller == nil {
		s.Stop()
		return
	}

	steps := 20
	stepDuration := duration / time.Duration(steps)
	if stepDuration <= 0 {
		stepDuration = time.Millisecond
	}
	for step := steps - 1; step >= 0; step-- {
		factor := float32(step) / float32(steps)
		controller.SetVolume(factor)
		time.Sleep(stepDuration)
	}
	s.Stop()
}

func (s *Session) Wait() error {
	err, ok := <-s.done
	if !ok {
		return nil
	}
	return err
}

// setController replays the state set during buffering; false means Stop came first.
func (s *Session) setController(c Controller) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return false
	}
	s.controller = c
	if s.volume != nil {
		c.SetVolume(*s.volume)
	}
	if s.paused {
		c.Pause()
	}
	return true
}

func runSourcePipeline(
	ctx context.Context,
	backend Backend,
	session *Session,
	src provider.Source,
	seekMS uint64,
	handler playback.EventHandler,
	opts PipelineOptions,
) error {
	if src.Bytes == nil {
		return reportErr(handler, fmt.Errorf("track %s has no byte stream to play", src.Info.ID))
	}

	initialMS := seekMS
	if !src.Seekable {
		initialMS = 0
	}
	if handler.OnTrackChanged != nil {
		handler.OnTrackChanged(src.Info, src.Quality, initialMS)
	}

	stream, err := src.Bytes.OpenRange(ctx, 0)
	if err != nil {
		return reportErr(handler, err)
	}
	defer func() { _ = stream.Close() }()

	contentLength := src.Bytes.Size()
	trackDurationMS := src.Info.DurationMS
	if trackDurationMS == 0 {
		trackDurationMS = estimateTotalDurationMS(contentLength, src.BytesPerSecond)
	}

	effectiveSeekMS := initialMS
	if effectiveSeekMS >= trackDurationMS {
		effectiveSeekMS = trackDurationMS - 1
	}

	if handler.OnPlaybackProgress != nil {
		handler.OnPlaybackProgress(effectiveSeekMS, trackDurationMS)
	}

	ch := make(chan []byte, 128)
	buffer := NewStreamBuffer(ch)

	var started bool
	finishedCh := make(chan error, 1)
	startBackend := func() error {
		if started {
			return nil
		}
		started = true
		finished := func(playErr error) {
			select {
			case finishedCh <- playErr:
			default:
			}
			if playErr == nil && handler.OnPlaybackStopped != nil {
				handler.OnPlaybackStopped()
			}
		}
		var controller Controller
		var err error
		if seeker, ok := backend.(SeekBackend); ok {
			controller, err = seeker.StartAt(buffer, src.Format, effectiveSeekMS, handler, finished)
		} else {
			controller, err = backend.Start(buffer, src.Format, handler, finished)
		}
		if err != nil {
			return err
		}
		if !session.setController(controller) {
			controller.Stop()
		}
		return nil
	}

	var queuedBytes int
	prebufferTarget := opts.PrebufferBytes
	if contentLength > 0 && int(contentLength) < prebufferTarget {
		prebufferTarget = int(contentLength)
	}
	reportBufferingProgress(handler, 0, prebufferTarget, playback.BufferingStageDownloading)
	pending := make([]byte, 0, opts.ChunkSize*2)
	readBuf := make([]byte, 32*1024)

	for {
		n, readErr := stream.Read(readBuf)
		if n > 0 {
			pending = append(pending, readBuf[:n]...)
		}

		for len(pending) >= opts.ChunkSize {
			chunk := append([]byte(nil), pending[:opts.ChunkSize]...)
			pending = pending[opts.ChunkSize:]

			queuedBytes += len(chunk)
			reportBufferingProgress(handler, queuedBytes, prebufferTarget, playback.BufferingStageDownloading)
			select {
			case ch <- chunk:
			case <-ctx.Done():
				close(ch)
				return ctx.Err()
			}

			if !started && queuedBytes >= prebufferTarget {
				reportBufferingProgress(handler, prebufferTarget, prebufferTarget, playback.BufferingStageReady)
				if err := startBackend(); err != nil {
					close(ch)
					return reportErr(handler, err)
				}
			}
		}

		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			close(ch)
			return reportErr(handler, readErr)
		}
	}

	if len(pending) > 0 {
		tail := append([]byte(nil), pending...)
		select {
		case ch <- tail:
		case <-ctx.Done():
			close(ch)
			return ctx.Err()
		}
		queuedBytes += len(tail)
		reportBufferingProgress(handler, queuedBytes, prebufferTarget, playback.BufferingStageDownloading)
	}

	if !started {
		reportBufferingProgress(handler, prebufferTarget, prebufferTarget, playback.BufferingStageReady)
		if err := startBackend(); err != nil {
			close(ch)
			return reportErr(handler, err)
		}
	}

	close(ch)
	if started {
		select {
		case playErr := <-finishedCh:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if playErr != nil {
				return reportErr(handler, playErr)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func reportBufferingProgress(handler playback.EventHandler, current, total int, stage playback.BufferingStage) {
	if handler.OnBufferingProgress == nil || total <= 0 {
		return
	}
	percent := current * 100 / total
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	handler.OnBufferingProgress(uint8(percent), stage)
}

// estimateTotalDurationMS assumes 320 kbps MP3 when the source gives no byte rate.
func estimateTotalDurationMS(totalBytes int64, bytesPerSecond int) uint64 {
	if bytesPerSecond <= 0 {
		bytesPerSecond = 40_000
	}
	if totalBytes <= 0 {
		return 224_000
	}
	duration := uint64(totalBytes) * 1000 / uint64(bytesPerSecond)
	if duration == 0 {
		return 1
	}
	return duration
}

// reportErr skips context.Canceled, because only Session.Stop cancels the pipeline.
func reportErr(handler playback.EventHandler, err error) error {
	if handler.OnError != nil && !errors.Is(err, context.Canceled) {
		handler.OnError(err)
	}
	return err
}
