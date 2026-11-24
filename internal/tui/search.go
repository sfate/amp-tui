package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/app"
)

const searchTimeout = 20 * time.Second

// searchState tags each request so results of a replaced or cancelled search are dropped.
type searchState struct {
	nextID   int
	activeID int
}

func (m *Model) onSearchLoaded(msg searchLoadedMsg) tea.Cmd {
	if msg.requestID != m.search.activeID {
		return nil
	}
	m.app.IsSearching = false
	m.app.SearchLoading = false
	m.app.SearchQuery = msg.query
	m.app.CurrentTracks = msg.tracks
	m.app.SearchPlaylists = msg.playlists
	m.app.SearchArtists = msg.artists
	m.app.ShowingSearchResult = true
	m.app.SearchCategory = app.SearchCategoryTracks
	m.app.ActivePanel = app.ActivePanelMain
	m.app.MainState.Select(intPtr(0))
	m.app.CurrentPlaylistID = stringPtr(searchCollectionID)
	m.app.StatusMessage = fmt.Sprintf("Search: %q", msg.query)
	return nil
}

func (m *Model) onSearchFailed(msg searchFailedMsg) tea.Cmd {
	if msg.requestID != m.search.activeID {
		return nil
	}
	m.app.IsSearching = false
	m.app.SearchLoading = false
	m.app.StatusMessage = msg.message
	return nil
}

func (m *Model) openSearch() {
	m.app.IsSearching = true
	m.app.SearchQuery = ""
	m.app.SearchLoading = false
	m.search.activeID = 0
	m.app.ActivePanel = app.ActivePanelSearch
	m.app.StatusMessage = "Search: type query and press Enter"
}

var searchLoadingFrames = []string{"⡿", "⣟", "⣯", "⣷", "⣾", "⣽", "⣻", "⢿"}

func (m *Model) handleSearchResultEnter() tea.Cmd {
	selected := derefOrZero(m.app.MainState.Selected())
	switch m.app.SearchCategory {
	case app.SearchCategoryTracks:
		if len(m.app.CurrentTracks) == 0 {
			return nil
		}
		return m.playSearchTrack(selected)
	case app.SearchCategoryPlaylists:
		if len(m.app.SearchPlaylists) == 0 {
			return nil
		}
		if selected >= len(m.app.SearchPlaylists) {
			selected = len(m.app.SearchPlaylists) - 1
		}
		pl := m.app.SearchPlaylists[selected]
		return loadPlaylistCmd(m.provider, pl.ID, pl.Title)
	case app.SearchCategoryArtists:
		if len(m.app.SearchArtists) == 0 {
			return nil
		}
		if selected >= len(m.app.SearchArtists) {
			selected = len(m.app.SearchArtists) - 1
		}
		artist := m.app.SearchArtists[selected]
		return m.startSearch(artist.Name)
	default:
		return nil
	}
}

func (m *Model) cancelActiveSearch() {
	m.search.activeID = 0
	m.app.IsSearching = false
	m.app.SearchLoading = false
}

func (m *Model) playSearchTrack(selected int) tea.Cmd {
	if len(m.app.CurrentTracks) == 0 {
		return nil
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= len(m.app.CurrentTracks) {
		selected = len(m.app.CurrentTracks) - 1
	}
	track := m.app.CurrentTracks[selected]
	m.app.QueueTracks = append([]app.Track(nil), m.app.CurrentTracks...)
	m.app.QueueIndex = intPtr(selected)
	m.app.QueueState.Select(intPtr(selected))
	m.app.IsPlaying = true
	m.app.IsFlowQueue = false
	m.app.StatusMessage = fmt.Sprintf("Queued %d search tracks, selected %s - %s", len(m.app.QueueTracks), track.Title, track.Artist)
	return m.startTrackPlayback(track.ID)
}

func (m *Model) handleSearchInput(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.returnToLibrary()
		return nil
	case "enter":
		query := strings.TrimSpace(m.app.SearchQuery)
		if query == "" {
			m.app.IsSearching = false
			m.app.StatusMessage = "Search query is empty"
			return nil
		}
		return m.startSearch(query)
	case "backspace":
		m.app.SearchQuery = trimLastRune(m.app.SearchQuery)
		return nil
	}
	if len(msg.Text) > 0 {
		m.app.SearchQuery += msg.Text
	}
	return nil
}

func (m *Model) startSearch(query string) tea.Cmd {
	query = strings.TrimSpace(query)
	if query == "" {
		m.app.IsSearching = false
		m.app.SearchLoading = false
		m.app.StatusMessage = "Search query is empty"
		return nil
	}
	if m.provider == nil {
		m.app.IsSearching = false
		m.app.SearchLoading = false
		m.search.activeID = 0
		m.app.StatusMessage = "Search unavailable: no music provider is signed in"
		return nil
	}
	m.search.nextID++
	m.search.activeID = m.search.nextID
	m.app.IsSearching = false
	m.app.SearchLoading = true
	m.app.SearchQuery = query
	m.app.ViewingSettings = false
	m.app.ActivePanel = app.ActivePanelMain
	m.app.CurrentPlaylistID = stringPtr(searchCollectionID)
	m.app.CurrentTracks = nil
	m.app.SearchPlaylists = nil
	m.app.SearchArtists = nil
	m.app.SearchCategory = app.SearchCategoryTracks
	m.app.MainState.Select(intPtr(0))
	m.app.ShowingSearchResult = true
	m.app.StatusMessage = fmt.Sprintf("Searching for %q...", query)
	return tea.Batch(searchCmd(m.provider, m.search.activeID, query), loadingTickCmd())
}

func (m Model) appendSearchResultRows(lines []string, width, height int) []string {
	lines = append(lines, renderSearchTabs(m.app.SearchCategory))
	lines = append(lines, "")
	selected := derefOrZero(m.app.MainState.Selected())
	switch m.app.SearchCategory {
	case app.SearchCategoryTracks:
		trackColumns := searchTrackColumns(max(12, width-2))
		lines = append(lines, tableHeader(formatSearchTrackHeader(trackColumns), activePalette.Orange))
		lines = append(lines, separatorLine(max(12, width-2), activePalette.Border))
		visibleRows := max(0, height-2-len(lines))
		start := scrollStart(selected, len(m.app.CurrentTracks), visibleRows, 0)
		end := min(len(m.app.CurrentTracks), start+visibleRows)
		for i := start; i < end; i++ {
			track := m.app.CurrentTracks[i]
			label := formatSearchTrackRow(i+1, track, trackColumns)
			lines = append(lines, trackRow(label, selected == i, activePalette.Aqua))
		}
		if len(m.app.CurrentTracks) == 0 {
			lines = append(lines, paint(" No tracks found", activePalette.TextMuted, ""))
		}
	case app.SearchCategoryPlaylists:
		lines = append(lines, tableHeader(" Playlist", activePalette.Orange))
		lines = append(lines, separatorLine(41, activePalette.Border))
		visibleRows := max(0, height-2-len(lines))
		start := scrollStart(selected, len(m.app.SearchPlaylists), visibleRows, 0)
		end := min(len(m.app.SearchPlaylists), start+visibleRows)
		for i := start; i < end; i++ {
			pl := m.app.SearchPlaylists[i]
			label := fmt.Sprintf(" %02d %s", i+1, truncate(pl.Title, 40))
			lines = append(lines, trackRow(label, selected == i, activePalette.Purple))
		}
		if len(m.app.SearchPlaylists) == 0 {
			lines = append(lines, paint(" No playlists found", activePalette.TextMuted, ""))
		}
	case app.SearchCategoryArtists:
		lines = append(lines, tableHeader(" Artist", activePalette.Orange))
		lines = append(lines, separatorLine(41, activePalette.Border))
		visibleRows := max(0, height-2-len(lines))
		start := scrollStart(selected, len(m.app.SearchArtists), visibleRows, 0)
		end := min(len(m.app.SearchArtists), start+visibleRows)
		for i := start; i < end; i++ {
			artist := m.app.SearchArtists[i]
			label := fmt.Sprintf(" %02d %s", i+1, truncate(artist.Name, 40))
			lines = append(lines, trackRow(label, selected == i, activePalette.Green))
		}
		if len(m.app.SearchArtists) == 0 {
			lines = append(lines, paint(" No artists found", activePalette.TextMuted, ""))
		}
	}
	return lines
}

func (m Model) renderSearchLoading(width, height int) []string {
	frame := searchLoadingFrames[m.loadingFrame%len(searchLoadingFrames)]
	contentWidth := max(1, width-4)
	lines := []string{
		"",
		centerText(fmt.Sprintf("%s %s", paint(frame, activePalette.Aqua, ""), paint("searching", activePalette.TextMuted, "")), contentWidth),
		"",
	}
	status := fmt.Sprintf("Searching for %q", truncate(m.app.SearchQuery, max(8, contentWidth-18)))
	lines = append(lines, centerText(paint(status, activePalette.TextMuted, ""), contentWidth))
	if len(lines) >= height {
		return lines[:height]
	}
	topPad := max(0, (height-len(lines))/2)
	padded := make([]string, 0, min(height, len(lines)+topPad))
	for range topPad {
		padded = append(padded, "")
	}
	padded = append(padded, lines...)
	if len(padded) > height {
		return padded[:height]
	}
	return padded
}

func (m Model) renderSearchBar() string {
	query := m.app.SearchQuery
	if query == "" && m.app.IsSearching {
		query = paint("_", activePalette.Aqua, "")
	}
	label := paint(" Search ", activePalette.TextMuted, "")
	if m.app.IsSearching || m.app.SearchLoading {
		label = paint(" Search ", activePalette.Orange, "")
	}
	help := paint("tab switch | hjkl move | enter select | space play/pause | / search", activePalette.TextMuted, "")
	return renderLineBox(fmt.Sprintf("%s %s", label, query), help, m.width)
}

type searchTrackColumnWidths struct {
	title  int
	artist int
	album  int
}

func searchTrackColumns(contentWidth int) searchTrackColumnWidths {
	available := max(30, contentWidth-12)
	title := max(12, available*45/100)
	artist := max(8, available*25/100)
	album := max(8, available-title-artist)
	if total := title + artist + album; total > available {
		overflow := total - available
		for overflow > 0 && title > 12 {
			title--
			overflow--
		}
		for overflow > 0 && album > 8 {
			album--
			overflow--
		}
		for overflow > 0 && artist > 8 {
			artist--
			overflow--
		}
	}
	return searchTrackColumnWidths{title: title, artist: artist, album: album}
}

func formatSearchTrackHeader(widths searchTrackColumnWidths) string {
	return fmt.Sprintf(" #  %-*s %-*s %-*s Year", widths.title, "Title", widths.artist, "Artist", widths.album, "Album")
}

func formatSearchTrackRow(index int, track app.Track, widths searchTrackColumnWidths) string {
	return fmt.Sprintf(" %02d %-*s %-*s %-*s %s",
		index,
		widths.title,
		truncate(track.Title, widths.title),
		widths.artist,
		truncate(track.Artist, widths.artist),
		widths.album,
		truncate(emptyFallback(track.Album, "-"), widths.album),
		truncate(emptyFallback(track.Year, "-"), 4),
	)
}

func renderSearchTabs(category app.SearchCategory) string {
	tabs := []struct {
		label string
		value app.SearchCategory
	}{
		{label: "Tracks", value: app.SearchCategoryTracks},
		{label: "Playlists", value: app.SearchCategoryPlaylists},
		{label: "Artists", value: app.SearchCategoryArtists},
	}
	parts := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		label := fmt.Sprintf(" %-9s ", strings.ToUpper(tab.label))
		if tab.value == category {
			label = paint(label, activePalette.Orange, "")
		} else {
			label = paint(label, activePalette.TextMuted, "")
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " | ")
}
