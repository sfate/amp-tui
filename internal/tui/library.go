package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/provider"
)

func (m *Model) onBootstrapFailed(msg bootstrapFailedMsg) tea.Cmd {
	m.ready = true
	if errors.Is(msg.err, provider.ErrUnauthenticated) {
		m.startLogin()
		m.bootstrapFailed = false
		m.shutdownRuntime()
		m.setProvider(nil)
		m.runtime = nil
		m.app.StatusMessage = fmt.Sprintf("Saved %s session expired or was rejected. Press Enter to sign in again", m.providerName())
	} else {
		m.bootstrapFailed = true
		m.app.StatusMessage = fmt.Sprintf("Bootstrap error: %v. Press Enter to retry or O to sign in", msg.err)
	}
	return nil
}

func (m *Model) onBootstrapLoaded(msg bootstrapLoadedMsg) tea.Cmd {
	m.bootstrapFailed = false
	m.app.Playlists = msg.playlists
	m.app.PlaylistState.Select(intPtr(0))
	m.app.StatusMessage = fmt.Sprintf("Loaded %d playlists", len(msg.playlists))
	return m.loadInitialSection()
}

func (m *Model) onCollectionLoaded(msg collectionLoadedMsg) tea.Cmd {
	if msg.isFlow && msg.append {
		return m.appendFlowTracks(msg)
	}

	m.app.CurrentPlaylistID = stringPtr(msg.id)
	if msg.isFlow {
		m.cancelActiveSearch()
		m.app.FlowNextIndex = len(msg.tracks)
		m.app.IsFlowQueue = true
		m.app.FlowCollectionID = msg.id
		trackID := m.app.LoadFlowTracks(msg.tracks, msg.autoplay)
		m.ready = true
		m.app.StatusMessage = fmt.Sprintf("Loaded %s (%d tracks)", msg.title, len(msg.tracks))
		if m.isInitialSection(msg.id) {
			m.showNotice()
		}
		if trackID != nil {
			_ = m.prebufferQueueWindowFrom(1)
			return m.startTrackPlayback(*trackID)
		}
		return m.prebufferQueueWindowFrom(0)
	}

	m.loadCollection(msg.id, msg.title, msg.tracks)
	m.ready = true
	if m.isInitialSection(msg.id) {
		m.app.ActivePanel = app.ActivePanelNavigation
		m.showNotice()
	}
	return m.prebufferTrackWindowFrom(msg.tracks, 0)
}

func (m *Model) appendFlowTracks(msg collectionLoadedMsg) tea.Cmd {
	oldQueueLen := len(m.app.QueueTracks)
	result := m.app.AppendFlowTracks(msg.tracks, msg.autoplay)
	m.app.FlowLoadingMore = false
	if result.AppendedCount == 0 {
		m.app.StatusMessage = fmt.Sprintf("%s returned no new tracks", msg.title)
		return nil
	}
	m.app.FlowNextIndex += len(msg.tracks)
	m.app.StatusMessage = fmt.Sprintf("Appended %d %s tracks", result.AppendedCount, msg.title)
	if result.AutoplayTrackID != nil {
		return m.startTrackPlayback(*result.AutoplayTrackID)
	}
	var prebufferCmd tea.Cmd
	if oldQueueLen < len(m.app.QueueTracks) {
		prebufferCmd = m.prebufferQueueWindowFrom(oldQueueLen)
	}
	return prebufferCmd
}

func (m *Model) onLoadFailed(msg loadFailedMsg) tea.Cmd {
	m.app.StatusMessage = msg.message
	m.app.FlowLoadingMore = false
	m.app.IsSearching = false
	m.app.SearchLoading = false
	m.search.activeID = 0
	m.ready = true
	return nil
}

func (m *Model) loadCollection(id, title string, tracks []app.Track) {
	wasFlowQueue := m.app.IsFlowQueue
	flowLoadingMore := m.app.FlowLoadingMore
	flowNextIndex := m.app.FlowNextIndex
	m.app.CurrentPlaylistID = stringPtr(id)
	m.app.CurrentTracks = append([]app.Track(nil), tracks...)
	if m.viewingFavorites() {
		m.applyFavoritesSort()
	}
	m.app.MainState.Select(intPtr(0))
	m.app.SearchPlaylists = nil
	m.app.SearchArtists = nil
	m.app.ShowingSearchResult = false
	m.cancelActiveSearch()
	m.app.ViewingSettings = false
	if m.ready {
		m.app.ActivePanel = app.ActivePanelMain
	}
	if len(m.app.QueueTracks) == 0 || m.app.QueueIndex == nil {
		m.app.IsFlowQueue = false
		m.app.FlowLoadingMore = false
		m.app.FlowNextIndex = 0
	} else {
		m.app.IsFlowQueue = wasFlowQueue
		m.app.FlowLoadingMore = flowLoadingMore
		m.app.FlowNextIndex = flowNextIndex
	}
	m.app.StatusMessage = fmt.Sprintf("Loaded %s (%d tracks)", title, len(tracks))
}

func (m *Model) toggleFavoritesSort() bool {
	if !m.viewingFavorites() || len(m.app.CurrentTracks) == 0 {
		return false
	}
	m.favoritesSortAsc = !m.favoritesSortAsc
	m.applyFavoritesSort()
	m.app.MainState.Select(intPtr(0))
	m.app.StatusMessage = fmt.Sprintf("Favorites sorted by added date %s", ternary(m.favoritesSortAsc, "ascending", "descending"))
	return true
}

func (m *Model) applyFavoritesSort() {
	queueOwned := m.currentCollectionOwnsQueue()
	sort.SliceStable(m.app.CurrentTracks, func(i, j int) bool {
		left := m.app.CurrentTracks[i].AddedAtMS
		right := m.app.CurrentTracks[j].AddedAtMS
		if left == nil && right == nil {
			return false
		}
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		if m.favoritesSortAsc {
			return *left < *right
		}
		return *left > *right
	})
	if queueOwned {
		m.app.QueueTracks = append([]app.Track(nil), m.app.CurrentTracks...)
	}
}

func (m Model) currentCollectionOwnsQueue() bool {
	if len(m.app.CurrentTracks) == 0 || len(m.app.QueueTracks) != len(m.app.CurrentTracks) {
		return false
	}
	for i := range m.app.CurrentTracks {
		if m.app.CurrentTracks[i].ID != m.app.QueueTracks[i].ID {
			return false
		}
	}
	return true
}

func (m Model) renderSidebar(width, height int) string {
	nav := []string{sectionHeading("Browse", activePalette.Blue)}
	items := m.app.NavItems
	selectedNav := derefOrZero(m.app.NavState.Selected())
	for i, item := range items {
		selected := i == selectedNav && m.app.ActivePanel == app.ActivePanelNavigation
		nav = append(nav, listRow(item, selected, activePalette.Blue))
	}
	nav = append(nav, "", sectionHeading("Library", activePalette.Purple))
	visiblePlaylists := max(0, height-2-len(nav))
	selectedPlaylist := derefOrZero(m.app.PlaylistState.Selected())
	start := scrollStart(selectedPlaylist, len(m.app.Playlists), visiblePlaylists, 0)
	end := min(len(m.app.Playlists), start+visiblePlaylists)
	for i := start; i < end; i++ {
		pl := m.app.Playlists[i]
		selected := i == derefOrZero(m.app.PlaylistState.Selected()) && m.app.ActivePanel == app.ActivePanelPlaylists
		nav = append(nav, listRow(truncate(pl.Title, 20), selected, activePalette.Purple))
	}
	return m.renderPanel("Library", strings.Join(nav, "\n"), m.app.ActivePanel == app.ActivePanelNavigation || m.app.ActivePanel == app.ActivePanelPlaylists, width, height)
}

func (m Model) renderMain(width, height int) string {
	var title string
	switch {
	case m.app.ViewingSettings:
		title = "Settings"
	case m.app.CurrentPlaylistID != nil:
		title = m.collectionTitle(*m.app.CurrentPlaylistID)
	default:
		title = "Browse"
	}

	lines := []string{}
	if m.app.ViewingSettings {
		lines = append(lines, m.renderSettingsRows()...)
	} else if m.app.SearchLoading {
		lines = append(lines, m.renderSearchLoading(width, height)...)
	} else if m.app.ShowingSearchResult {
		lines = m.appendSearchResultRows(lines, width, height)
	} else if len(m.app.CurrentTracks) == 0 {
		lines = append(lines, "", paint(" No tracks loaded", activePalette.TextMuted, ""))
	} else {
		selected := derefOrZero(m.app.MainState.Selected())
		playAll := trackRow(" Play Collection", selected == 0, activePalette.Yellow)
		lines = append(lines, playAll)
		if m.viewingFavorites() {
			lines = append(lines, paint(fmt.Sprintf(" Sort: added date %s", ternary(m.favoritesSortAsc, "asc", "desc")), activePalette.TextMuted, ""))
		}
		lines = append(lines, "")
		lines = append(lines, tableHeader(" #  Title                               Artist", activePalette.Orange))
		lines = append(lines, separatorLine(58, activePalette.Border))
		visibleRows := max(0, height-2-len(lines))
		selectedTrack := max(0, selected-1)
		start := 0
		if selected > 0 {
			start = scrollStart(selectedTrack, len(m.app.CurrentTracks), visibleRows, 0)
		}
		end := min(len(m.app.CurrentTracks), start+visibleRows)
		for i := start; i < end; i++ {
			track := m.app.CurrentTracks[i]
			label := fmt.Sprintf(" %02d %-35s %s", i+1, truncate(track.Title, 35), truncate(track.Artist, 18))
			lines = append(lines, trackRow(label, selected == i+1, activePalette.Aqua))
		}
	}

	return m.renderPanel(title, strings.Join(lines, "\n"), m.app.ActivePanel == app.ActivePanelMain || m.app.ActivePanel == app.ActivePanelSearch, width, height)
}

// showNotice repeats the one-time notice after loading statuses replaced it.
func (m *Model) showNotice() {
	if m.notice != "" {
		m.app.StatusMessage = m.notice
		m.notice = ""
	}
}
