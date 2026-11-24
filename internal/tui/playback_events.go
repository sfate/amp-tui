package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/sanitize"
)

// Player events carry a playID; events from a replaced session are ignored.

func (m *Model) onPlaybackStarted(msg playbackStartedMsg) tea.Cmd {
	if msg.playID != m.nextPlaybackID {
		msg.session.Stop()
		return nil
	}
	m.session = msg.session
	m.currentPlayID = msg.playID
	m.session.SetVolume(float32(m.app.Volume) / 100)
	m.app.IsPlaying = true
	m.syncMediaControl()
	m.progress.active = false
	m.progress.bufferingPercent = intPtr(0)
	m.progress.baseMS = 0
	m.progress.since = time.Time{}
	m.app.AutoTransitionArmed = false
	m.app.StatusMessage = "Buffering..."
	m.visualizer.reset()
	if m.pauseRequested {
		m.session.Pause()
		m.app.IsPlaying = false
		m.app.StatusMessage = "Paused"
	}
	return tea.Batch(
		waitPlaybackCmd(msg.playID, msg.session),
		listenPlaybackEventCmd(m.playbackEvents),
	)
}

func (m *Model) onPlaybackStartFailed(msg playbackStartFailedMsg) tea.Cmd {
	if msg.playID != m.nextPlaybackID {
		return nil
	}
	return m.retryPlaybackAfterError(msg.err)
}

func (m *Model) onBufferingProgress(msg bufferingProgressMsg) tea.Cmd {
	if msg.playID != m.currentPlayID && msg.playID != m.nextPlaybackID {
		return listenPlaybackEventCmd(m.playbackEvents)
	}
	if m.progress.active || strings.EqualFold(strings.TrimSpace(m.app.StatusMessage), "Playing") {
		return listenPlaybackEventCmd(m.playbackEvents)
	}
	percent := int(msg.percent)
	m.progress.bufferingPercent = &percent
	m.app.StatusMessage = bufferingStatusMessage(percent, msg.stage)
	return listenPlaybackEventCmd(m.playbackEvents)
}

func (m *Model) onPlaybackTrackChanged(msg playbackTrackChangedMsg) tea.Cmd {
	if msg.playID != m.currentPlayID && msg.playID != m.nextPlaybackID {
		return listenPlaybackEventCmd(m.playbackEvents)
	}
	m.app.NowPlaying = &app.NowPlaying{
		ID:          msg.info.ID,
		Title:       sanitize.Text(msg.info.Title),
		Artist:      sanitize.Text(msg.info.Artist),
		Quality:     msg.quality,
		CurrentMS:   msg.initialMS,
		TotalMS:     msg.info.DurationMS,
		AlbumArtURL: msg.info.AlbumArtURL,
	}
	if _, ok := m.runtime.prebufferer(); ok {
		m.setPrebufferStatus(msg.info.ID, msg.quality, PrebufferStatusReady)
	}
	m.progress.bufferingPercent = nil
	m.progress.baseMS = msg.initialMS
	m.progress.since = time.Now()
	m.progress.active = m.app.IsPlaying
	m.visualizer.reset()
	if m.app.IsPlaying {
		m.app.StatusMessage = "Playing"
	}
	m.syncMediaControl()
	m.artwork.clear()
	nextCmd := listenPlaybackEventCmd(m.playbackEvents)
	if m.progress.active {
		nextCmd = tea.Batch(nextCmd, playbackTickCmd())
	}
	if artworkCmd := m.loadArtwork(msg.info.AlbumArtURL); artworkCmd != nil {
		return tea.Batch(nextCmd, artworkCmd)
	}
	return nextCmd
}

func (m *Model) onPlaybackProgress(msg playbackProgressMsg) tea.Cmd {
	if msg.playID != m.currentPlayID || m.app.NowPlaying == nil {
		return listenPlaybackEventCmd(m.playbackEvents)
	}
	m.app.NowPlaying.CurrentMS = msg.currentMS
	if msg.totalMS > 0 {
		m.app.NowPlaying.TotalMS = msg.totalMS
	}
	m.syncMediaControl()
	m.progress.bufferingPercent = nil
	m.progress.baseMS = msg.currentMS
	m.progress.since = time.Now()
	m.progress.active = m.app.IsPlaying
	m.app.StatusMessage = "Playing"
	crossfadeCmd := m.maybeStartCrossfadeTransition(msg.currentMS, m.app.NowPlaying.TotalMS)
	if crossfadeCmd != nil {
		if m.progress.active {
			return tea.Batch(listenPlaybackEventCmd(m.playbackEvents), playbackTickCmd(), crossfadeCmd)
		}
		return tea.Batch(listenPlaybackEventCmd(m.playbackEvents), crossfadeCmd)
	}
	loadMoreCmd := m.maybePrebufferNextTrack()
	if m.progress.active {
		return tea.Batch(listenPlaybackEventCmd(m.playbackEvents), playbackTickCmd(), loadMoreCmd)
	}
	return tea.Batch(listenPlaybackEventCmd(m.playbackEvents), loadMoreCmd)
}

func (m *Model) onPlaybackError(msg playbackErrorMsg) tea.Cmd {
	if msg.playID != m.currentPlayID && msg.playID != m.nextPlaybackID {
		return listenPlaybackEventCmd(m.playbackEvents)
	}
	return tea.Batch(m.retryPlaybackAfterError(msg.err), listenPlaybackEventCmd(m.playbackEvents))
}

func (m *Model) onPlaybackFinished(msg playbackFinishedMsg) tea.Cmd {
	if msg.playID != m.currentPlayID {
		return nil
	}
	m.session = nil
	m.progress.active = false
	m.progress.bufferingPercent = nil
	m.progress.since = time.Time{}
	m.visualizer.reset()
	if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
		return m.retryPlaybackAfterError(msg.err)
	}
	if errors.Is(msg.err, context.Canceled) {
		m.syncMediaControl()
		return nil
	}
	cmd := m.handlePlaybackFinished()
	m.syncMediaControl()
	return cmd
}

func (m *Model) onPlaybackTick() tea.Cmd {
	if !m.progress.active || !m.app.IsPlaying || m.app.NowPlaying == nil {
		return nil
	}
	elapsed := uint64(time.Since(m.progress.since).Milliseconds())
	current := m.progress.baseMS + elapsed
	if m.app.NowPlaying.TotalMS > 0 && current > m.app.NowPlaying.TotalMS {
		current = m.app.NowPlaying.TotalMS
	}
	m.app.NowPlaying.CurrentMS = current
	m.syncMediaControl()
	crossfadeCmd := m.maybeStartCrossfadeTransition(current, m.app.NowPlaying.TotalMS)
	if crossfadeCmd != nil {
		return tea.Batch(playbackTickCmd(), crossfadeCmd)
	}
	return playbackTickCmd()
}

func (m *Model) onScheduledPlayback(msg scheduledPlaybackMsg) tea.Cmd {
	pending := m.pendingPlayback
	if msg.requestID != pending.requestID || strings.TrimSpace(pending.trackID) == "" {
		return nil
	}
	m.pendingPlayback.trackID = ""
	return m.startTrackPlaybackWithQualityAndSeek(pending.trackID, pending.quality, pending.seekMS, pending.resetRetries)
}
