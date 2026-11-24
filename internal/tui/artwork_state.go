package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

const artworkCacheLimit = 40

type artworkState struct {
	url        string
	ansi       string
	cache      map[string]string
	cacheOrder []string
}

func (a *artworkState) clear() {
	a.ansi = ""
	a.url = ""
}

// loadArtwork shows cached art at once and returns a fetch command otherwise.
func (m *Model) loadArtwork(url *string) tea.Cmd {
	if url == nil || strings.TrimSpace(*url) == "" {
		return nil
	}
	m.artwork.url = *url
	if cached, ok := m.artwork.cache[m.artwork.url]; ok {
		m.artwork.ansi = cached
		return nil
	}
	return fetchArtworkCmd(m.artwork.url, 14, 14)
}

func (m *Model) onArtworkLoaded(msg artworkLoadedMsg) tea.Cmd {
	if msg.url == "" || msg.url != m.artwork.url {
		return nil
	}
	m.rememberArtwork(msg.url, msg.art)
	m.artwork.ansi = msg.art
	return nil
}

func (m *Model) rememberArtwork(url string, art string) {
	if strings.TrimSpace(url) == "" {
		return
	}
	if m.artwork.cache == nil {
		m.artwork.cache = map[string]string{}
	}
	if _, ok := m.artwork.cache[url]; !ok {
		m.artwork.cacheOrder = append(m.artwork.cacheOrder, url)
	}
	m.artwork.cache[url] = art
	for len(m.artwork.cacheOrder) > artworkCacheLimit {
		evict := m.artwork.cacheOrder[0]
		m.artwork.cacheOrder = m.artwork.cacheOrder[1:]
		if evict != url {
			delete(m.artwork.cache, evict)
		}
	}
}

func (m Model) renderArtworkSlot(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	for len(lines) < 7 {
		lines = append(lines, "")
	}
	out := make([]string, 0, height)
	topPad := 1
	bottomPad := 1
	sidePad := 1
	for i := 0; i < topPad; i++ {
		out = append(out, paint(strings.Repeat(" ", width), "", activePalette.Background))
	}
	for i := 0; i < 7; i++ {
		line := fitArtworkLine(lines[i], 14)
		out = append(out, paint(strings.Repeat(" ", sidePad), "", activePalette.Background)+line+paint(strings.Repeat(" ", sidePad), "", activePalette.Background))
	}
	for i := 0; i < bottomPad; i++ {
		out = append(out, paint(strings.Repeat(" ", width), "", activePalette.Background))
	}
	return strings.Join(out, "\n")
}

func fitArtworkLine(line string, width int) string {
	line = stripTrailingSGR(truncate(line, width))
	pad := max(0, width-textWidth(line))
	return line + baseBackgroundReset() + strings.Repeat(" ", pad)
}

func stripTrailingSGR(s string) string {
	if !strings.HasSuffix(s, "m") {
		return s
	}
	idx := strings.LastIndex(s, "\x1b[")
	if idx < 0 {
		return s
	}
	return s[:idx]
}

func defaultArtworkANSI() string {
	lines := []string{
		"  +--------+  ",
		"  |  /\\    |  ",
		"  | /  \\   |  ",
		"  | \\__/ o |  ",
		"  |        |  ",
		"  +--------+  ",
		"    NO ART    ",
	}
	for i, line := range lines {
		lines[i] = paint(fitToWidth(line, 14), activePalette.TextMuted, "")
	}
	return strings.Join(lines, "\n")
}
