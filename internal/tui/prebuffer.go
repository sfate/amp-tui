package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback"
)

const (
	prebufferWindowSize       = 20
	flowPrebufferWindowSize   = 6
	prebufferCacheStatusLimit = 20
)

// prebufferState mirrors the runtime prebuffer per track and quality for the queue indicators.
type prebufferState struct {
	statuses map[string]PrebufferStatus
	ready    []string
	ticking  bool
}

func (m *Model) onPrebufferStatus(msg prebufferStatusMsg) tea.Cmd {
	if msg.quality.ID != m.configuredQuality().ID {
		return listenPlaybackEventCmd(m.playbackEvents)
	}
	m.setPrebufferStatus(msg.trackID, msg.quality, msg.status)
	if msg.status == PrebufferStatusLoading && !m.prebuffer.ticking {
		m.prebuffer.ticking = true
		return tea.Batch(listenPlaybackEventCmd(m.playbackEvents), prebufferTickCmd())
	}
	return listenPlaybackEventCmd(m.playbackEvents)
}

func (m *Model) onPrebufferTick() tea.Cmd {
	if !m.hasLoadingPrebuffer() {
		m.prebuffer.ticking = false
		return nil
	}
	m.prebuffer.ticking = true
	m.loadingFrame = (m.loadingFrame + 1) % len(prebufferSpinnerFrames)
	return prebufferTickCmd()
}

var prebufferSpinnerFrames = []string{"◐", "◓", "◑", "◒"}

func (m *Model) maybePrebufferNextTrack() tea.Cmd {
	if m.app.QueueIndex == nil || len(m.app.QueueTracks) == 0 {
		m.cancelPrebufferWindow()
		return nil
	}

	nextIndex := *m.app.QueueIndex + 1
	if nextIndex < 0 || nextIndex >= len(m.app.QueueTracks) {
		if cmd := m.loadMoreFlow(false); cmd != nil {
			return cmd
		}
		if m.app.RepeatMode == app.RepeatModeAll && len(m.app.QueueTracks) > 0 {
			return m.prebufferQueueWindowFrom(0)
		}
		if m.app.RepeatMode == app.RepeatModeOne && *m.app.QueueIndex >= 0 && *m.app.QueueIndex < len(m.app.QueueTracks) {
			return m.prebufferQueueWindowFrom(*m.app.QueueIndex)
		}
		m.cancelPrebufferWindow()
		return nil
	}

	return m.prebufferQueueWindowFrom(nextIndex)
}

func (m *Model) prebufferQueueWindowFrom(start int) tea.Cmd {
	limit := prebufferWindowSize
	if m.app.IsFlowQueue {
		limit = flowPrebufferWindowSize
	}
	return m.prebufferTrackWindowFromLimit(m.app.QueueTracks, start, limit)
}

func (m *Model) prebufferTrackWindowFrom(tracks []app.Track, start int) tea.Cmd {
	return m.prebufferTrackWindowFromLimit(tracks, start, prebufferWindowSize)
}

func (m *Model) prebufferTrackWindowFromLimit(tracks []app.Track, start int, limit int) tea.Cmd {
	prebufferer, ok := m.runtime.prebufferer()
	if !ok {
		return nil
	}
	if start < 0 || start >= len(tracks) {
		prebufferer.Prebuffer(nil, nil)
		return nil
	}
	if limit <= 0 {
		limit = prebufferWindowSize
	}
	end := min(len(tracks), start+limit)
	quality := m.configuredQuality()
	window := make([]playback.Track, 0, end-start)
	for _, track := range tracks[start:end] {
		if strings.TrimSpace(track.ID) == "" {
			continue
		}
		m.setPrebufferStatus(track.ID, quality, PrebufferStatusScheduled)
		window = append(window, m.runtime.track(track.ID, quality))
	}
	prebufferer.Prebuffer(window, prebufferStatusSender(m.playbackEvents))
	if len(window) > 0 {
		return listenPlaybackEventCmd(m.playbackEvents)
	}
	return nil
}

// prebufferStatusSender drops a status when the event queue is full, as the queue indicator is cosmetic.
func prebufferStatusSender(events chan<- tea.Msg) func(playback.Track, playback.PrebufferStatus) {
	return func(track playback.Track, status playback.PrebufferStatus) {
		select {
		case events <- prebufferStatusMsg{trackID: track.ID, quality: track.Quality, status: prebufferStatusFrom(status)}:
		default:
		}
	}
}

func (m *Model) cancelPrebufferWindow() {
	if prebufferer, ok := m.runtime.prebufferer(); ok {
		prebufferer.Prebuffer(nil, nil)
	}
}

func (m *Model) setPrebufferStatus(trackID string, quality core.Quality, status PrebufferStatus) {
	if strings.TrimSpace(trackID) == "" {
		return
	}
	if m.prebuffer.statuses == nil {
		m.prebuffer.statuses = map[string]PrebufferStatus{}
	}
	key := prebufferKey(trackID, quality)
	if current, ok := m.prebuffer.statuses[key]; ok && current == PrebufferStatusReady && status != PrebufferStatusFailed {
		return
	}
	m.prebuffer.statuses[key] = status
	if status == PrebufferStatusReady {
		m.rememberReadyPrebuffer(key)
	}
}

func (m *Model) rememberReadyPrebuffer(key string) {
	for _, existing := range m.prebuffer.ready {
		if existing == key {
			return
		}
	}
	m.prebuffer.ready = append(m.prebuffer.ready, key)
	for len(m.prebuffer.ready) > prebufferCacheStatusLimit {
		evict := m.prebuffer.ready[0]
		m.prebuffer.ready = m.prebuffer.ready[1:]
		if status, ok := m.prebuffer.statuses[evict]; ok && status == PrebufferStatusReady {
			delete(m.prebuffer.statuses, evict)
		}
	}
}

func (m Model) prebufferStatus(trackID string) (PrebufferStatus, bool) {
	if strings.TrimSpace(trackID) == "" || m.prebuffer.statuses == nil {
		return PrebufferStatusScheduled, false
	}
	status, ok := m.prebuffer.statuses[prebufferKey(trackID, m.configuredQuality())]
	return status, ok
}

func (m Model) hasLoadingPrebuffer() bool {
	for _, status := range m.prebuffer.statuses {
		if status == PrebufferStatusLoading {
			return true
		}
	}
	return false
}

func prebufferKey(trackID string, quality core.Quality) string {
	return quality.ID + "\x00" + trackID
}
