// Package playbacktest holds a fake Engine and the contract every Engine must meet.
package playbacktest

import (
	"context"
	"sync"
	"time"

	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

// StartCall records one Engine.Start.
type StartCall struct {
	Source  provider.Source
	Opts    playback.StartOptions
	Handler playback.EventHandler
}

// Engine records Start calls and returns a Session; Simulate reports events for the source.
type Engine struct {
	Caps     playback.EngineCaps
	Simulate bool
	StartErr error

	mu       sync.Mutex
	starts   []StartCall
	sessions []*Session
}

var _ playback.Engine = (*Engine)(nil)

func (e *Engine) Name() string { return "fake" }

func (e *Engine) Capabilities() playback.EngineCaps { return e.Caps }

func (e *Engine) Start(ctx context.Context, source provider.Source, opts playback.StartOptions, handler playback.EventHandler) (playback.Session, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.starts = append(e.starts, StartCall{Source: source, Opts: opts, Handler: handler})
	if e.StartErr != nil {
		return nil, e.StartErr
	}
	session := newSession(e.Caps.Seek)
	e.sessions = append(e.sessions, session)
	if e.Simulate {
		go session.simulate(source, opts, handler)
	} else {
		session.finish(nil)
	}
	return session, nil
}

func (e *Engine) Starts() []StartCall {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]StartCall(nil), e.starts...)
}

func (e *Engine) Sessions() []*Session {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]*Session(nil), e.sessions...)
}

// LastSession is nil before the first successful Start.
func (e *Engine) LastSession() *Session {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.sessions) == 0 {
		return nil
	}
	return e.sessions[len(e.sessions)-1]
}

// Session records control calls; a passive session's Wait returns WaitErr at once.
type Session struct {
	WaitErr error
	Faded   chan time.Duration

	canSeek bool
	end     chan struct{}
	endOnce sync.Once
	done    chan struct{}
	result  error

	mu      sync.Mutex
	paused  bool
	resumed bool
	stopped bool
	volume  float32
	seeks   []uint64
}

// NewSession returns a passive session for tests that hand one to the caller directly.
func NewSession() *Session {
	s := newSession(false)
	s.finish(nil)
	return s
}

func newSession(canSeek bool) *Session {
	return &Session{canSeek: canSeek, end: make(chan struct{}), done: make(chan struct{}), volume: 1}
}

func (s *Session) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = true
}

func (s *Session) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = false
	s.resumed = true
}

func (s *Session) Stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.EndTrack()
}

func (s *Session) SetVolume(v float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.volume = v
}

func (s *Session) Seek(ms uint64) error {
	if !s.canSeek {
		return playback.ErrSeekUnsupported
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seeks = append(s.seeks, ms)
	return nil
}

func (s *Session) FadeOutStop(d time.Duration) {
	if s.Faded != nil {
		s.Faded <- d
	}
}

func (s *Session) Wait() error {
	<-s.done
	if s.WaitErr != nil {
		return s.WaitErr
	}
	return s.result
}

// EndTrack makes a simulated session reach its end.
func (s *Session) EndTrack() {
	s.endOnce.Do(func() { close(s.end) })
}

func (s *Session) Paused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

func (s *Session) Resumed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resumed
}

func (s *Session) Stopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

func (s *Session) Volume() float32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.volume
}

func (s *Session) Seeks() []uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uint64(nil), s.seeks...)
}

func (s *Session) finish(err error) {
	s.result = err
	close(s.done)
}

func (s *Session) simulate(src provider.Source, opts playback.StartOptions, handler playback.EventHandler) {
	initialMS := opts.StartMS
	if !src.Seekable {
		initialMS = 0
	}
	if handler.OnTrackChanged != nil {
		handler.OnTrackChanged(src.Info, src.Quality, initialMS)
	}
	if handler.OnPlaybackProgress != nil {
		handler.OnPlaybackProgress(initialMS, src.Info.DurationMS)
	}
	<-s.end
	if s.Stopped() {
		s.finish(context.Canceled)
		return
	}
	if handler.OnPlaybackStopped != nil {
		handler.OnPlaybackStopped()
	}
	s.finish(nil)
}
