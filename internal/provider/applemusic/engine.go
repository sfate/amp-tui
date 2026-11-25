package applemusic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/mediactl"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

const defaultPollInterval = 250 * time.Millisecond

// PlaybackEngine returns the Music.app remote-control engine for this provider.
func (p *Provider) PlaybackEngine() playback.Engine { return newEngine(p.runner, defaultPollInterval) }

type musicEngine struct {
	runner   scriptRunner
	interval time.Duration
	media    mediactl.Controller
}

func newEngine(runner scriptRunner, interval time.Duration) *musicEngine {
	if interval <= 0 {
		interval = defaultPollInterval
	}
	return &musicEngine{runner: runner, interval: interval, media: mediactl.New()}
}

func (*musicEngine) Name() string { return "Music.app" }

func (*musicEngine) Capabilities() playback.EngineCaps { return playback.EngineCaps{Seek: true} }

func (e *musicEngine) MediaCommands() <-chan playback.MediaCommand { return e.media.MediaCommands() }

func (e *musicEngine) UpdateMediaState(state playback.MediaState) { e.media.UpdateMediaState(state) }

func (e *musicEngine) CloseMediaControl() { e.media.Close() }

func (e *musicEngine) Start(ctx context.Context, source provider.Source, opts playback.StartOptions, handler playback.EventHandler) (playback.Session, error) {
	if source.Info.ID == "" || source.Quality.ID != qualityID {
		return nil, errors.New("invalid Music.app playback source")
	}
	paused := strconv.FormatBool(opts.Paused)
	if _, err := e.runner.Run(ctx, "play", source.Info.ID, strconv.FormatUint(opts.StartMS, 10), paused); err != nil {
		return nil, fmt.Errorf("start Music.app playback: %w", err)
	}
	info := source.Info
	if handler.OnTrackChanged != nil {
		handler.OnTrackChanged(info, source.Quality, opts.StartMS)
	}
	s := &musicSession{engine: e, info: info, handler: handler, done: make(chan struct{}), result: make(chan error, 1), monitorDone: make(chan struct{})}
	go s.monitor(ctx)
	return s, nil
}

type musicSession struct {
	engine      *musicEngine
	info        core.TrackInfo
	handler     playback.EventHandler
	done        chan struct{}
	result      chan error
	monitorDone chan struct{}
	once        sync.Once
	commands    sync.WaitGroup
}

func (s *musicSession) Pause()  { s.command("pause") }
func (s *musicSession) Resume() { s.command("resume") }
func (s *musicSession) Stop() {
	s.command("stop")
	s.finish(context.Canceled)
}

func (s *musicSession) SetVolume(v float32) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	s.command("volume", strconv.FormatFloat(float64(v), 'f', 3, 32))
}

func (s *musicSession) Seek(ms uint64) error {
	_, err := s.engine.runner.Run(context.Background(), "seek", strconv.FormatUint(ms, 10))
	if err != nil {
		return fmt.Errorf("seek Music.app playback: %w", err)
	}
	return nil
}

func (s *musicSession) FadeOutStop(_ time.Duration) { s.Stop() }

func (s *musicSession) Wait() error {
	err := <-s.result
	<-s.monitorDone
	s.commands.Wait()
	return err
}

func (s *musicSession) command(operation string, args ...string) {
	s.commands.Add(1)
	go func() {
		defer s.commands.Done()
		if _, err := s.engine.runner.Run(context.Background(), operation, args...); err != nil && s.handler.OnError != nil {
			s.handler.OnError(fmt.Errorf("music.app %s: %w", operation, err))
		}
	}()
}

func (s *musicSession) finish(err error) {
	s.once.Do(func() {
		close(s.done)
		s.result <- err
	})
}

func (s *musicSession) monitor(ctx context.Context) {
	defer close(s.monitorDone)
	ticker := time.NewTicker(s.engine.interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ctx.Done():
			s.finish(ctx.Err())
			return
		case <-ticker.C:
			output, err := s.engine.runner.Run(ctx, "state")
			if err != nil {
				if s.handler.OnError != nil {
					s.handler.OnError(fmt.Errorf("read Music.app playback state: %w", err))
				}
				s.finish(err)
				return
			}
			var state playbackState
			if err := json.Unmarshal(output, &state); err != nil {
				wrapped := fmt.Errorf("parse Music.app playback state: %w", err)
				if s.handler.OnError != nil {
					s.handler.OnError(wrapped)
				}
				s.finish(wrapped)
				return
			}
			position := uint64(max(state.Position, 0) * 1000)
			if s.handler.OnPlaybackProgress != nil {
				duration := s.info.DurationMS
				if state.Track.Duration > 0 {
					duration = uint64(state.Track.Duration * 1000)
				}
				s.handler.OnPlaybackProgress(position, duration)
			}
			if strings.EqualFold(state.State, "stopped") {
				s.finish(nil)
				return
			}
		}
	}
}
