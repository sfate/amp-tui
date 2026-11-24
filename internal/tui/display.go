package tui

import (
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/config"
)

const visualizerUIUpdateMinInterval = 33 * time.Millisecond

type visualizerState struct {
	bands []uint8
	peaks []float64
}

func (v *visualizerState) reset() {
	v.bands = nil
	v.peaks = nil
}

func (m *Model) onPlaybackVisualizer(msg playbackVisualizerMsg) tea.Cmd {
	if msg.playID != m.currentPlayID || !m.app.IsPlaying || len(msg.bands) == 0 {
		return listenPlaybackEventCmd(m.playbackEvents)
	}
	m.visualizer.bands = append(m.visualizer.bands[:0], msg.bands...)
	m.updateVisualizerPeaks(msg.bands)
	return listenPlaybackEventCmd(m.playbackEvents)
}

func (m *Model) updateVisualizerPeaks(bands []uint8) {
	if len(bands) == 0 {
		return
	}
	if len(m.visualizer.peaks) != len(bands) {
		m.visualizer.peaks = make([]float64, len(bands))
	}
	const peakFall = 0.035
	for i, band := range bands {
		level := float64(band) / 8
		if level >= m.visualizer.peaks[i] {
			m.visualizer.peaks[i] = level
			continue
		}
		m.visualizer.peaks[i] = math.Max(level, m.visualizer.peaks[i]-peakFall)
	}
}

func (m Model) renderDisplayPanel(width int) string {
	return m.renderDisplayPanelWithHeight(width, 11)
}

func (m Model) renderDisplayPanelWithHeight(width, height int) string {
	body := m.renderDisplayBody(max(1, width-2), max(1, height-2))
	return m.renderPanel("Display", body, displayMode(m.app.Config) != config.DisplayModeOff && len(m.visualizer.bands) > 0, width, height)
}

func (m Model) renderDisplayBody(width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	lines := make([]string, height)
	mode := displayMode(m.app.Config)
	if mode == config.DisplayModeOff || len(m.visualizer.bands) == 0 {
		for i := range lines {
			lines[i] = strings.Repeat(" ", width)
		}
		return strings.Join(lines, "\n")
	}

	switch mode {
	case config.DisplayModeSpectrum:
		return renderSpectrumDisplay(width, height, m.visualizer.bands)
	case config.DisplayModeWaveform:
		return renderWaveformDisplay(width, height, m.visualizer.bands)
	case config.DisplayModeScope:
		return renderScopeDisplay(width, height, m.visualizer.bands)
	default:
		return renderEqualizerDisplay(width, height, m.visualizer.bands, m.visualizer.peaks)
	}
}

func renderSpectrumDisplay(width, height int, bands []uint8) string {
	return renderEqualizerDisplay(width, height, bands, nil)
}

// The current event API provides frequency-band levels rather than PCM samples.
// These two modes render that live envelope until a backend exposes sample data.
func renderWaveformDisplay(width, height int, bands []uint8) string {
	grid := blankDisplayGrid(width, height)
	center := height / 2
	for x := 0; x < width; x++ {
		level := interpolatedBandLevel(bands, float64(x)*float64(max(0, len(bands)-1))/float64(max(1, width-1)))
		amplitude := int(math.Round(level * float64(max(1, height/2))))
		for y := max(0, center-amplitude); y <= min(height-1, center+amplitude); y++ {
			grid[y][x] = displayCell{glyph: "█", color: equalizerColor(level, y, height)}
		}
	}
	return displayGridString(grid)
}

func renderScopeDisplay(width, height int, bands []uint8) string {
	grid := blankDisplayGrid(width, height)
	center := height / 2
	for x := 0; x < width; x++ {
		phase := float64(x) / float64(max(1, width-1))
		level := interpolatedBandLevel(bands, phase*float64(max(0, len(bands)-1)))
		y := center - int(math.Round(math.Sin(phase*2*math.Pi)*level*float64(max(1, height/2))))
		if y >= 0 && y < height {
			grid[y][x] = displayCell{glyph: "●", color: equalizerColor(level, y, height)}
		}
	}
	return displayGridString(grid)
}

func renderEqualizerDisplay(width, height int, bands []uint8, peaks []float64) string {
	grid := blankDisplayGrid(width, height)
	if len(bands) == 0 {
		return displayGridString(grid)
	}
	bars := max(len(bands)*3, min(width, len(bands)*5))
	gap := 1
	barWidth := max(1, (width-(bars-1)*gap)/bars)
	usedWidth := bars*barWidth + (bars-1)*gap
	if usedWidth > width {
		gap = 0
		barWidth = max(1, width/bars)
		usedWidth = bars * barWidth
	}
	leftPad := max(0, (width-usedWidth)/2)
	for bar := 0; bar < bars; bar++ {
		position := float64(bar) * float64(len(bands)-1) / math.Max(1, float64(bars-1))
		level := interpolatedBandLevel(bands, position)
		fill := int(math.Round(level * float64(height)))
		peakLevel := interpolatedPeakLevel(peaks, position)
		peakRow := height - 1 - int(math.Round(peakLevel*float64(height-1)))
		x0 := leftPad + bar*(barWidth+gap)
		for y := height - 1; y >= max(0, height-fill); y-- {
			for x := x0; x < min(width, x0+barWidth); x++ {
				grid[y][x] = displayCell{
					glyph: equalizerGlyph(level, y, height),
					color: equalizerColor(level, y, height),
				}
			}
		}
		if peakLevel > 0 && peakRow >= 0 && peakRow < height {
			for x := x0; x < min(width, x0+barWidth); x++ {
				grid[peakRow][x] = displayCell{glyph: "▀", color: activePalette.TextStrong}
			}
		}
	}
	return displayGridString(grid)
}

func equalizerGlyph(level float64, row, height int) string {
	switch {
	case level > 0.72 && row < height/3:
		return "█"
	case level > 0.42:
		return "▓"
	default:
		return "▒"
	}
}

func equalizerColor(level float64, row, height int) string {
	switch {
	case level > 0.78 && row < height/3:
		return activePalette.Orange
	case level > 0.55:
		return activePalette.Yellow
	case level > 0.32:
		return activePalette.Aqua
	default:
		return activePalette.Green
	}
}

func interpolatedBandLevel(bands []uint8, position float64) float64 {
	if len(bands) == 0 {
		return 0
	}
	if len(bands) == 1 {
		return float64(bands[0]) / 8
	}
	if position < 0 {
		position = 0
	}
	maxPosition := float64(len(bands) - 1)
	if position > maxPosition {
		position = maxPosition
	}
	left := int(math.Floor(position))
	right := min(len(bands)-1, left+1)
	weight := position - float64(left)
	value := float64(bands[left])*(1-weight) + float64(bands[right])*weight
	return value / 8
}

func interpolatedPeakLevel(peaks []float64, position float64) float64 {
	if len(peaks) == 0 {
		return 0
	}
	if len(peaks) == 1 {
		return clampFloat64(peaks[0], 0, 1)
	}
	if position < 0 {
		position = 0
	}
	maxPosition := float64(len(peaks) - 1)
	if position > maxPosition {
		position = maxPosition
	}
	left := int(math.Floor(position))
	right := min(len(peaks)-1, left+1)
	weight := position - float64(left)
	value := peaks[left]*(1-weight) + peaks[right]*weight
	return clampFloat64(value, 0, 1)
}

func clampFloat64(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

type displayCell struct {
	glyph string
	color string
}

func blankDisplayGrid(width, height int) [][]displayCell {
	grid := make([][]displayCell, height)
	for row := range grid {
		grid[row] = make([]displayCell, width)
	}
	return grid
}

func displayGridString(grid [][]displayCell) string {
	out := make([]string, len(grid))
	for row := range grid {
		var line strings.Builder
		line.Grow(len(grid[row]))
		for _, cell := range grid[row] {
			if cell.glyph == "" {
				line.WriteByte(' ')
				continue
			}
			line.WriteString(paint(cell.glyph, cell.color, ""))
		}
		out[row] = line.String()
	}
	return strings.Join(out, "\n")
}

func displayMode(cfg config.Config) config.DisplayMode {
	return config.NormalizeDisplayMode(cfg.DisplayMode, cfg.DisplayEnabled)
}
