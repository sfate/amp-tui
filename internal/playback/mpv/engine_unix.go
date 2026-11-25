//go:build darwin || linux

package mpv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

const (
	startupTimeout   = 5 * time.Second
	processStopLimit = 750 * time.Millisecond
)

func mpvArguments(socketPath string) []string {
	return []string{
		"--idle=yes",
		"--no-video",
		"--no-config",
		"--no-terminal",
		"--input-media-keys=no",
		"--demuxer-lavf-o=fflags=+fastseek",
		"--ytdl=no",
		"--input-ipc-server=" + socketPath,
		"--af=@astats:lavfi=[astats=metadata=1:reset=1]",
	}
}

type mpvProcess struct {
	cmd    *exec.Cmd
	done   chan error
	client *ipcClient
}

func startMPV(ctx context.Context, binary, socketPath, streamURL string, startMS uint64, paused bool, volume float32) (*mpvProcess, error) {
	cmd := exec.CommandContext(ctx, binary, mpvArguments(socketPath)...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mpv: %w", err)
	}
	process := &mpvProcess{cmd: cmd, done: make(chan error, 1)}
	go func() { process.done <- cmd.Wait(); close(process.done) }()

	deadline := time.NewTimer(startupTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(30 * time.Millisecond)
	defer ticker.Stop()
	for {
		conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
		if err == nil {
			process.client = newIPCClient(conn)
			break
		}
		select {
		case err := <-process.done:
			return nil, fmt.Errorf("mpv exited before opening its IPC socket: %w", err)
		case <-ctx.Done():
			process.stop()
			return nil, ctx.Err()
		case <-deadline.C:
			process.stop()
			return nil, fmt.Errorf("timed out waiting for mpv IPC socket: %w", err)
		case <-ticker.C:
		}
	}

	setup := []struct {
		command []any
	}{
		{[]any{"set_property", "volume", float64(volume) * 100}},
		{[]any{"set_property", "pause", paused}},
		{[]any{"observe_property", 1, "time-pos"}},
		{[]any{"observe_property", 2, "duration"}},
		{[]any{"observe_property", 3, "af-metadata/astats"}},
		{[]any{"loadfile", streamURL, "replace", "start=" + seconds(startMS)}},
	}
	for _, item := range setup {
		if err := process.client.command(ctx, item.command...); err != nil {
			process.stop()
			return nil, err
		}
	}
	return process, nil
}

func seconds(ms uint64) string {
	return strconv.FormatFloat(float64(ms)/1000, 'f', 3, 64)
}

func (p *mpvProcess) stop() {
	if p == nil {
		return
	}
	if p.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_ = p.client.command(ctx, "quit")
		cancel()
		p.client.close()
	}
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	select {
	case <-p.done:
		return
	case <-time.After(processStopLimit):
		_ = p.cmd.Process.Kill()
		<-p.done
	}
}

type mpvSession struct {
	ctx       context.Context
	cancel    context.CancelFunc
	binary    string
	directory string
	socket    string
	stream    *sourceServer
	source    provider.Source
	handler   playback.EventHandler
	startMS   uint64

	mu         sync.Mutex
	process    *mpvProcess
	paused     bool
	volume     float32
	lastMS     uint64
	durationMS uint64
	started    bool
	stopped    bool

	done       chan struct{}
	result     error
	finishOnce sync.Once
}

var _ playback.Session = (*mpvSession)(nil)

func startSession(ctx context.Context, binary string, source provider.Source, opts playback.StartOptions, handler playback.EventHandler) (playback.Session, error) {
	if binary == "" {
		return nil, errors.New("mpv executable was not found on PATH")
	}
	if source.Bytes == nil {
		return nil, errors.New("mpv playback requires a byte source")
	}
	if !source.Seekable {
		opts.StartMS = 0
	}
	stream, err := newSourceServer(ctx, source)
	if err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp("", "amp-")
	if err != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("create private mpv socket directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		_ = stream.Close()
		_ = os.RemoveAll(directory)
		return nil, fmt.Errorf("secure mpv socket directory: %w", err)
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	session := &mpvSession{
		ctx: sessionCtx, cancel: cancel, binary: binary, directory: directory,
		socket: filepath.Join(directory, "ipc.sock"), stream: stream,
		source: source, handler: handler, startMS: opts.StartMS,
		paused: opts.Paused, volume: 1, lastMS: opts.StartMS,
		durationMS: source.Info.DurationMS, done: make(chan struct{}),
	}
	process, err := startMPV(sessionCtx, binary, session.socket, stream.URL, opts.StartMS, opts.Paused, session.volume)
	if err != nil {
		cancel()
		_ = stream.Close()
		_ = os.RemoveAll(directory)
		return nil, err
	}
	session.process = process
	if handler.OnBufferingProgress != nil {
		handler.OnBufferingProgress(5, playback.BufferingStageDownloading)
	}
	go session.run(process)
	return session, nil
}

func (s *mpvSession) run(process *mpvProcess) {
	defer s.cleanup()
	for {
		select {
		case <-s.ctx.Done():
			s.finish(context.Canceled)
			return
		case message, ok := <-process.client.events:
			if !ok {
				if s.ctx.Err() != nil {
					s.finish(context.Canceled)
					return
				}
				process.stop()
				next, err := s.restart()
				if err != nil {
					if s.ctx.Err() != nil {
						s.finish(context.Canceled)
						return
					}
					s.reportError(err)
					s.finish(err)
					return
				}
				process = next
				continue
			}
			if s.ctx.Err() != nil {
				process.stop()
				s.finish(context.Canceled)
				return
			}
			if ended, err := s.handleMessage(message); ended {
				process.stop()
				if err == nil && s.handler.OnPlaybackStopped != nil {
					s.handler.OnPlaybackStopped()
				} else if !errors.Is(err, context.Canceled) {
					s.reportError(err)
				}
				s.finish(err)
				return
			}
		case err := <-process.done:
			if s.ctx.Err() != nil {
				s.finish(context.Canceled)
				return
			}
			if process.client != nil {
				process.client.close()
				if s.drainEvents(process) {
					return
				}
			}
			next, restartErr := s.restart()
			if restartErr != nil {
				if s.ctx.Err() != nil {
					s.finish(context.Canceled)
					return
				}
				s.reportError(fmt.Errorf("mpv stopped unexpectedly: %w", errors.Join(err, restartErr)))
				s.finish(restartErr)
				return
			}
			process = next
		}
	}
}

func (s *mpvSession) drainEvents(process *mpvProcess) bool {
	for {
		select {
		case message, ok := <-process.client.events:
			if !ok {
				return false
			}
			if s.ctx.Err() != nil {
				process.stop()
				s.finish(context.Canceled)
				return true
			}
			if ended, err := s.handleMessage(message); ended {
				process.stop()
				if err == nil && s.handler.OnPlaybackStopped != nil {
					s.handler.OnPlaybackStopped()
				} else if !errors.Is(err, context.Canceled) {
					s.reportError(err)
				}
				s.finish(err)
				return true
			}
		default:
			return false
		}
	}
}

func (s *mpvSession) restart() (*mpvProcess, error) {
	s.mu.Lock()
	position := s.lastMS
	paused, volume := s.paused, s.volume
	s.mu.Unlock()
	_ = os.Remove(s.socket)
	process, err := startMPV(s.ctx, s.binary, s.socket, s.stream.URL, position, paused, volume)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.process = process
	s.mu.Unlock()
	return process, nil
}

func (s *mpvSession) handleMessage(message ipcMessage) (bool, error) {
	switch message.Event {
	case "playback-restart":
		s.mu.Lock()
		first := !s.started
		s.started = true
		position, duration := s.lastMS, s.durationMS
		s.mu.Unlock()
		if first && s.handler.OnTrackChanged != nil {
			s.handler.OnTrackChanged(s.source.Info, s.source.Quality, position)
		}
		if s.handler.OnPlaybackProgress != nil {
			s.handler.OnPlaybackProgress(position, duration)
		}
	case "property-change":
		s.handleProperty(message.Name, message.Data)
	case "end-file":
		switch message.Reason {
		case "eof":
			return true, nil
		case "error":
			if message.FileError == "" {
				message.FileError = "mpv could not play this audio source"
			}
			return true, errors.New(message.FileError)
		case "stop", "quit":
			s.mu.Lock()
			stopped := s.stopped
			s.mu.Unlock()
			if stopped {
				return true, context.Canceled
			}
			return true, errors.New("mpv stopped playback unexpectedly")
		}
	}
	return false, nil
}

func (s *mpvSession) handleProperty(name string, data any) {
	switch name {
	case "time-pos":
		seconds, ok := data.(float64)
		if !ok || seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
			return
		}
		position := uint64(seconds * 1000)
		s.mu.Lock()
		s.lastMS = position
		duration := s.durationMS
		s.mu.Unlock()
		if s.handler.OnPlaybackProgress != nil {
			s.handler.OnPlaybackProgress(position, duration)
		}
	case "duration":
		seconds, ok := data.(float64)
		if !ok || seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
			return
		}
		duration := uint64(seconds * 1000)
		s.mu.Lock()
		s.durationMS = duration
		position := s.lastMS
		s.mu.Unlock()
		if s.handler.OnPlaybackProgress != nil {
			s.handler.OnPlaybackProgress(position, duration)
		}
	case "af-metadata/astats":
		if s.handler.OnAudioBands != nil {
			if bands, ok := astatsBands(data); ok {
				s.handler.OnAudioBands(bands)
			}
		}
	}
}

func astatsBands(value any) ([]uint8, bool) {
	metadata, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	level, ok := metadata["lavfi.astats.Overall.RMS_level"].(string)
	if !ok {
		return nil, false
	}
	decibels, err := strconv.ParseFloat(level, 64)
	if err != nil || math.IsNaN(decibels) || math.IsInf(decibels, 0) {
		return nil, false
	}
	strength := uint8(math.Max(0, math.Min(255, ((decibels+60)/60)*255)))
	bands := make([]uint8, 8)
	for i := range bands {
		bands[i] = strength
	}
	return bands, true
}

func (s *mpvSession) reportError(err error) {
	if err != nil && s.handler.OnError != nil {
		s.handler.OnError(err)
	}
}

func (s *mpvSession) finish(err error) {
	s.finishOnce.Do(func() {
		s.mu.Lock()
		s.result = err
		s.mu.Unlock()
		close(s.done)
	})
}

func (s *mpvSession) cleanup() {
	s.mu.Lock()
	process := s.process
	s.process = nil
	s.mu.Unlock()
	process.stop()
	s.cancel()
	_ = s.stream.Close()
	_ = os.RemoveAll(s.directory)
}

func (s *mpvSession) withClient(fn func(context.Context, *ipcClient) error) error {
	s.mu.Lock()
	process := s.process
	stopped := s.stopped
	s.mu.Unlock()
	if stopped {
		return context.Canceled
	}
	if process == nil || process.client == nil {
		return errors.New("mpv IPC is not connected")
	}
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Second)
	defer cancel()
	return fn(ctx, process.client)
}

func (s *mpvSession) setProperty(property string, value any) error {
	return s.withClient(func(ctx context.Context, client *ipcClient) error {
		return client.command(ctx, "set_property", property, value)
	})
}

func (s *mpvSession) Pause() {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
	_ = s.setProperty("pause", true)
}

func (s *mpvSession) Resume() {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()
	_ = s.setProperty("pause", false)
}

func (s *mpvSession) Stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.cancel()
}

func (s *mpvSession) SetVolume(volume float32) {
	if volume < 0 {
		volume = 0
	}
	if volume > 1 {
		volume = 1
	}
	s.mu.Lock()
	s.volume = volume
	s.mu.Unlock()
	_ = s.setProperty("volume", float64(volume)*100)
}

func (s *mpvSession) Seek(ms uint64) error {
	s.mu.Lock()
	if !s.source.Seekable {
		s.mu.Unlock()
		return playback.ErrSeekUnsupported
	}
	s.mu.Unlock()
	err := s.withClient(func(ctx context.Context, client *ipcClient) error {
		return client.command(ctx, "seek", seconds(ms), "absolute+exact")
	})
	if err == nil {
		s.mu.Lock()
		s.lastMS = ms
		s.mu.Unlock()
	}
	return err
}

func (s *mpvSession) FadeOutStop(duration time.Duration) {
	if duration <= 0 {
		s.Stop()
		return
	}
	const steps = 20
	stepDuration := duration / steps
	if stepDuration <= 0 {
		stepDuration = time.Millisecond
	}
	for step := steps - 1; step >= 0; step-- {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		s.SetVolume(float32(step) / steps)
		time.Sleep(stepDuration)
	}
	s.Stop()
}

func (s *mpvSession) Wait() error {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result
}
