package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
)

const (
	seekStepMS               uint64 = 10_000
	manualPlaybackStartDelay        = 90 * time.Millisecond
)

// pendingPlayback holds a debounced start; requestID drops stale scheduledPlaybackMsg ticks.
type pendingPlayback struct {
	requestID    int
	trackID      string
	quality      core.Quality
	seekMS       uint64
	resetRetries bool
}

// playbackProgress extrapolates the position between player progress events.
type playbackProgress struct {
	baseMS           uint64
	since            time.Time
	active           bool
	bufferingPercent *int
}

func (m *Model) startTrackPlayback(trackID string) tea.Cmd {
	return m.startTrackPlaybackWithQualityAndSeek(trackID, m.configuredQuality(), 0, true)
}

func (m *Model) startTrackPlaybackWithQuality(trackID string, quality core.Quality, resetRetries bool) tea.Cmd {
	return m.startTrackPlaybackWithQualityAndSeek(trackID, quality, 0, resetRetries)
}

func (m *Model) startTrackPlaybackAt(trackID string, quality core.Quality, seekMS uint64, resetRetries bool) tea.Cmd {
	return m.startTrackPlaybackWithQualityAndSeek(trackID, quality, seekMS, resetRetries)
}

func (m *Model) scheduleTrackPlayback(trackID string, quality core.Quality, seekMS uint64, resetRetries bool) tea.Cmd {
	if !m.resetForTrackStart(trackID, resetRetries) {
		return nil
	}
	m.pendingPlayback.requestID++
	m.pendingPlayback.trackID = ""
	m.currentTrackID = trackID
	m.currentQuality = quality
	m.app.IsPlaying = true
	m.app.StatusMessage = "Starting playback..."
	m.pendingPlayback.requestID++
	m.pendingPlayback.trackID = trackID
	m.pendingPlayback.quality = quality
	m.pendingPlayback.seekMS = seekMS
	m.pendingPlayback.resetRetries = resetRetries
	requestID := m.pendingPlayback.requestID
	return tea.Tick(manualPlaybackStartDelay, func(time.Time) tea.Msg {
		return scheduledPlaybackMsg{requestID: requestID}
	})
}

func (m *Model) startTrackPlaybackWithQualityAndSeek(trackID string, quality core.Quality, seekMS uint64, resetRetries bool) tea.Cmd {
	if !m.resetForTrackStart(trackID, resetRetries) {
		return nil
	}
	m.currentTrackID = trackID
	m.currentQuality = quality
	m.nextPlaybackID++
	playID := m.nextPlaybackID
	m.app.IsPlaying = true
	m.app.StatusMessage = "Starting playback..."
	enableVisualizer := displayMode(m.app.Config) != config.DisplayModeOff && m.runtime.capabilities().Visualizer
	return tea.Batch(
		startPlaybackCmdWithEvents(playID, trackID, m.runtime, quality, seekMS, enableVisualizer, m.playbackEvents),
		m.loadMoreFlow(false),
	)
}

func (m *Model) seekCurrentTrack(deltaMS int64) tea.Cmd {
	if m.app.NowPlaying == nil || strings.TrimSpace(m.currentTrackID) == "" {
		m.app.StatusMessage = "Nothing playing"
		return nil
	}
	if !m.currentQuality.Seekable {
		m.app.StatusMessage = fmt.Sprintf("Seeking is not supported for %s playback", m.currentQuality.Label)
		return nil
	}
	seekMS := m.currentPlaybackPositionMS()
	if deltaMS < 0 {
		step := uint64(-deltaMS)
		if step >= seekMS {
			seekMS = 0
		} else {
			seekMS -= step
		}
	} else {
		seekMS += uint64(deltaMS)
	}
	if total := m.app.NowPlaying.TotalMS; total > 0 && seekMS >= total {
		seekMS = total - 1
	}
	return m.seekTo(seekMS)
}

func (m *Model) seekCurrentTrackTo(positionMS uint64) tea.Cmd {
	if m.app.NowPlaying == nil || strings.TrimSpace(m.currentTrackID) == "" {
		m.app.StatusMessage = "Nothing playing"
		return nil
	}
	if !m.currentQuality.Seekable {
		m.app.StatusMessage = fmt.Sprintf("Seeking is not supported for %s playback", m.currentQuality.Label)
		return nil
	}
	if total := m.app.NowPlaying.TotalMS; total > 0 && positionMS >= total {
		positionMS = total - 1
	}
	return m.seekTo(positionMS)
}

// seekTo moves the playing session when the engine can, else restarts the track at positionMS.
func (m *Model) seekTo(positionMS uint64) tea.Cmd {
	m.app.StatusMessage = fmt.Sprintf("Seeking to %s", formatClock(positionMS))
	if m.session != nil && m.runtime.capabilities().Seek && m.session.Seek(positionMS) == nil {
		m.app.NowPlaying.CurrentMS = positionMS
		m.progress.baseMS = positionMS
		m.progress.since = time.Now()
		m.syncMediaControl()
		return nil
	}
	return m.startTrackPlaybackAt(m.currentTrackID, m.currentQuality, positionMS, true)
}

func (m *Model) switchCurrentQuality(direction int) tea.Cmd {
	if m.app.NowPlaying == nil || strings.TrimSpace(m.currentTrackID) == "" {
		m.app.StatusMessage = "Nothing playing"
		return nil
	}
	next := m.stepQuality(m.currentQuality, direction, false)
	if next.ID == m.currentQuality.ID {
		m.app.StatusMessage = fmt.Sprintf("Quality: %s", qualityLabel(m.currentQuality))
		return nil
	}
	seekMS := m.currentPlaybackPositionMS()
	if !next.Seekable {
		seekMS = 0
	}
	m.app.StatusMessage = fmt.Sprintf("Quality: %s", qualityLabel(next))
	return m.startTrackPlaybackAt(m.currentTrackID, next, seekMS, true)
}

func (m Model) currentPlaybackPositionMS() uint64 {
	if m.app.NowPlaying == nil {
		return 0
	}
	current := m.app.NowPlaying.CurrentMS
	if m.progress.active && m.app.IsPlaying && !m.progress.since.IsZero() {
		current = m.progress.baseMS + uint64(time.Since(m.progress.since).Milliseconds())
	}
	if total := m.app.NowPlaying.TotalMS; total > 0 && current >= total {
		return total - 1
	}
	return current
}

func (m *Model) retryPlaybackAfterError(err error) tea.Cmd {
	trackID := strings.TrimSpace(m.currentTrackID)
	if trackID == "" {
		m.app.IsPlaying = false
		m.app.StatusMessage = fmt.Sprintf("Playback error: %v", err)
		return nil
	}

	if m.playbackRetries == 0 {
		m.playbackRetries++
		m.app.StatusMessage = fmt.Sprintf("Playback error, reloading track: %v", err)
		return m.startTrackPlaybackWithQuality(trackID, m.currentQuality, false)
	}

	if lower, ok := m.lowerPlaybackQuality(m.currentQuality); ok {
		m.playbackRetries++
		m.app.StatusMessage = fmt.Sprintf("Playback error, retrying at %s: %v", qualityLabel(lower), err)
		return m.startTrackPlaybackWithQuality(trackID, lower, false)
	}

	m.app.IsPlaying = false
	m.app.StatusMessage = fmt.Sprintf("Playback error: %v", err)
	return nil
}

// lowerPlaybackQuality is the step below current on the provider ladder.
func (m Model) lowerPlaybackQuality(current core.Quality) (core.Quality, bool) {
	ladder := m.qualityLadder()
	idx := qualityIndex(ladder, current.ID)
	if idx <= 0 {
		return core.Quality{}, false
	}
	return ladder[idx-1], true
}

func (m *Model) stopCurrentSession() {
	if m.session == nil {
		return
	}
	if m.app.Config.CrossfadeEnabled && m.app.Config.CrossfadeDurationMS > 0 {
		go m.session.FadeOutStop(time.Duration(m.app.Config.CrossfadeDurationMS) * time.Millisecond)
		return
	}
	m.session.Stop()
}

func (m *Model) shutdownRuntime() {
	m.runtime.shutdown()
}

func (m Model) qualityLadder() []core.Quality {
	streams, ok := m.provider.(provider.StreamSource)
	if !ok {
		return nil
	}
	return streams.Qualities()
}

// configuredQuality maps the provider settings ID onto the ladder; an unknown ID gives the provider default.
func (m Model) configuredQuality() core.Quality {
	id := m.app.Config.ProviderSettings(string(m.providerID)).DefaultQuality
	streams, ok := m.provider.(provider.StreamSource)
	if !ok {
		return core.Quality{ID: id}
	}
	ladder := streams.Qualities()
	if idx := qualityIndex(ladder, id); idx >= 0 {
		return ladder[idx]
	}
	return streams.DefaultQuality()
}

// stepQuality moves along the provider ladder; wrap is for Settings, the u/i keys stop at the ends.
func (m Model) stepQuality(current core.Quality, direction int, wrap bool) core.Quality {
	ladder := m.qualityLadder()
	if len(ladder) == 0 {
		return current
	}
	idx := qualityIndex(ladder, current.ID)
	if idx < 0 {
		idx = max(0, qualityIndex(ladder, m.configuredQuality().ID))
	}
	switch {
	case direction < 0 && wrap:
		idx = (idx - 1 + len(ladder)) % len(ladder)
	case direction < 0:
		idx = max(idx-1, 0)
	case direction > 0 && wrap:
		idx = (idx + 1) % len(ladder)
	case direction > 0:
		idx = min(idx+1, len(ladder)-1)
	}
	return ladder[idx]
}

func qualityIndex(ladder []core.Quality, id string) int {
	for i, q := range ladder {
		if q.ID == id {
			return i
		}
	}
	return -1
}

// resetForTrackStart stops the current session and clears per-track state; false means nothing can start.
func (m *Model) resetForTrackStart(trackID string, resetRetries bool) bool {
	if strings.TrimSpace(trackID) == "" {
		return false
	}
	if m.runtime == nil {
		m.app.StatusMessage = "Playback runtime is not configured"
		return false
	}
	if m.session != nil {
		m.stopCurrentSession()
		m.session = nil
	}
	m.currentPlayID = 0
	m.progress.active = false
	m.progress.bufferingPercent = intPtr(0)
	m.progress.baseMS = 0
	m.progress.since = time.Time{}
	m.app.AutoTransitionArmed = false
	m.pauseRequested = false
	m.app.NowPlaying = nil
	m.artwork.clear()
	m.visualizer.reset()
	if resetRetries {
		m.playbackRetries = 0
	}
	return true
}
