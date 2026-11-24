package tui

import (
	"fmt"
	"strings"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/player"
)

func (m Model) renderCompactStatusColumn(width, height int) string {
	bodyHeight := max(1, height-2)
	body := m.renderStatusBody(width)
	statusBodyLines := lineCount(body)
	if displayMode(m.app.Config) == config.DisplayModeOff || bodyHeight <= statusBodyLines+1 {
		return m.renderStatusPanel(width, height)
	}
	displayHeight := bodyHeight - statusBodyLines - 1
	body += "\n" + m.renderDisplayBody(max(1, width-2), displayHeight)
	return m.renderPanel("Status", body, m.app.IsPlaying || m.app.NowPlaying != nil, width, height)
}

func (m Model) renderPlaybar() string {
	left := paint(m.controlsHint(), activePalette.Text, "")
	stateColor := activePalette.TextMuted
	if m.app.IsPlaying {
		stateColor = activePalette.Orange
	}
	right := fmt.Sprintf("%s | %s",
		paint(fmt.Sprintf("vol %d%%", m.app.Volume), activePalette.Aqua, ""),
		paint(ternary(m.app.IsPlaying, "playing", "paused"), stateColor, ""),
	)
	return renderLineBox(left, right, m.width)
}

func (m Model) renderHeader() string {
	left := " amp-tui "
	center := fmt.Sprintf(" %s ", m.collectionTitle(derefString(m.app.CurrentPlaylistID, "Browse")))
	right := fmt.Sprintf(" %s | q quit ", activePanelLabel(m.app.ActivePanel))
	return renderTripleLine(left, center, right, m.width)
}

func (m Model) renderStatusArea() string {
	if displayMode(m.app.Config) == config.DisplayModeOff || m.width < 64 {
		return m.renderStatusLine()
	}
	gap := 2
	statusWidth := max(30, (m.width-gap)/2)
	displayWidth := max(30, m.width-gap-statusWidth)
	return joinColumns(
		m.renderStatusPanel(statusWidth, 11),
		m.renderDisplayPanel(displayWidth),
	)
}

func (m Model) renderStatusLine() string {
	return m.renderStatusPanel(m.width, 11)
}

func (m Model) renderStatusPanel(width, height int) string {
	body := m.renderStatusBody(width)
	return m.renderPanel("Status", body, m.app.IsPlaying || m.app.NowPlaying != nil, width, height)
}

func (m Model) renderStatusBody(width int) string {
	title := "Nothing playing"
	artist := "-"
	progress := renderProgress(0, 0, statusProgressBarWidth(width))
	source := m.collectionTitle(derefString(m.app.CurrentPlaylistID, "Browse"))
	quality := "-"
	elapsed := "00:00"
	total := "00:00"
	queueInfo := "-"
	if m.app.NowPlaying != nil {
		title = truncate(m.app.NowPlaying.Title, max(20, width-22))
		artist = truncate(m.app.NowPlaying.Artist, max(20, width-22))
		progress = renderProgress(m.app.NowPlaying.CurrentMS, m.app.NowPlaying.TotalMS, statusProgressBarWidth(width))
		quality = qualityLabel(m.app.NowPlaying.Quality)
		elapsed = formatClock(m.app.NowPlaying.CurrentMS)
		total = formatClock(m.app.NowPlaying.TotalMS)
	} else if m.app.QueueIndex != nil && *m.app.QueueIndex < len(m.app.QueueTracks) {
		track := m.app.QueueTracks[*m.app.QueueIndex]
		title = truncate(track.Title, max(20, width-22))
		artist = truncate(track.Artist, max(20, width-22))
	}
	if m.app.QueueIndex != nil && len(m.app.QueueTracks) > 0 {
		queueInfo = fmt.Sprintf("%d/%d", *m.app.QueueIndex+1, len(m.app.QueueTracks))
	}
	lines := []string{
		kvLine("State", m.displayState(), activePalette.Green),
		kvLine("Track", title, activePalette.Yellow),
		kvLine("Artist", artist, activePalette.Aqua),
		kvLine("Progress", progress, activePalette.Text),
		kvLine("Elapsed", elapsed+" / "+total, activePalette.Orange),
		kvLine("Quality", quality, activePalette.Purple),
		kvLine("Source", source, activePalette.Blue),
		kvLine("Queue", queueInfo, activePalette.TextMuted),
	}
	art := m.artwork.ansi
	if art == "" {
		art = defaultArtworkANSI()
	}
	body := joinColumns(
		" ",
		m.renderArtworkSlot(art, 16, 9),
		m.renderTextSlot(strings.Join(lines, "\n"), max(24, width-24), 9, 1, 1),
	)
	return body
}

func renderProgress(currentMS, totalMS uint64, width int) string {
	if width < 4 {
		width = 4
	}
	filled := 0
	if totalMS > 0 {
		filled = int((currentMS * uint64(width)) / totalMS)
		if filled > width {
			filled = width
		}
	}
	filledBar := paint(strings.Repeat("━", filled), activePalette.Orange, "")
	emptyBar := paint(strings.Repeat("-", width-filled), activePalette.Border, "")
	return fmt.Sprintf("[%s%s]", filledBar, emptyBar)
}

func statusProgressBarWidth(panelWidth int) int {
	textSlotWidth := max(24, panelWidth-24)
	valueWidth := max(4, textSlotWidth-30)
	return max(4, min(48, valueWidth))
}

func formatClock(ms uint64) string {
	seconds := ms / 1000
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func bufferingStatusMessage(percent int, stage player.BufferingStage) string {
	if strings.TrimSpace(string(stage)) == "" {
		return fmt.Sprintf("Buffering %d%%", percent)
	}
	return fmt.Sprintf("Buffering %d%% - %s", percent, stage)
}

func activePanelLabel(panel app.ActivePanel) string {
	switch panel {
	case app.ActivePanelNavigation:
		return "nav"
	case app.ActivePanelPlaylists:
		return "playlists"
	case app.ActivePanelSearch:
		return "search"
	case app.ActivePanelMain:
		return "tracks"
	case app.ActivePanelQueue:
		return "queue"
	case app.ActivePanelPlayer, app.ActivePanelPlayerInfo, app.ActivePanelPlayerProgress:
		return "player"
	default:
		return "app"
	}
}

func (m Model) displayState() string {
	status := strings.TrimSpace(m.app.StatusMessage)
	switch {
	case strings.EqualFold(status, "Buffering..."):
		return "Buffering"
	case strings.HasPrefix(status, "Buffering "):
		return status
	case m.progress.bufferingPercent != nil:
		return fmt.Sprintf("Buffering %d%%", *m.progress.bufferingPercent)
	case strings.EqualFold(status, "Paused"):
		return "Paused"
	case strings.EqualFold(status, "Playing"):
		return "Playing"
	case strings.EqualFold(status, "Starting playback..."):
		return "Starting"
	case strings.EqualFold(status, "Playback finished"):
		return "Finished"
	case strings.HasPrefix(status, "Playback error:"):
		return "Playback error"
	case strings.HasPrefix(status, "Playback runtime error:"):
		return "Playback runtime error"
	case strings.EqualFold(status, "Loading more Flow..."):
		return "Loading more Flow"
	case m.pauseRequested:
		return "Paused"
	case m.app.IsPlaying:
		return "Playing"
	case m.session != nil:
		return "Ready"
	default:
		return "Stopped"
	}
}
