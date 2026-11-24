package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) onLoadingTick() tea.Cmd {
	if m.app.SearchLoading {
		m.loadingFrame = (m.loadingFrame + 1) % len(searchLoadingFrames)
		return loadingTickCmd()
	}
	if !m.ready {
		m.loadingFrame = (m.loadingFrame + 1) % len(loadingHeartFrames)
		return loadingTickCmd()
	}
	return nil
}

var loadingHeartFrames = [][]string{
	{
		"    ██    ██    ",
		"  ████████████  ",
		" ██████████████ ",
		"  ████████████  ",
		"   ██████████   ",
		"     ██████     ",
		"       ██       ",
	},
	{
		"   ████  ████   ",
		" ██████████████ ",
		"████████████████",
		" ██████████████ ",
		"  ████████████  ",
		"    ████████    ",
		"      ████      ",
	},
	{
		"  ██████  ██████  ",
		" ████████████████ ",
		"██████████████████",
		" ████████████████ ",
		"  ██████████████  ",
		"    ██████████    ",
		"       ████       ",
	},
	{
		"   ████  ████   ",
		" ██████████████ ",
		"████████████████",
		" ██████████████ ",
		"  ████████████  ",
		"    ████████    ",
		"      ████      ",
	},
}

var loadingWordmark = []string{
	"▄ ▄▖▄▖▄▖▄▖▄▖",
	"▌▌▙▖▙▖▗▘▙▖▙▘",
	"▙▘▙▖▙▖▙▖▙▖▌▌",
}

func (m Model) renderLoadingScreen() string {
	frame := loadingHeartFrames[m.loadingFrame%len(loadingHeartFrames)]
	lines := make([]string, 0, len(frame)+len(loadingWordmark)+3)
	for _, line := range frame {
		lines = append(lines, centerText(paint(line, activePalette.Purple, ""), m.width))
	}
	lines = append(lines, "")
	for _, line := range loadingWordmark {
		lines = append(lines, centerText(paint(line, activePalette.Aqua, ""), m.width))
	}
	lines = append(lines, centerText(paint(strings.TrimSpace(m.app.StatusMessage), activePalette.TextMuted, ""), m.width))
	return verticalCenter(strings.Join(lines, "\n"), m.height)
}
