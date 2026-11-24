package player

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
)

const (
	DefaultChunkSize      = 2048
	DefaultPrebufferBytes = 512 * 1024
)

type BufferingStage string

const (
	BufferingStageResolving   BufferingStage = "Resolving..."
	BufferingStageDownloading BufferingStage = "Downloading..."
	BufferingStagePreparing   BufferingStage = "Preparing..."
	BufferingStageReady       BufferingStage = "Ready"
)

type Backend interface {
	Start(stream io.ReadSeeker, format core.AudioFormat, handler EventHandler, onFinished func(error)) (Controller, error)
}

type SeekBackend interface {
	StartAt(stream io.ReadSeeker, format core.AudioFormat, initialMS uint64, handler EventHandler, onFinished func(error)) (Controller, error)
}

type Controller interface {
	Pause()
	Resume()
	Stop()
	SetVolume(v float32)
}

type EventHandler struct {
	OnTrackChanged      func(info core.TrackInfo, quality core.Quality, initialMS uint64)
	OnBufferingProgress func(percent uint8, stage BufferingStage)
	OnPlaybackProgress  func(currentMS, totalMS uint64)
	OnAudioBands        func([]uint8)
	OnPlaybackStopped   func()
	OnError             func(error)
}

type PipelineOptions struct {
	PrebufferBytes int
	ChunkSize      int
}

type Session struct {
	mu         sync.RWMutex
	controller Controller
	done       chan error
}

// StartTrackPipeline resolves trackID through streams and plays its bytes on backend.
func StartTrackPipeline(
	ctx context.Context,
	streams provider.StreamSource,
	backend Backend,
	trackID string,
	quality core.Quality,
	seekMS uint64,
	handler EventHandler,
	opts PipelineOptions,
) *Session {
	if opts.PrebufferBytes <= 0 {
		opts.PrebufferBytes = DefaultPrebufferBytes
	}
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = DefaultChunkSize
	}

	session := &Session{done: make(chan error, 1)}
	go func() {
		session.done <- runTrackPipeline(ctx, streams, backend, session, trackID, quality, seekMS, handler, opts)
		close(session.done)
	}()
	return session
}

func (s *Session) Pause()              { s.withController(func(c Controller) { c.Pause() }) }
func (s *Session) Resume()             { s.withController(func(c Controller) { c.Resume() }) }
func (s *Session) Stop()               { s.withController(func(c Controller) { c.Stop() }) }
func (s *Session) SetVolume(v float32) { s.withController(func(c Controller) { c.SetVolume(v) }) }

func (s *Session) FadeOutStop(duration time.Duration) {
	s.mu.RLock()
	controller := s.controller
	s.mu.RUnlock()
	if controller == nil {
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
	controller.Stop()
}

func (s *Session) Wait() error {
	err, ok := <-s.done
	if !ok {
		return nil
	}
	return err
}

func (s *Session) setController(c Controller) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.controller = c
}

func (s *Session) withController(fn func(Controller)) {
	s.mu.RLock()
	controller := s.controller
	s.mu.RUnlock()
	if controller != nil {
		fn(controller)
	}
}

func runTrackPipeline(
	ctx context.Context,
	streams provider.StreamSource,
	backend Backend,
	session *Session,
	trackID string,
	quality core.Quality,
	seekMS uint64,
	handler EventHandler,
	opts PipelineOptions,
) error {
	src, err := streams.Resolve(ctx, trackID, quality)
	if err != nil {
		return reportErr(handler, err)
	}
	if src.Bytes == nil {
		return reportErr(handler, fmt.Errorf("track %s has no byte stream to play", trackID))
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
		session.setController(controller)
		return nil
	}

	var queuedBytes int
	prebufferTarget := opts.PrebufferBytes
	if contentLength > 0 && int(contentLength) < prebufferTarget {
		prebufferTarget = int(contentLength)
	}
	reportBufferingProgress(handler, 0, prebufferTarget, BufferingStageDownloading)
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
			reportBufferingProgress(handler, queuedBytes, prebufferTarget, BufferingStageDownloading)
			select {
			case ch <- chunk:
			case <-ctx.Done():
				close(ch)
				return ctx.Err()
			}

			if !started && queuedBytes >= prebufferTarget {
				reportBufferingProgress(handler, prebufferTarget, prebufferTarget, BufferingStageReady)
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
		reportBufferingProgress(handler, queuedBytes, prebufferTarget, BufferingStageDownloading)
	}

	if !started {
		reportBufferingProgress(handler, prebufferTarget, prebufferTarget, BufferingStageReady)
		if err := startBackend(); err != nil {
			close(ch)
			return reportErr(handler, err)
		}
	}

	close(ch)
	if started {
		select {
		case playErr := <-finishedCh:
			if playErr != nil {
				return reportErr(handler, playErr)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func reportBufferingProgress(handler EventHandler, current, total int, stage BufferingStage) {
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

func reportErr(handler EventHandler, err error) error {
	if handler.OnError != nil {
		handler.OnError(err)
	}
	return err
}
