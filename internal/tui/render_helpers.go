package tui

import (
	"fmt"
	"strings"

	cellansi "github.com/charmbracelet/x/ansi"

	"github.com/sfate/amp-tui/internal/colorscheme"
)

var activePalette = colorscheme.Lookup(colorscheme.Aetheria).Palette

func (m Model) renderPanel(title, body string, active bool, width, height int) string {
	h := "─"
	v := "│"
	tl := "┌"
	tr := "┐"
	bl := "└"
	br := "┘"
	if active {
		h = "═"
		v = "║"
		tl = "╔"
		tr = "╗"
		bl = "╚"
		br = "╝"
	}
	innerWidth := max(12, width-2)
	bodyHeight := max(1, height-2)
	titleText := " " + truncate(title, max(1, innerWidth-2)) + " "
	borderFG := activePalette.Border
	titleFG := activePalette.TextMuted
	titleBG := activePalette.Background
	panelBG := activePalette.Background
	contentFG := activePalette.Text
	if active {
		borderFG = activePalette.Border
		titleFG = activePalette.Orange
		titleBG = activePalette.Background
		panelBG = activePalette.Background
		contentFG = activePalette.TextStrong
	}
	top := paint(tl, borderFG, panelBG) +
		paint(titleText, titleFG, titleBG) +
		paint(strings.Repeat(h, max(0, innerWidth-textWidth(titleText))), borderFG, panelBG) +
		paint(tr, borderFG, panelBG)
	lines := strings.Split(body, "\n")
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	framed := []string{top}
	for i := 0; i < bodyHeight; i++ {
		framed = append(framed,
			paint(v, borderFG, panelBG)+
				paint(fitToWidth(lines[i], innerWidth), contentFG, panelBG)+
				paint(v, borderFG, panelBG),
		)
	}
	framed = append(framed, paint(bl+strings.Repeat(h, innerWidth)+br, borderFG, panelBG))
	return strings.Join(framed, "\n")
}

func (m Model) renderFixedBlock(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		out = append(out, fitToWidth(lines[i], width))
	}
	return strings.Join(out, "\n")
}

func (m Model) renderTextSlot(content string, width, height, padX, padY int) string {
	innerWidth := max(1, width-(padX*2))
	innerHeight := max(1, height-(padY*2))
	text := m.renderFixedBlock(content, innerWidth, innerHeight)
	lines := strings.Split(text, "\n")
	out := make([]string, 0, height)
	for i := 0; i < padY; i++ {
		out = append(out, strings.Repeat(" ", width))
	}
	for _, line := range lines {
		out = append(out, strings.Repeat(" ", padX)+fitToWidth(line, innerWidth)+strings.Repeat(" ", padX))
	}
	for len(out) < height {
		out = append(out, strings.Repeat(" ", width))
	}
	return strings.Join(out, "\n")
}

func scrollStart(index, total, visible, following int) int {
	if total <= 0 || visible <= 0 || visible >= total || index < 0 {
		return 0
	}
	index = min(max(index, 0), total-1)
	following = max(following, 0)
	start := index + following - visible + 1
	start = max(start, 0)
	return min(start, total-visible)
}

func joinColumns(columns ...string) string {
	split := make([][]string, len(columns))
	maxLines := 0
	widths := make([]int, len(columns))
	for i, col := range columns {
		lines := strings.Split(col, "\n")
		split[i] = lines
		if len(lines) > maxLines {
			maxLines = len(lines)
		}
		for _, line := range lines {
			if textWidth(line) > widths[i] {
				widths[i] = textWidth(line)
			}
		}
	}

	rows := make([]string, 0, maxLines)
	for row := 0; row < maxLines; row++ {
		parts := make([]string, 0, len(columns))
		for col := range columns {
			line := ""
			if row < len(split[col]) {
				line = split[col][row]
			}
			parts = append(parts, padRight(line, widths[col]))
		}
		rows = append(rows, strings.Join(parts, "  "))
	}
	return strings.Join(rows, "\n")
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func truncate(s string, width int) string {
	if width <= 0 || textWidth(s) <= width {
		return s
	}
	if width <= 3 {
		return cellansi.Truncate(s, width, "")
	}
	return cellansi.Truncate(s, width, "...")
}

func renderLineBox(left, right string, width int) string {
	inner := max(10, width-2)
	left = truncate(left, inner)
	right = truncate(right, inner/2)
	space := inner - textWidth(left) - textWidth(right)
	if space < 1 {
		right = ""
		space = inner - textWidth(left)
	}
	border := paint("┌"+strings.Repeat("─", inner)+"┐", activePalette.Border, activePalette.Background)
	content := paint("│", activePalette.Border, activePalette.Background) +
		paint(fitToWidth(left+strings.Repeat(" ", max(0, space))+right, inner), activePalette.Text, activePalette.Background) +
		paint("│", activePalette.Border, activePalette.Background)
	bottom := paint("└"+strings.Repeat("─", inner)+"┘", activePalette.Border, activePalette.Background)
	return border + "\n" + content + "\n" + bottom
}

func renderTripleLine(left, center, right string, width int) string {
	inner := max(10, width-2)
	left = truncate(left, inner/3)
	right = truncate(right, inner/3)
	remaining := inner - textWidth(left) - textWidth(right)
	center = truncate(center, remaining)
	leftPad := max(0, (remaining-textWidth(center))/2)
	rightPad := max(0, remaining-textWidth(center)-leftPad)
	top := paint("┌"+strings.Repeat("─", inner)+"┐", activePalette.Border, activePalette.Background)
	content := paint("│", activePalette.Border, activePalette.Background) +
		paint(fitToWidth(
			paint(left, activePalette.Orange, "")+
				strings.Repeat(" ", leftPad)+
				paint(center, activePalette.TextMuted, "")+
				strings.Repeat(" ", rightPad)+
				paint(right, activePalette.TextMuted, ""),
			inner,
		), activePalette.Text, activePalette.Background) +
		paint("│", activePalette.Border, activePalette.Background)
	bottom := paint("└"+strings.Repeat("─", inner)+"┘", activePalette.Border, activePalette.Background)
	return top + "\n" + content + "\n" + bottom
}

func padRight(s string, width int) string {
	if textWidth(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-textWidth(s))
}

func fitToWidth(s string, width int) string {
	return padRight(truncate(s, width), width)
}

func fillBackground(content string, width int) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = paint(fitToWidth(line, width), "", activePalette.Background)
	}
	return strings.Join(lines, "\n")
}

func fillViewport(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		lines[i] = paint(fitToWidth(line, width), "", activePalette.Background)
	}
	return strings.Join(lines, "\n")
}

func lineCount(content string) int {
	if content == "" {
		return 0
	}
	return len(strings.Split(content, "\n"))
}

func textWidth(s string) int {
	return cellansi.StringWidth(s)
}

func applyTheme(theme colorscheme.Name) {
	activePalette = colorscheme.Lookup(theme).Palette
}

func centerText(text string, width int) string {
	text = truncate(text, max(1, width))
	pad := max(0, (width-textWidth(text))/2)
	return strings.Repeat(" ", pad) + text
}

func verticalCenter(content string, height int) string {
	lines := strings.Split(content, "\n")
	if len(lines) >= height {
		return strings.Join(lines[:height], "\n")
	}
	topPad := max(0, (height-len(lines))/2)
	out := make([]string, 0, height)
	for i := 0; i < topPad; i++ {
		out = append(out, "")
	}
	out = append(out, lines...)
	for len(out) < height {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

func sectionHeading(text, color string) string {
	return paint(" "+text, color, "")
}

func tableHeader(text, color string) string {
	return paint(text, color, "")
}

func separatorLine(width int, color string) string {
	return paint(" "+strings.Repeat("─", max(0, width-1)), color, "")
}

func kvLine(label, value, valueColor string) string {
	return paint(fmt.Sprintf(" %-10s ", label+":"), activePalette.TextMuted, "") + paint(value, valueColor, "")
}

func listRow(text string, selected bool, accent string) string {
	if selected {
		return paint("> ", accent, "") + paint(text, activePalette.TextStrong, "")
	}
	return paint("  ", accent, "") + paint(text, activePalette.Text, "")
}

func trackRow(text string, selected bool, accent string) string {
	if selected {
		return paint("> ", accent, "") + paint(strings.TrimLeft(text, " "), activePalette.TextStrong, "")
	}
	return "  " + paint(strings.TrimLeft(text, " "), activePalette.Text, "")
}

func paint(text, fg, bg string) string {
	if text == "" {
		return ""
	}
	var parts []string
	var resets []string
	if fg != "" {
		parts = append(parts, hexToANSI(fg, 38))
		resets = append(resets, "39")
	}
	if bg != "" {
		parts = append(parts, hexToANSI(bg, 48))
		resets = append(resets, "49")
	}
	if len(parts) == 0 {
		return text
	}
	resetBG := bg
	if resetBG == "" {
		resetBG = activePalette.Background
	}
	reset := append([]string{"39", hexToANSI(resetBG, 48)}, resetsWithoutColorReset(resets)...)
	return "\x1b[" + strings.Join(parts, ";") + "m" + text + "\x1b[" + strings.Join(reset, ";") + "m"
}

func baseBackgroundReset() string {
	return "\x1b[39;" + hexToANSI(activePalette.Background, 48) + "m"
}

func resetsWithoutColorReset(resets []string) []string {
	filtered := make([]string, 0, len(resets))
	for _, reset := range resets {
		if reset == "39" || reset == "49" {
			continue
		}
		filtered = append(filtered, reset)
	}
	return filtered
}

func hexToANSI(hex string, mode int) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return ""
	}
	var r, g, b uint8
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		return ""
	}
	return fmt.Sprintf("%d;2;%d;%d;%d", mode, r, g, b)
}
