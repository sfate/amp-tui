package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/playback"
)

func (m *Model) togglePlayPause() {
	if m.app.IsPlaying {
		if m.session != nil {
			m.session.Pause()
		} else {
			m.pauseRequested = true
		}
		if m.progress.active && m.app.NowPlaying != nil {
			elapsed := uint64(time.Since(m.progress.since).Milliseconds())
			m.progress.baseMS += elapsed
			m.app.NowPlaying.CurrentMS = m.progress.baseMS
		}
		m.progress.active = false
		m.progress.since = time.Time{}
		m.app.IsPlaying = false
		m.app.StatusMessage = "Paused"
		m.visualizer.reset()
		m.syncMediaControl()
		return
	}
	if m.session != nil {
		m.pauseRequested = false
		m.session.Resume()
		m.app.IsPlaying = true
		if m.app.NowPlaying != nil {
			m.progress.since = time.Now()
			m.progress.active = true
		}
		m.app.StatusMessage = "Playing"
		m.syncMediaControl()
		if m.progress.active {
			return
		}
		return
	}
}

func (m *Model) handleMediaControlCommand(command playback.MediaCommand) tea.Cmd {
	switch command.Kind {
	case playback.MediaPlay:
		if !m.app.IsPlaying {
			return m.playCurrentQueueTrackOrResume()
		}
	case playback.MediaPause:
		if m.app.IsPlaying {
			m.togglePlayPause()
		}
	case playback.MediaToggle:
		if m.session != nil || m.app.IsPlaying {
			m.togglePlayPause()
			return nil
		}
		return m.playCurrentQueueTrackOrResume()
	case playback.MediaStop:
		if m.session != nil {
			m.session.Stop()
			m.session = nil
		}
		m.nextPlaybackID++
		m.currentPlayID = 0
		m.app.IsPlaying = false
		m.progress.active = false
		m.app.AutoTransitionArmed = false
		m.app.StatusMessage = "Stopped"
		m.syncMediaControl()
	case playback.MediaSetVolume:
		m.setVolume(int(command.Volume*100 + 0.5))
	case playback.MediaSetLoopStatus:
		switch command.LoopStatus {
		case "None":
			m.app.RepeatMode = app.RepeatModeOff
		case "Track":
			m.app.RepeatMode = app.RepeatModeOne
		case "Playlist":
			m.app.RepeatMode = app.RepeatModeAll
		default:
			return nil
		}
		m.app.StatusMessage = fmt.Sprintf("Repeat: %s", repeatModeLabel(m.app.RepeatMode))
		m.syncMediaControl()
	case playback.MediaNext:
		return m.handleNext()
	case playback.MediaPrevious:
		return m.handlePrevious()
	case playback.MediaSetPosition:
		return m.seekCurrentTrackTo(command.PositionMS)
	}
	return nil
}

func (m *Model) playCurrentQueueTrackOrResume() tea.Cmd {
	if m.session != nil {
		m.togglePlayPause()
		return nil
	}
	if m.app.QueueIndex != nil && *m.app.QueueIndex >= 0 && *m.app.QueueIndex < len(m.app.QueueTracks) {
		m.app.QueueState.Select(intPtr(*m.app.QueueIndex))
		return m.startTrackPlayback(m.app.QueueTracks[*m.app.QueueIndex].ID)
	}
	if len(m.app.QueueTracks) > 0 {
		m.app.QueueIndex = intPtr(0)
		m.app.QueueState.Select(intPtr(0))
		return m.startTrackPlayback(m.app.QueueTracks[0].ID)
	}
	return nil
}

func (m Model) listenMediaControlCmd() tea.Cmd {
	media, ok := m.runtime.mediaController()
	if !ok {
		return nil
	}
	return listenMediaControlCmd(media.MediaCommands())
}

func (m Model) syncMediaControl() {
	media, ok := m.runtime.mediaController()
	if !ok {
		return
	}
	state := playback.MediaState{
		Playing:     m.app.IsPlaying,
		Stopped:     !m.app.IsPlaying && m.session == nil,
		Volume:      m.app.Volume,
		CanNext:     m.canGoNext(),
		CanPrevious: m.canGoPrevious(),
		CanSeek:     m.app.NowPlaying != nil && m.currentQuality.Seekable,
		LoopStatus:  mprisLoopStatus(m.app.RepeatMode),
	}
	if now := m.app.NowPlaying; now != nil {
		state.TrackID = now.ID
		state.Title = now.Title
		state.Artist = now.Artist
		state.PositionMS = now.CurrentMS
		state.DurationMS = now.TotalMS
		if now.AlbumArtURL != nil {
			state.AlbumArtURL = *now.AlbumArtURL
		}
	}
	media.UpdateMediaState(state)
}

func mprisLoopStatus(mode app.RepeatMode) string {
	switch mode {
	case app.RepeatModeAll:
		return "Playlist"
	case app.RepeatModeOne:
		return "Track"
	default:
		return "None"
	}
}

func (m Model) canGoNext() bool {
	if m.app.QueueIndex == nil || len(m.app.QueueTracks) == 0 {
		return false
	}
	current := *m.app.QueueIndex
	return current+1 < len(m.app.QueueTracks) || m.app.ShouldLoadMoreFlow() || m.app.RepeatMode == app.RepeatModeAll || m.app.RepeatMode == app.RepeatModeOne
}

func (m Model) canGoPrevious() bool {
	if m.app.QueueIndex == nil || len(m.app.QueueTracks) == 0 {
		return false
	}
	return *m.app.QueueIndex > 0 || m.app.RepeatMode == app.RepeatModeAll || m.app.RepeatMode == app.RepeatModeOne
}

func (m *Model) handleNext() tea.Cmd {
	if m.app.QueueIndex == nil || len(m.app.QueueTracks) == 0 {
		return nil
	}
	current := *m.app.QueueIndex
	if current+1 < len(m.app.QueueTracks) {
		nextIndex := current + 1
		m.app.QueueIndex = intPtr(nextIndex)
		m.app.QueueState.Select(intPtr(nextIndex))
		return m.scheduleTrackPlayback(m.app.QueueTracks[nextIndex].ID, m.configuredQuality(), 0, true)
	}
	if cmd := m.loadMoreFlow(true); cmd != nil {
		return cmd
	}
	if m.app.RepeatMode == app.RepeatModeAll && len(m.app.QueueTracks) > 0 {
		m.app.QueueIndex = intPtr(0)
		m.app.QueueState.Select(intPtr(0))
		return m.scheduleTrackPlayback(m.app.QueueTracks[0].ID, m.configuredQuality(), 0, true)
	}
	if m.app.RepeatMode == app.RepeatModeOne && current >= 0 && current < len(m.app.QueueTracks) {
		return m.scheduleTrackPlayback(m.app.QueueTracks[current].ID, m.configuredQuality(), 0, true)
	}
	return nil
}

func (m *Model) handlePrevious() tea.Cmd {
	if m.app.QueueIndex == nil || len(m.app.QueueTracks) == 0 {
		return nil
	}
	current := *m.app.QueueIndex
	if current == 0 {
		return m.startTrackPlayback(m.app.QueueTracks[0].ID)
	}
	prevIndex := current - 1
	m.app.QueueIndex = intPtr(prevIndex)
	m.app.QueueState.Select(intPtr(prevIndex))
	return m.scheduleTrackPlayback(m.app.QueueTracks[prevIndex].ID, m.configuredQuality(), 0, true)
}

func (m *Model) adjustVolume(delta int) {
	m.setVolume(int(m.app.Volume) + delta)
}

func (m *Model) setVolume(next int) {
	if next < 0 {
		next = 0
	}
	if next > 100 {
		next = 100
	}
	m.app.Volume = uint16(next)
	if m.session != nil {
		m.session.SetVolume(float32(m.app.Volume) / 100)
	}
	m.app.StatusMessage = fmt.Sprintf("Volume: %d%%", m.app.Volume)
	m.syncMediaControl()
}

func (m *Model) cycleRepeatMode() {
	m.app.RepeatMode = nextRepeatMode(m.app.RepeatMode)
	m.app.StatusMessage = fmt.Sprintf("Repeat: %s", repeatModeLabel(m.app.RepeatMode))
	m.syncMediaControl()
}

func (m *Model) maybeStartCrossfadeTransition(currentMS, totalMS uint64) tea.Cmd {
	if m.caps.ManagedAudio || !m.app.Config.CrossfadeEnabled || m.app.Config.CrossfadeDurationMS == 0 {
		return nil
	}
	if m.app.AutoTransitionArmed || m.session == nil || m.app.QueueIndex == nil || len(m.app.QueueTracks) == 0 {
		return nil
	}
	if totalMS == 0 || currentMS >= totalMS {
		return nil
	}
	remainingMS := totalMS - currentMS
	if remainingMS > m.app.Config.CrossfadeDurationMS {
		return nil
	}

	cmd := m.nextPlaybackCommandForAutoTransition()
	if cmd == nil {
		return nil
	}
	m.app.AutoTransitionArmed = true
	return cmd
}

func (m *Model) nextPlaybackCommandForAutoTransition() tea.Cmd {
	current := *m.app.QueueIndex
	if current < 0 || current >= len(m.app.QueueTracks) {
		return nil
	}

	if m.app.RepeatMode == app.RepeatModeOne {
		m.app.QueueState.Select(intPtr(current))
		return m.startTrackPlayback(m.app.QueueTracks[current].ID)
	}

	if current+1 < len(m.app.QueueTracks) {
		nextIndex := current + 1
		m.app.QueueIndex = intPtr(nextIndex)
		m.app.QueueState.Select(intPtr(nextIndex))
		return m.startTrackPlayback(m.app.QueueTracks[nextIndex].ID)
	}

	if cmd := m.loadMoreFlow(true); cmd != nil {
		return cmd
	}

	if m.app.RepeatMode == app.RepeatModeAll {
		m.app.QueueIndex = intPtr(0)
		m.app.QueueState.Select(intPtr(0))
		return m.startTrackPlayback(m.app.QueueTracks[0].ID)
	}

	return nil
}

func (m *Model) handlePlaybackFinished() tea.Cmd {
	if m.app.QueueIndex == nil || len(m.app.QueueTracks) == 0 {
		m.app.IsPlaying = false
		m.app.StatusMessage = "Playback finished"
		return nil
	}

	current := *m.app.QueueIndex
	if current < 0 || current >= len(m.app.QueueTracks) {
		m.app.IsPlaying = false
		m.app.StatusMessage = "Playback finished"
		return nil
	}

	if m.app.RepeatMode == app.RepeatModeOne {
		m.app.QueueState.Select(intPtr(current))
		return m.startTrackPlayback(m.app.QueueTracks[current].ID)
	}

	if current+1 < len(m.app.QueueTracks) {
		nextIndex := current + 1
		m.app.QueueIndex = intPtr(nextIndex)
		m.app.QueueState.Select(intPtr(nextIndex))
		return m.startTrackPlayback(m.app.QueueTracks[nextIndex].ID)
	}

	if cmd := m.loadMoreFlow(true); cmd != nil {
		return cmd
	}

	if m.app.RepeatMode == app.RepeatModeAll {
		m.app.QueueIndex = intPtr(0)
		m.app.QueueState.Select(intPtr(0))
		return m.startTrackPlayback(m.app.QueueTracks[0].ID)
	}

	m.app.IsPlaying = false
	m.app.StatusMessage = "Playback finished"
	return nil
}

func repeatModeLabel(mode app.RepeatMode) string {
	switch mode {
	case app.RepeatModeAll:
		return "All"
	case app.RepeatModeOne:
		return "One"
	default:
		return "Off"
	}
}

func nextRepeatMode(mode app.RepeatMode) app.RepeatMode {
	switch mode {
	case app.RepeatModeOff:
		return app.RepeatModeAll
	case app.RepeatModeAll:
		return app.RepeatModeOne
	default:
		return app.RepeatModeOff
	}
}
