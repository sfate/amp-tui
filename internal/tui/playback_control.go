package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/deezer"
)

const (
	seekStepMS               uint64 = 10_000
	manualPlaybackStartDelay        = 90 * time.Millisecond
)

// pendingPlayback holds a debounced start; requestID drops stale scheduledPlaybackMsg ticks.
type pendingPlayback struct {
	requestID    int
	trackID      string
	quality      deezer.AudioQuality
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
	return m.startTrackPlaybackWithQualityAndSeek(trackID, qualityFromConfig(m.app.Config.DefaultQuality), 0, true)
}

func (m *Model) startTrackPlaybackWithQuality(trackID string, quality deezer.AudioQuality, resetRetries bool) tea.Cmd {
	return m.startTrackPlaybackWithQualityAndSeek(trackID, quality, 0, resetRetries)
}

func (m *Model) startTrackPlaybackAt(trackID string, quality deezer.AudioQuality, seekMS uint64, resetRetries bool) tea.Cmd {
	return m.startTrackPlaybackWithQualityAndSeek(trackID, quality, seekMS, resetRetries)
}

func (m *Model) scheduleTrackPlayback(trackID string, quality deezer.AudioQuality, seekMS uint64, resetRetries bool) tea.Cmd {
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

func (m *Model) startTrackPlaybackWithQualityAndSeek(trackID string, quality deezer.AudioQuality, seekMS uint64, resetRetries bool) tea.Cmd {
	if !m.resetForTrackStart(trackID, resetRetries) {
		return nil
	}
	m.currentTrackID = trackID
	m.currentQuality = quality
	m.nextPlaybackID++
	playID := m.nextPlaybackID
	m.app.IsPlaying = true
	m.app.StatusMessage = "Starting playback..."
	enableVisualizer := displayMode(m.app.Config) != config.DisplayModeOff
	return tea.Batch(
		startPlaybackCmdWithEvents(playID, trackID, m.runtime, quality, seekMS, enableVisualizer, m.playbackEvents),
		m.maybeLoadMoreFlowForTail(),
	)
}

func (m *Model) seekCurrentTrack(deltaMS int64) tea.Cmd {
	if m.app.NowPlaying == nil || strings.TrimSpace(m.currentTrackID) == "" {
		m.app.StatusMessage = "Nothing playing"
		return nil
	}
	if m.currentQuality == deezer.AudioQualityFlac {
		m.app.StatusMessage = "Seeking is not supported for FLAC playback"
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
	m.app.StatusMessage = fmt.Sprintf("Seeking to %s", formatClock(seekMS))
	return m.startTrackPlaybackAt(m.currentTrackID, m.currentQuality, seekMS, true)
}

func (m *Model) seekCurrentTrackTo(positionMS uint64) tea.Cmd {
	if m.app.NowPlaying == nil || strings.TrimSpace(m.currentTrackID) == "" {
		m.app.StatusMessage = "Nothing playing"
		return nil
	}
	if m.currentQuality == deezer.AudioQualityFlac {
		m.app.StatusMessage = "Seeking is not supported for FLAC playback"
		return nil
	}
	if total := m.app.NowPlaying.TotalMS; total > 0 && positionMS >= total {
		positionMS = total - 1
	}
	m.app.StatusMessage = fmt.Sprintf("Seeking to %s", formatClock(positionMS))
	return m.startTrackPlaybackAt(m.currentTrackID, m.currentQuality, positionMS, true)
}

func (m *Model) switchCurrentQuality(direction int) tea.Cmd {
	if m.app.NowPlaying == nil || strings.TrimSpace(m.currentTrackID) == "" {
		m.app.StatusMessage = "Nothing playing"
		return nil
	}
	next := nextDeezerQuality(m.currentQuality, direction)
	if next == m.currentQuality {
		m.app.StatusMessage = fmt.Sprintf("Quality: %s", qualityLabel(configQualityFromDeezer(m.currentQuality)))
		return nil
	}
	seekMS := m.currentPlaybackPositionMS()
	if next == deezer.AudioQualityFlac {
		seekMS = 0
	}
	m.app.StatusMessage = fmt.Sprintf("Quality: %s", qualityLabel(configQualityFromDeezer(next)))
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

	if lower, ok := lowerPlaybackQuality(m.currentQuality); ok {
		m.playbackRetries++
		m.app.StatusMessage = fmt.Sprintf("Playback error, retrying at %s: %v", qualityLabel(configQualityFromDeezer(lower)), err)
		return m.startTrackPlaybackWithQuality(trackID, lower, false)
	}

	m.app.IsPlaying = false
	m.app.StatusMessage = fmt.Sprintf("Playback error: %v", err)
	return nil
}

func lowerPlaybackQuality(current deezer.AudioQuality) (deezer.AudioQuality, bool) {
	switch current {
	case deezer.AudioQualityFlac:
		return deezer.AudioQuality320, true
	case deezer.AudioQuality320:
		return deezer.AudioQuality128, true
	default:
		return "", false
	}
}

func (m *Model) stopCurrentSession() {
	if m.session == nil {
		return
	}
	if m.app.Config.CrossfadeEnabled && m.app.Config.CrossfadeDurationMS > 0 {
		if session, ok := m.session.(fadeStoppingSession); ok {
			go session.FadeOutStop(time.Duration(m.app.Config.CrossfadeDurationMS) * time.Millisecond)
			return
		}
	}
	m.session.Stop()
}

func (m *Model) shutdownRuntime() {
	if runtime, ok := m.runtime.(ShutdownRuntime); ok {
		runtime.Shutdown()
	}
}

func nextDeezerQuality(current deezer.AudioQuality, direction int) deezer.AudioQuality {
	qualities := []deezer.AudioQuality{deezer.AudioQuality128, deezer.AudioQuality320, deezer.AudioQualityFlac}
	idx := 1
	for i, quality := range qualities {
		if quality == current {
			idx = i
			break
		}
	}
	if direction < 0 {
		idx--
		if idx < 0 {
			idx = 0
		}
	} else if direction > 0 {
		idx = min(idx+1, len(qualities)-1)
	}
	return qualities[idx]
}

func qualityFromConfig(q config.AudioQuality) deezer.AudioQuality {
	switch q {
	case config.AudioQuality128:
		return deezer.AudioQuality128
	case config.AudioQualityFlac:
		return deezer.AudioQualityFlac
	default:
		return deezer.AudioQuality320
	}
}

func configQualityFromDeezer(q deezer.AudioQuality) config.AudioQuality {
	switch q {
	case deezer.AudioQuality128:
		return config.AudioQuality128
	case deezer.AudioQualityFlac:
		return config.AudioQualityFlac
	default:
		return config.AudioQuality320
	}
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
