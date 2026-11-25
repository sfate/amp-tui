//go:build darwin

package native

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/mediactl"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/player"
	"github.com/sfate/amp-tui/internal/provider"
)

// Engine plays prepared temp files through the Swift helper, which also serves Now Playing.
type Engine struct {
	player filePlayer
	media  *darwinHelperManager

	mu        sync.Mutex
	prebuffer *darwinPrebufferStore
}

var (
	_ playback.Engine             = (*Engine)(nil)
	_ playback.Prebufferer        = (*Engine)(nil)
	_ playback.MediaController    = (*Engine)(nil)
	_ playback.MediaControlCloser = (*Engine)(nil)
)

// filePlayer is the helper side of a session; tests replace the helper process with it.
type filePlayer interface {
	play(path string, volume float32, seekMS uint64) (int, <-chan error, error)
	pause(token int) error
	resume(token int) error
	stop(token int) error
	setVolume(token int, volume float32) error
}

// New starts no process; the helper compiles and starts on first use.
func New() *Engine {
	manager := newDarwinHelperManager()
	return &Engine{player: manager, media: manager, prebuffer: newDarwinPrebufferStore()}
}

func newEngineWith(player filePlayer) *Engine {
	return &Engine{player: player, prebuffer: newDarwinPrebufferStore()}
}

func (e *Engine) Name() string { return engineName }

func (e *Engine) Capabilities() playback.EngineCaps { return capabilities }

func (e *Engine) Start(ctx context.Context, source provider.Source, opts playback.StartOptions, handler playback.EventHandler) (playback.Session, error) {
	session := &darwinPlaybackSession{
		done:   make(chan error, 1),
		volume: 1,
		paused: opts.Paused,
		player: e.player,
	}
	go session.run(ctx, e.currentPrebuffer(), source, opts.StartMS, handler)
	return session, nil
}

func (e *Engine) Prebuffer(tracks []playback.Track, onStatus func(playback.Track, playback.PrebufferStatus)) {
	e.currentPrebuffer().Prebuffer(context.Background(), tracks, onStatus)
}

// ClearPrebuffer closes the store so in-flight jobs drop their files, then starts an empty one.
func (e *Engine) ClearPrebuffer() {
	e.mu.Lock()
	old := e.prebuffer
	e.prebuffer = newDarwinPrebufferStore()
	e.mu.Unlock()
	old.Close()
}

func (e *Engine) currentPrebuffer() *darwinPrebufferStore {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.prebuffer
}

func (e *Engine) MediaCommands() <-chan playback.MediaCommand {
	if e.media == nil {
		return nil
	}
	e.media.startAsync()
	return e.media.mediaEvents
}

func (e *Engine) UpdateMediaState(state playback.MediaState) {
	if e.media == nil {
		return
	}
	e.media.updateMediaControl(state)
}

func (e *Engine) CloseMediaControl() {
	if e.media != nil {
		e.media.close()
	}
}

type darwinPlaybackSession struct {
	mu              sync.Mutex
	file            string
	paused          bool
	stopped         bool
	volume          float32
	done            chan error
	token           int
	player          filePlayer
	finished        bool
	visualizerStop  chan struct{}
	visualizerPause chan bool
	visualizerOnce  sync.Once
}

func (s *darwinPlaybackSession) run(ctx context.Context, prebuffer *darwinPrebufferStore, source provider.Source, seekMS uint64, handler playback.EventHandler) {
	defer close(s.done)

	prepared, err := prebuffer.TakeOrPrepareSource(ctx, source, handler.OnBufferingProgress)
	if err != nil {
		s.done <- err
		return
	}
	defer func() {
		if prepared.File != "" && !prepared.Cached {
			_ = os.Remove(prepared.File)
		}
	}()

	info := prepared.Info
	initialMS := seekMS
	if !prepared.Seekable {
		initialMS = 0
	}
	if totalMS := info.DurationMS; totalMS > 0 && initialMS >= totalMS {
		initialMS = totalMS - 1
	}
	if handler.OnTrackChanged != nil {
		handler.OnTrackChanged(info, prepared.Quality, initialMS)
	}

	// A play call replaces the helper's current track, so a session stopped while preparing must not make one.
	s.mu.Lock()
	s.file = prepared.File
	stopped := s.stopped
	volume := s.volume
	s.mu.Unlock()
	if stopped {
		s.done <- context.Canceled
		return
	}

	token, finishCh, err := s.player.play(prepared.File, volume, initialMS)
	if err != nil {
		s.done <- err
		return
	}
	s.mu.Lock()
	s.token = token
	paused := s.paused
	stopped = s.stopped
	s.mu.Unlock()

	if stopped {
		_ = s.player.stop(token)
		s.done <- context.Canceled
		return
	}
	if paused {
		_ = s.player.pause(token)
	}

	if handler.OnPlaybackProgress != nil {
		handler.OnPlaybackProgress(initialMS, info.DurationMS)
	}
	if handler.OnAudioBands != nil {
		stop := make(chan struct{})
		pause := make(chan bool, 1)
		s.mu.Lock()
		startVisualizer := !s.stopped
		if startVisualizer {
			s.visualizerStop = stop
			s.visualizerPause = pause
			if s.paused {
				pause <- true
			}
		}
		s.mu.Unlock()
		if startVisualizer {
			go player.StreamVisualizerFile(prepared.File, prepared.Format, initialMS, stop, pause, handler.OnAudioBands)
			defer s.stopVisualizer()
		}
	}

	err = <-finishCh
	s.mu.Lock()
	stopped = s.stopped
	s.finished = true
	s.mu.Unlock()
	if stopped {
		s.done <- context.Canceled
		return
	}
	if err != nil {
		if handler.OnError != nil {
			handler.OnError(err)
		}
		s.done <- err
		return
	}
	if handler.OnPlaybackStopped != nil {
		handler.OnPlaybackStopped()
	}
	s.done <- nil
}

func (s *darwinPlaybackSession) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = true
	s.sendVisualizerPauseLocked(true)
	if s.token != 0 {
		_ = s.player.pause(s.token)
	}
}

func (s *darwinPlaybackSession) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = false
	s.sendVisualizerPauseLocked(false)
	if s.token != 0 {
		_ = s.player.resume(s.token)
	}
}

func (s *darwinPlaybackSession) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	s.stopVisualizerLocked()
	if s.finished {
		return
	}
	if s.token != 0 {
		_ = s.player.stop(s.token)
	}
}

func (s *darwinPlaybackSession) Seek(uint64) error { return playback.ErrSeekUnsupported }

func (s *darwinPlaybackSession) stopVisualizer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopVisualizerLocked()
}

func (s *darwinPlaybackSession) stopVisualizerLocked() {
	if s.visualizerStop == nil {
		return
	}
	s.visualizerOnce.Do(func() {
		close(s.visualizerStop)
	})
}

func (s *darwinPlaybackSession) sendVisualizerPauseLocked(paused bool) {
	if s.visualizerPause == nil {
		return
	}
	select {
	case s.visualizerPause <- paused:
	default:
		select {
		case <-s.visualizerPause:
		default:
		}
		s.visualizerPause <- paused
	}
}

func (s *darwinPlaybackSession) FadeOutStop(duration time.Duration) {
	if duration <= 0 {
		s.Stop()
		return
	}

	steps := 20
	stepDuration := duration / time.Duration(steps)
	if stepDuration <= 0 {
		stepDuration = time.Millisecond
	}
	for step := steps - 1; step >= 0; step-- {
		s.SetVolume(float32(step) / float32(steps))
		time.Sleep(stepDuration)
	}
	s.Stop()
}

func (s *darwinPlaybackSession) Wait() error {
	return <-s.done
}

func (s *darwinPlaybackSession) SetVolume(v float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	s.volume = v
	if s.token != 0 {
		_ = s.player.setVolume(s.token, v)
	}
}

type darwinHelperManager struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	waitErrCh   chan error
	nextToken   int
	current     *darwinPlayHandle
	mediaEvents chan playback.MediaCommand
}

type darwinPlayHandle struct {
	token int
	done  chan error
}

func newDarwinHelperManager() *darwinHelperManager {
	return &darwinHelperManager{
		nextToken:   1,
		mediaEvents: make(chan playback.MediaCommand, 16),
	}
}

func (m *darwinHelperManager) startAsync() {
	go func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		_ = m.ensureStartedLocked()
	}()
}

func (m *darwinHelperManager) close() {
	m.mu.Lock()
	if m.stdin != nil {
		_ = m.sendLocked("quit")
		_ = m.stdin.Close()
		m.stdin = nil
	}
	cmd, done := m.cmd, m.waitErrCh
	m.mu.Unlock()
	if cmd == nil || cmd.Process == nil || done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	m.mu.Lock()
	if m.cmd == cmd {
		m.cmd = nil
		m.waitErrCh = nil
	}
	m.mu.Unlock()
}

func (m *darwinHelperManager) play(path string, volume float32, seekMS uint64) (int, <-chan error, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ensureStartedLocked(); err != nil {
		return 0, nil, err
	}

	if m.current != nil {
		select {
		case m.current.done <- context.Canceled:
		default:
		}
	}

	token := m.nextToken
	m.nextToken++
	handle := &darwinPlayHandle{
		token: token,
		done:  make(chan error, 1),
	}
	m.current = handle
	if err := m.sendLocked(fmt.Sprintf("play\t%d\t%.4f\t%d\t%s", token, volume, seekMS, path)); err != nil {
		return 0, nil, err
	}
	return token, handle.done, nil
}

func (m *darwinHelperManager) pause(token int) error {
	return m.simpleCommand(token, "pause")
}

func (m *darwinHelperManager) resume(token int) error {
	return m.simpleCommand(token, "resume")
}

func (m *darwinHelperManager) stop(token int) error {
	return m.simpleCommand(token, "stop")
}

func (m *darwinHelperManager) setVolume(token int, volume float32) error {
	return m.simpleCommand(token, "volume\t"+strconv.FormatFloat(float64(volume), 'f', 4, 32))
}

func (m *darwinHelperManager) updateMediaControl(state playback.MediaState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureStartedLocked(); err != nil {
		return
	}

	_ = m.sendLocked(strings.Join([]string{
		"nowplaying",
		sanitizeHelperField(state.TrackID),
		sanitizeHelperField(state.Title),
		sanitizeHelperField(state.Artist),
		strconv.FormatUint(state.DurationMS, 10),
		strconv.FormatUint(state.PositionMS, 10),
		sanitizeHelperField(state.AlbumArtURL),
	}, "\t"))

	stateName := "stopped"
	if state.Playing {
		stateName = "playing"
	} else if !state.Stopped {
		stateName = "paused"
	}
	_ = m.sendLocked("state\t" + stateName)
	_ = m.sendLocked(fmt.Sprintf("position\t%d\t%d", state.PositionMS, state.DurationMS))
	_ = m.sendLocked(fmt.Sprintf("capabilities\t%s\t%s\t%s",
		helperBool(state.CanNext),
		helperBool(state.CanPrevious),
		helperBool(state.CanSeek),
	))
}

func helperBool(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func sanitizeHelperField(value string) string {
	replacer := strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")
	return replacer.Replace(value)
}

func (m *darwinHelperManager) simpleCommand(token int, cmd string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ensureStartedLocked(); err != nil {
		return err
	}
	if m.current == nil || m.current.token != token {
		return nil
	}
	return m.sendLocked(cmd)
}

func (m *darwinHelperManager) ensureStartedLocked() error {
	if m.stdin != nil && m.cmd != nil && m.cmd.Process != nil {
		return nil
	}

	helperPath, err := ensureMacPlayerHelper()
	if err != nil {
		return err
	}

	cmd := exec.Command(helperPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	m.cmd = cmd
	m.stdin = stdin
	m.waitErrCh = make(chan error, 1)
	go m.readLoop(stdout)
	go m.stderrLoop(stderr)
	go func() {
		m.waitErrCh <- cmd.Wait()
		close(m.waitErrCh)
	}()
	return nil
}

func (m *darwinHelperManager) sendLocked(line string) error {
	if m.stdin == nil {
		return errors.New("mac helper stdin is not available")
	}
	_, err := io.WriteString(m.stdin, line+"\n")
	return err
}

func (m *darwinHelperManager) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		switch parts[0] {
		case "remote":
			m.emitRemoteCommand(parts)
		case "finished":
			token, _ := strconv.Atoi(parts[1])
			m.finishToken(token, nil)
		case "stopped":
			token, _ := strconv.Atoi(parts[1])
			m.finishToken(token, context.Canceled)
		case "error":
			token, _ := strconv.Atoi(parts[1])
			msg := "mac player helper error"
			if len(parts) > 2 {
				msg = parts[2]
			}
			m.finishToken(token, errors.New(msg))
		}
	}
	if err := scanner.Err(); err != nil {
		m.finishCurrent(err)
	}
}

func (m *darwinHelperManager) emitRemoteCommand(parts []string) {
	if len(parts) < 2 {
		return
	}
	command := playback.MediaCommand{}
	switch parts[1] {
	case "play":
		command.Kind = playback.MediaPlay
	case "pause":
		command.Kind = playback.MediaPause
	case "toggle":
		command.Kind = playback.MediaToggle
	case "next":
		command.Kind = playback.MediaNext
	case "previous":
		command.Kind = playback.MediaPrevious
	case "setPosition":
		command.Kind = playback.MediaSetPosition
		if len(parts) > 2 {
			command.PositionMS, _ = strconv.ParseUint(parts[2], 10, 64)
		}
	default:
		return
	}
	select {
	case m.mediaEvents <- command:
	default:
	}
}

func (m *darwinHelperManager) stderrLoop(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		msg := strings.TrimSpace(scanner.Text())
		if msg != "" {
			m.finishCurrent(errors.New(msg))
		}
	}
}

func (m *darwinHelperManager) finishToken(token int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil || m.current.token != token {
		return
	}
	select {
	case m.current.done <- err:
	default:
	}
	m.current = nil
}

func (m *darwinHelperManager) finishCurrent(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return
	}
	select {
	case m.current.done <- err:
	default:
	}
	m.current = nil
}

type darwinPreparedTrack struct {
	TrackID  string
	Quality  core.Quality
	Info     core.TrackInfo
	Format   core.AudioFormat
	Seekable bool
	File     string
	Cached   bool
}

type darwinPrebufferJob struct {
	track    playback.Track
	done     chan struct{}
	result   *darwinPreparedTrack
	err      error
	cancel   context.CancelFunc
	onStatus func(playback.Track, playback.PrebufferStatus)
}

type darwinPrebufferRequest struct {
	key      string
	track    playback.Track
	onStatus func(playback.Track, playback.PrebufferStatus)
}

type darwinPrebufferStore struct {
	mu       sync.Mutex
	current  map[string]*darwinPreparedTrack
	order    []string
	inflight map[string]*darwinPrebufferJob
	pending  []darwinPrebufferRequest
	closed   bool
}

const (
	maxDarwinPrebufferInflight = 2
	maxDarwinPrebufferCached   = 20
)

func newDarwinPrebufferStore() *darwinPrebufferStore {
	return &darwinPrebufferStore{
		current:  map[string]*darwinPreparedTrack{},
		inflight: map[string]*darwinPrebufferJob{},
	}
}

func (s *darwinPrebufferStore) Prebuffer(ctx context.Context, tracks []playback.Track, onStatus func(playback.Track, playback.PrebufferStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	if len(tracks) == 0 {
		s.cancelPendingLocked()
		return
	}

	keep := map[string]playback.Track{}
	ordered := make([]string, 0, len(tracks))
	for _, track := range tracks {
		if strings.TrimSpace(track.ID) == "" {
			continue
		}
		key := darwinPrebufferKey(track)
		if _, ok := keep[key]; ok {
			continue
		}
		keep[key] = track
		ordered = append(ordered, key)
	}
	if len(keep) == 0 {
		s.cancelPendingLocked()
		return
	}

	for key, job := range s.inflight {
		if _, ok := keep[key]; !ok {
			job.cancel()
			delete(s.inflight, key)
		}
	}
	s.retainPendingLocked(keep)

	for _, key := range ordered {
		track := keep[key]
		if _, ok := s.current[key]; ok {
			sendPrebufferStatus(onStatus, track, playback.PrebufferReady)
			continue
		}
		if _, ok := s.inflight[key]; ok {
			continue
		}
		if s.pendingContainsLocked(key) {
			continue
		}
		s.pending = append(s.pending, darwinPrebufferRequest{
			key:      key,
			track:    track,
			onStatus: onStatus,
		})
	}
	s.startPendingLocked(ctx)
}

func (s *darwinPrebufferStore) TakeOrPrepareSource(ctx context.Context, source provider.Source, onBufferingProgress func(uint8, playback.BufferingStage)) (*darwinPreparedTrack, error) {
	key := darwinSourcePrebufferKey(source)
	for {
		s.mu.Lock()
		if prepared, ok := s.current[key]; ok {
			prepared.Cached = true
			s.rememberCachedLocked(key)
			s.mu.Unlock()
			if onBufferingProgress != nil {
				onBufferingProgress(100, playback.BufferingStageReady)
			}
			return prepared, nil
		}
		if job, ok := s.inflight[key]; ok {
			s.mu.Unlock()
			if onBufferingProgress != nil {
				onBufferingProgress(80, playback.BufferingStageDownloading)
			}
			<-job.done
			if job.err != nil {
				return nil, job.err
			}
			continue
		}
		s.mu.Unlock()
		prepared, err := prepareDarwinSourceFile(ctx, source, onBufferingProgress)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		if _, ok := s.current[key]; ok {
			if prepared.File != "" {
				_ = os.Remove(prepared.File)
			}
			s.mu.Unlock()
			continue
		}
		if s.closed {
			if prepared.File != "" {
				_ = os.Remove(prepared.File)
			}
			s.mu.Unlock()
			return nil, context.Canceled
		}
		if job, ok := s.inflight[key]; ok {
			job.cancel()
			delete(s.inflight, key)
		}
		prepared.Cached = true
		s.current[key] = prepared
		s.rememberCachedLocked(key)
		s.mu.Unlock()
		return prepared, nil
	}
}

func (s *darwinPrebufferStore) runJob(ctx context.Context, job *darwinPrebufferJob) {
	defer close(job.done)

	result, err := prepareDarwinTrackFile(ctx, job.track, nil)
	s.mu.Lock()
	defer s.mu.Unlock()

	key := darwinPrebufferKey(job.track)
	if s.closed || s.inflight[key] != job {
		if result != nil && result.File != "" {
			_ = os.Remove(result.File)
		}
		return
	}
	delete(s.inflight, key)
	job.err = err
	job.result = result
	if err != nil {
		sendPrebufferStatus(job.onStatus, job.track, playback.PrebufferFailed)
		s.startPendingLocked(context.Background())
		return
	}
	if old := s.current[key]; old != nil && old.File != "" {
		_ = os.Remove(old.File)
	}
	s.current[key] = result
	s.rememberCachedLocked(key)
	sendPrebufferStatus(job.onStatus, job.track, playback.PrebufferReady)
	s.startPendingLocked(context.Background())
}

func (s *darwinPrebufferStore) cancelPendingLocked() {
	for key, job := range s.inflight {
		job.cancel()
		delete(s.inflight, key)
	}
	s.pending = nil
}

func (s *darwinPrebufferStore) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	s.cancelPendingLocked()
	for key, prepared := range s.current {
		if prepared != nil && prepared.File != "" {
			_ = os.Remove(prepared.File)
		}
		delete(s.current, key)
	}
	s.order = nil
}

func (s *darwinPrebufferStore) rememberCachedLocked(key string) {
	s.removeCachedOrderLocked(key)
	s.order = append(s.order, key)
	for len(s.order) > maxDarwinPrebufferCached {
		evict := s.order[0]
		s.order = s.order[1:]
		if prepared := s.current[evict]; prepared != nil {
			if prepared.File != "" {
				_ = os.Remove(prepared.File)
			}
			delete(s.current, evict)
		}
	}
}

func (s *darwinPrebufferStore) removeCachedOrderLocked(key string) {
	for i, existing := range s.order {
		if existing == key {
			s.order = append(s.order[:i], s.order[i+1:]...)
			return
		}
	}
}

func (s *darwinPrebufferStore) startPendingLocked(ctx context.Context) {
	for len(s.pending) > 0 && len(s.inflight) < maxDarwinPrebufferInflight {
		req := s.pending[0]
		s.pending = s.pending[1:]
		if _, ok := s.current[req.key]; ok {
			sendPrebufferStatus(req.onStatus, req.track, playback.PrebufferReady)
			continue
		}
		if _, ok := s.inflight[req.key]; ok {
			continue
		}
		jobCtx, cancel := context.WithCancel(ctx)
		job := &darwinPrebufferJob{
			track:    req.track,
			done:     make(chan struct{}),
			cancel:   cancel,
			onStatus: req.onStatus,
		}
		s.inflight[req.key] = job
		sendPrebufferStatus(req.onStatus, req.track, playback.PrebufferLoading)
		go s.runJob(jobCtx, job)
	}
}

func (s *darwinPrebufferStore) retainPendingLocked(keep map[string]playback.Track) {
	if len(s.pending) == 0 {
		return
	}
	next := s.pending[:0]
	for _, req := range s.pending {
		if _, ok := keep[req.key]; ok {
			next = append(next, req)
		}
	}
	s.pending = next
}

func (s *darwinPrebufferStore) pendingContainsLocked(key string) bool {
	for _, req := range s.pending {
		if req.key == key {
			return true
		}
	}
	return false
}

func darwinPrebufferKey(track playback.Track) string {
	return track.Quality.ID + "\x00" + track.ID
}

func darwinSourcePrebufferKey(source provider.Source) string {
	return source.Quality.ID + "\x00" + source.Info.ID
}

func sendPrebufferStatus(onStatus func(playback.Track, playback.PrebufferStatus), track playback.Track, status playback.PrebufferStatus) {
	if onStatus != nil {
		onStatus(track, status)
	}
}

// prepareDarwinTrackFile writes the resolved track to a temp file, because the Swift helper plays files.
func prepareDarwinTrackFile(ctx context.Context, track playback.Track, onBufferingProgress func(uint8, playback.BufferingStage)) (*darwinPreparedTrack, error) {
	report := func(percent uint8, stage playback.BufferingStage) {
		if onBufferingProgress != nil {
			onBufferingProgress(percent, stage)
		}
	}
	if track.Resolve == nil {
		return nil, errors.New("provider does not stream audio")
	}
	report(0, playback.BufferingStageResolving)
	src, err := track.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	return prepareDarwinSourceFile(ctx, src, onBufferingProgress)
}

func prepareDarwinSourceFile(ctx context.Context, src provider.Source, onBufferingProgress func(uint8, playback.BufferingStage)) (*darwinPreparedTrack, error) {
	report := func(percent uint8, stage playback.BufferingStage) {
		if onBufferingProgress != nil {
			onBufferingProgress(percent, stage)
		}
	}
	if src.Bytes == nil {
		return nil, fmt.Errorf("track %s has no byte stream to play", src.Info.ID)
	}
	report(40, playback.BufferingStageDownloading)

	body, err := src.Bytes.OpenRange(ctx, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()

	file, err := os.CreateTemp("", "amp-tui-*"+src.Format.Extension())
	if err != nil {
		return nil, err
	}
	progress := &downloadProgressWriter{total: src.Bytes.Size(), report: report}
	if _, err := io.Copy(io.MultiWriter(file, progress), body); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}
	report(90, playback.BufferingStagePreparing)
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return nil, err
	}
	report(100, playback.BufferingStageReady)

	return &darwinPreparedTrack{
		TrackID:  src.Info.ID,
		Quality:  src.Quality,
		Info:     src.Info,
		Format:   src.Format,
		Seekable: src.Seekable,
		File:     file.Name(),
	}, nil
}

// downloadProgressWriter maps download bytes onto the 15-70% buffering band.
type downloadProgressWriter struct {
	written int64
	total   int64
	report  func(uint8, playback.BufferingStage)
}

func (w *downloadProgressWriter) Write(p []byte) (int, error) {
	w.written += int64(len(p))
	if w.total <= 0 {
		w.report(40, playback.BufferingStageDownloading)
		return len(p), nil
	}
	percent := min(15+int((w.written*55)/w.total), 70)
	w.report(uint8(percent), playback.BufferingStageDownloading)
	return len(p), nil
}

func ensureMacPlayerHelper() (string, error) {
	return mediactl.EnsureHelper()
}
