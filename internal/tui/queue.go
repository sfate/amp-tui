package tui

import (
	"fmt"
	"strings"

	"github.com/sfate/amp-tui/internal/app"
)

func (m Model) renderQueue(width, height int) string {
	contentWidth := max(16, width-4)
	queueIndicatorWidth := 2
	queueRowTextWidth := max(8, contentWidth-2)
	lines := []string{
		kvLine("State", ternary(m.app.IsPlaying, "Playing", "Stopped"), activePalette.Green),
		kvLine("Volume", fmt.Sprintf("%d%%", m.app.Volume), activePalette.Aqua),
		kvLine("Repeat", repeatModeLabel(m.app.RepeatMode), activePalette.Orange),
		kvLine("Flow", fmt.Sprintf("%t", m.app.IsFlowQueue), activePalette.Purple),
	}

	if len(m.app.QueueTracks) == 0 {
		lines = append(lines, "", paint(" Nothing queued", activePalette.TextMuted, ""))
	}

	if len(m.app.QueueTracks) > 0 {
		lines = append(lines, "", sectionHeading("Queue", activePalette.Orange))
		lines = append(lines, separatorLine(contentWidth, activePalette.Border))
		visibleRows := max(0, height-2-len(lines))
		target := -1
		following := 2
		if m.app.ActivePanel == app.ActivePanelQueue {
			target = derefOrZero(m.app.QueueState.Selected())
			following = 0
		} else if m.app.QueueIndex != nil {
			target = *m.app.QueueIndex
		}
		start := scrollStart(target, len(m.app.QueueTracks), visibleRows, following)
		end := min(len(m.app.QueueTracks), start+visibleRows)
		for i := start; i < end; i++ {
			item := m.app.QueueTracks[i].QueueLabel()
			indicator := m.queuePrebufferIndicator(i)
			prefix := fmt.Sprintf(" %02d ", i+1)
			itemWidth := max(1, queueRowTextWidth-textWidth(prefix)-queueIndicatorWidth)
			line := prefix + fitToWidth(item, itemWidth)
			if m.app.QueueIndex != nil && i == *m.app.QueueIndex && m.app.IsPlaying {
				line = playingQueueRow(line, indicator, queueIndicatorWidth)
			} else if m.app.ActivePanel == app.ActivePanelQueue && i == derefOrZero(m.app.QueueState.Selected()) {
				line = selectedQueueRow(line, indicator, queueIndicatorWidth)
			} else if m.app.QueueIndex != nil && i == *m.app.QueueIndex {
				line = queueRow(line, "▶ ", activePalette.Aqua, indicator, queueIndicatorWidth)
			} else {
				line = queueRow(line, "  ", activePalette.Text, indicator, queueIndicatorWidth)
			}
			lines = append(lines, line)
		}
	}

	return m.renderPanel("Queue", strings.Join(lines, "\n"), m.app.ActivePanel == app.ActivePanelQueue || m.app.ActivePanel == app.ActivePanelPlayer || m.app.ActivePanel == app.ActivePanelPlayerInfo || m.app.ActivePanel == app.ActivePanelPlayerProgress, width, height)
}

type queueIndicator struct {
	glyph string
	fg    string
}

func playingQueueRow(text string, indicator queueIndicator, indicatorWidth int) string {
	indicator.glyph = "▶"
	return paint("▶ ", activePalette.BackgroundHard, activePalette.Aqua) +
		paint(strings.TrimLeft(text, " "), activePalette.BackgroundHard, activePalette.Aqua) +
		paint(fitToWidth(indicator.glyph, indicatorWidth), activePalette.BackgroundHard, activePalette.Aqua)
}

func selectedQueueRow(text string, indicator queueIndicator, indicatorWidth int) string {
	return paint("> ", activePalette.Orange, "") +
		paint(strings.TrimLeft(text, " "), activePalette.TextStrong, "") +
		paint(fitToWidth(indicator.glyph, indicatorWidth), indicator.fg, "")
}

func queueRow(text, prefix, accent string, indicator queueIndicator, indicatorWidth int) string {
	return paint(prefix, accent, "") +
		paint(strings.TrimLeft(text, " "), activePalette.Text, "") +
		paint(fitToWidth(indicator.glyph, indicatorWidth), indicator.fg, "")
}

func (m Model) queuePrebufferIndicator(index int) queueIndicator {
	if m.app.QueueIndex != nil && index == *m.app.QueueIndex && m.app.IsPlaying {
		return queueIndicator{glyph: "▶", fg: activePalette.Aqua}
	}
	if index < 0 || index >= len(m.app.QueueTracks) {
		return queueIndicator{glyph: "-", fg: activePalette.TextMuted}
	}
	status, ok := m.prebufferStatus(m.app.QueueTracks[index].ID)
	if !ok {
		return queueIndicator{glyph: "-", fg: activePalette.TextMuted}
	}
	switch status {
	case PrebufferStatusScheduled:
		return queueIndicator{glyph: "○", fg: activePalette.TextMuted}
	case PrebufferStatusLoading:
		return queueIndicator{glyph: prebufferSpinnerFrames[m.loadingFrame%len(prebufferSpinnerFrames)], fg: activePalette.Yellow}
	case PrebufferStatusReady:
		return queueIndicator{glyph: "✓", fg: activePalette.Green}
	case PrebufferStatusFailed:
		return queueIndicator{glyph: "×", fg: activePalette.Orange}
	default:
		return queueIndicator{glyph: "-", fg: activePalette.TextMuted}
	}
}
