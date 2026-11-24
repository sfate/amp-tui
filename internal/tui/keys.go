package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/provider"
)

func (m *Model) onKeyPress(msg tea.KeyPressMsg) tea.Cmd {
	if m.login.active {
		return m.handleLoginInput(msg)
	}
	if m.app.IsSearching {
		return m.handleSearchInput(msg)
	}
	if m.bootstrapFailed && msg.String() == "enter" {
		m.bootstrapFailed = false
		m.ready = false
		m.app.StatusMessage = m.loadingStatus()
		return tea.Batch(bootstrapCmd(m.provider), loadingTickCmd())
	}
	if m.gateKey(msg.String()) {
		return nil
	}
	switch msg.String() {
	case "ctrl+c", "q":
		if m.session != nil {
			m.session.Stop()
		}
		m.shutdownRuntime()
		return tea.Quit
	case "esc":
		m.returnToLibrary()
	case "tab":
		if m.app.ActivePanel == app.ActivePanelMain && m.app.ShowingSearchResult {
			m.app.SwitchSearchCategoryRight()
			return nil
		}
		m.cyclePanelForward()
	case "shift+tab":
		m.cyclePanelBackward()
	case "up", "k":
		m.app.HandleUp()
	case "down", "j":
		m.app.HandleDown()
	case "left", "h":
		if m.app.ActivePanel == app.ActivePanelMain && m.app.ViewingSettings {
			m.adjustSelectedSetting(-1)
			return nil
		}
		m.app.HandleLeft()
	case "right", "l":
		if m.app.ActivePanel == app.ActivePanelMain && m.app.ViewingSettings {
			m.adjustSelectedSetting(1)
			return nil
		}
		m.app.HandleRight()
	case "enter":
		return m.handleEnter()
	case " ", "space":
		return m.handleSpacebar()
	case "n":
		return m.handleNext()
	case "p":
		return m.handlePrevious()
	case "o", "O":
		m.startLogin()
	case "r":
		m.cycleRepeatMode()
	case ",":
		return m.seekCurrentTrack(-int64(seekStepMS))
	case ".":
		return m.seekCurrentTrack(int64(seekStepMS))
	case "u":
		return m.switchCurrentQuality(-1)
	case "i":
		return m.switchCurrentQuality(1)
	case "+":
		m.adjustVolume(5)
	case "-":
		m.adjustVolume(-5)
	case "s":
		m.toggleFavoritesSort()
	case "/":
		m.openSearch()
	}
	return nil
}

// returnToLibrary closes search and settings and focuses the navigation panel.
func (m *Model) returnToLibrary() {
	m.app.IsSearching = false
	m.app.SearchLoading = false
	m.search.activeID = 0
	m.app.ViewingSettings = false
	m.app.ActivePanel = app.ActivePanelNavigation
	m.app.StatusMessage = "Library"
}

func (m *Model) cyclePanelForward() {
	switch m.app.ActivePanel {
	case app.ActivePanelNavigation:
		m.app.ActivePanel = app.ActivePanelPlaylists
	case app.ActivePanelPlaylists:
		m.app.ActivePanel = app.ActivePanelQueue
	case app.ActivePanelQueue:
		m.app.ActivePanel = app.ActivePanelMain
	case app.ActivePanelMain:
		m.app.ActivePanel = app.ActivePanelPlayer
	case app.ActivePanelSearch:
		m.app.ActivePanel = app.ActivePanelMain
	default:
		m.app.ActivePanel = app.ActivePanelNavigation
	}
}

func (m *Model) cyclePanelBackward() {
	switch m.app.ActivePanel {
	case app.ActivePanelPlayer, app.ActivePanelPlayerInfo, app.ActivePanelPlayerProgress:
		m.app.ActivePanel = app.ActivePanelMain
	case app.ActivePanelMain:
		m.app.ActivePanel = app.ActivePanelQueue
	case app.ActivePanelQueue:
		m.app.ActivePanel = app.ActivePanelPlaylists
	case app.ActivePanelPlaylists:
		m.app.ActivePanel = app.ActivePanelNavigation
	case app.ActivePanelSearch:
		m.app.ActivePanel = app.ActivePanelQueue
	default:
		m.app.ActivePanel = app.ActivePanelPlayer
	}
}

func (m *Model) handleSpacebar() tea.Cmd {
	if m.session != nil || m.app.IsPlaying {
		m.togglePlayPause()
		return nil
	}

	if track := m.selectedTrack(); track != nil {
		switch m.app.ActivePanel {
		case app.ActivePanelQueue:
			idx := derefOrZero(m.app.QueueState.Selected())
			if idx >= 0 && idx < len(m.app.QueueTracks) {
				m.app.QueueIndex = intPtr(idx)
				m.app.QueueState.Select(intPtr(idx))
				m.app.IsPlaying = true
				m.app.StatusMessage = fmt.Sprintf("Selected %s - %s", track.Title, track.Artist)
				return m.startTrackPlayback(track.ID)
			}
		case app.ActivePanelMain, app.ActivePanelSearch:
			if m.app.ShowingSearchResult && m.app.SearchCategory == app.SearchCategoryTracks {
				return m.playSearchTrack(derefOrZero(m.app.MainState.Selected()))
			}
			if m.app.ActivePanel == app.ActivePanelMain && !m.app.ShowingSearchResult {
				selected := derefOrZero(m.app.MainState.Selected())
				if selected > 0 {
					trackIndex := selected - 1
					if m.currentCollectionOwnsQueue() && trackIndex < len(m.app.QueueTracks) {
						m.app.QueueIndex = intPtr(trackIndex)
						m.app.QueueState.Select(intPtr(trackIndex))
						m.app.IsPlaying = true
						m.app.StatusMessage = fmt.Sprintf("Selected %s - %s", track.Title, track.Artist)
						return m.startTrackPlayback(track.ID)
					}
				}
			}
			m.app.QueueTracks = []app.Track{*track}
			m.app.QueueIndex = intPtr(0)
			m.app.QueueState.Select(intPtr(0))
			m.app.IsPlaying = true
			m.app.IsFlowQueue = false
			m.app.StatusMessage = fmt.Sprintf("Selected %s - %s", track.Title, track.Artist)
			return m.startTrackPlayback(track.ID)
		}
	}

	switch m.app.ActivePanel {
	case app.ActivePanelNavigation, app.ActivePanelPlaylists, app.ActivePanelMain, app.ActivePanelSearch, app.ActivePanelQueue:
		return m.handleEnter()
	default:
		if len(m.app.QueueTracks) > 0 && m.app.QueueIndex != nil && *m.app.QueueIndex < len(m.app.QueueTracks) {
			return m.startTrackPlayback(m.app.QueueTracks[*m.app.QueueIndex].ID)
		}
	}
	return nil
}

func (m *Model) handleEnter() tea.Cmd {
	switch m.app.ActivePanel {
	case app.ActivePanelNavigation:
		index := derefOrZero(m.app.NavState.Selected())
		sections := m.catalog.sections
		switch {
		case index >= 0 && index < len(sections):
			sec := sections[index]
			return loadSectionCmd(m.provider, sec, sec.Kind == provider.SectionRadio)
		case index == len(sections):
			m.app.ViewingSettings = true
			m.app.ActivePanel = app.ActivePanelMain
			m.app.SettingsState.Select(intPtr(0))
			m.app.StatusMessage = "Settings"
		}
	case app.ActivePanelPlaylists:
		if len(m.app.Playlists) == 0 {
			return nil
		}
		idx := derefOrZero(m.app.PlaylistState.Selected())
		if idx >= len(m.app.Playlists) {
			return nil
		}
		pl := m.app.Playlists[idx]
		return loadPlaylistCmd(m.provider, pl.ID, pl.Title)
	case app.ActivePanelQueue:
		if len(m.app.QueueTracks) == 0 {
			return nil
		}
		idx := derefOrZero(m.app.QueueState.Selected())
		if idx < 0 || idx >= len(m.app.QueueTracks) {
			return nil
		}
		m.app.QueueIndex = intPtr(idx)
		m.app.QueueState.Select(intPtr(idx))
		m.app.IsPlaying = true
		m.app.StatusMessage = fmt.Sprintf("Selected %s - %s", m.app.QueueTracks[idx].Title, m.app.QueueTracks[idx].Artist)
		return m.startTrackPlayback(m.app.QueueTracks[idx].ID)
	case app.ActivePanelMain:
		if m.app.ViewingSettings {
			m.adjustSelectedSetting(1)
			return nil
		}
		if m.app.ShowingSearchResult {
			return m.handleSearchResultEnter()
		}
		if len(m.app.CurrentTracks) == 0 {
			return nil
		}
		selected := derefOrZero(m.app.MainState.Selected())
		if selected == 0 {
			m.app.QueueTracks = append([]app.Track(nil), m.app.CurrentTracks...)
			m.app.QueueIndex = intPtr(0)
			m.app.QueueState.Select(intPtr(0))
			m.app.IsPlaying = true
			m.app.IsFlowQueue = m.viewingRadio()
			if m.app.IsFlowQueue {
				m.app.FlowCollectionID = *m.app.CurrentPlaylistID
			}
			m.app.StatusMessage = fmt.Sprintf("Queued %d tracks", len(m.app.QueueTracks))
			return m.startTrackPlayback(m.app.QueueTracks[0].ID)
		}
		trackIndex := selected - 1
		if trackIndex >= 0 && trackIndex < len(m.app.CurrentTracks) {
			track := m.app.CurrentTracks[trackIndex]
			if m.currentCollectionOwnsQueue() && trackIndex < len(m.app.QueueTracks) {
				m.app.QueueIndex = intPtr(trackIndex)
				m.app.QueueState.Select(intPtr(trackIndex))
				m.app.IsPlaying = true
				m.app.StatusMessage = fmt.Sprintf("Selected %s - %s", track.Title, track.Artist)
				return m.startTrackPlayback(track.ID)
			}
			m.app.QueueTracks = []app.Track{track}
			m.app.QueueIndex = intPtr(0)
			m.app.QueueState.Select(intPtr(0))
			m.app.IsPlaying = true
			m.app.IsFlowQueue = false
			m.app.StatusMessage = fmt.Sprintf("Selected %s - %s", track.Title, track.Artist)
			return m.startTrackPlayback(track.ID)
		}
	}
	return nil
}

func (m Model) selectedTrack() *app.Track {
	switch m.app.ActivePanel {
	case app.ActivePanelQueue:
		idx := derefOrZero(m.app.QueueState.Selected())
		if idx >= 0 && idx < len(m.app.QueueTracks) {
			track := m.app.QueueTracks[idx]
			return &track
		}
	case app.ActivePanelMain, app.ActivePanelSearch:
		if len(m.app.CurrentTracks) == 0 {
			return nil
		}
		idx := derefOrZero(m.app.MainState.Selected())
		if m.app.ShowingSearchResult {
			if m.app.SearchCategory != app.SearchCategoryTracks {
				return nil
			}
			if idx >= 0 && idx < len(m.app.CurrentTracks) {
				track := m.app.CurrentTracks[idx]
				return &track
			}
			return nil
		}
		if idx <= 0 {
			return nil
		}
		trackIdx := idx - 1
		if trackIdx >= 0 && trackIdx < len(m.app.CurrentTracks) {
			track := m.app.CurrentTracks[trackIdx]
			return &track
		}
	}
	return nil
}
