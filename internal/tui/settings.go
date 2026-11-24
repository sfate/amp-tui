package tui

import (
	"fmt"

	"github.com/sfate/amp-tui/internal/colorscheme"
	"github.com/sfate/amp-tui/internal/config"
)

func (m *Model) adjustSelectedSetting(direction int) {
	switch derefOrZero(m.app.SettingsState.Selected()) {
	case 0:
		m.app.Config.Theme = colorscheme.Next(m.app.Config.Theme, direction)
		applyTheme(m.app.Config.Theme)
		m.app.StatusMessage = fmt.Sprintf("Theme: %s", colorscheme.Label(m.app.Config.Theme))
		m.persistConfig()
	case 1:
		m.adjustVolume(direction * 5)
	case 2:
		m.app.Config.DefaultQuality = nextQuality(m.app.Config.DefaultQuality, direction)
		m.app.StatusMessage = fmt.Sprintf("Quality: %s", qualityLabel(m.app.Config.DefaultQuality))
		m.persistConfig()
	case 3:
		m.app.Config.CrossfadeEnabled = !m.app.Config.CrossfadeEnabled
		m.app.StatusMessage = fmt.Sprintf("Crossfade: %s", onOff(m.app.Config.CrossfadeEnabled))
		m.persistConfig()
	case 4:
		m.app.Config.CrossfadeDurationMS = nextCrossfadeDuration(m.app.Config.CrossfadeDurationMS, direction)
		m.app.StatusMessage = fmt.Sprintf("Crossfade duration: %dms", m.app.Config.CrossfadeDurationMS)
		m.persistConfig()
	case 5:
		m.app.Config.DisplayMode = nextDisplayMode(m.app.Config.DisplayMode, direction)
		m.app.Config.DisplayEnabled = m.app.Config.DisplayMode != config.DisplayModeOff
		m.app.StatusMessage = fmt.Sprintf("Display: %s", displayModeLabel(m.app.Config.DisplayMode))
		m.persistConfig()
	case 6:
		m.app.Config.CompactView = !m.app.Config.CompactView
		m.app.StatusMessage = fmt.Sprintf("Compact view: %s", onOff(m.app.Config.CompactView))
		m.persistConfig()
	}
}

func (m *Model) persistConfig() {
	if err := m.writeConfig(); err != nil {
		m.app.StatusMessage = fmt.Sprintf("Save config error: %v", err)
	}
}

// writeConfig refuses to save while the config file on disk failed to load, so the file is not replaced by defaults.
func (m *Model) writeConfig() error {
	if m.configLoadErr != nil {
		return fmt.Errorf("fix or remove the config file first: %w", m.configLoadErr)
	}
	if m.saveConfig == nil {
		return nil
	}
	return m.saveConfig(m.app.Config)
}

func qualityLabel(q config.AudioQuality) string {
	switch q {
	case config.AudioQuality128:
		return "128 kbps"
	case config.AudioQualityFlac:
		return "FLAC"
	default:
		return "320 kbps"
	}
}

func nextQuality(current config.AudioQuality, direction int) config.AudioQuality {
	qualities := []config.AudioQuality{config.AudioQuality128, config.AudioQuality320, config.AudioQualityFlac}
	idx := 1
	for i, quality := range qualities {
		if quality == current {
			idx = i
			break
		}
	}
	if direction < 0 {
		idx = (idx - 1 + len(qualities)) % len(qualities)
	} else {
		idx = (idx + 1) % len(qualities)
	}
	return qualities[idx]
}

func nextCrossfadeDuration(current uint64, direction int) uint64 {
	presets := []uint64{0, 1000, 3000, 5000, 8000, 10000, 13000}
	idx := 0
	for i, value := range presets {
		if value == current {
			idx = i
			break
		}
		if value < current {
			idx = i
		}
	}
	if direction < 0 {
		idx = (idx - 1 + len(presets)) % len(presets)
	} else {
		idx = (idx + 1) % len(presets)
	}
	return presets[idx]
}

func nextDisplayMode(current config.DisplayMode, direction int) config.DisplayMode {
	modes := []config.DisplayMode{
		config.DisplayModeOff,
		config.DisplayModeEqualizer,
		config.DisplayModeSpectrum,
		config.DisplayModeWaveform,
		config.DisplayModeScope,
	}
	current = config.NormalizeDisplayMode(current, true)
	idx := 1
	for i, mode := range modes {
		if mode == current {
			idx = i
			break
		}
	}
	if direction < 0 {
		idx = (idx - 1 + len(modes)) % len(modes)
	} else {
		idx = (idx + 1) % len(modes)
	}
	return modes[idx]
}

func displayModeLabel(mode config.DisplayMode) string {
	switch config.NormalizeDisplayMode(mode, true) {
	case config.DisplayModeOff:
		return "Off"
	case config.DisplayModeSpectrum:
		return "Spectrum"
	case config.DisplayModeWaveform:
		return "Waveform"
	case config.DisplayModeScope:
		return "Oscilloscope"
	default:
		return "Equalizer"
	}
}

func onOff(v bool) string {
	if v {
		return "On"
	}
	return "Off"
}

func (m Model) renderSettingsRows() []string {
	selected := derefOrZero(m.app.SettingsState.Selected())
	rows := []struct {
		label string
		value string
		color string
	}{
		{label: "Theme", value: colorscheme.Label(m.app.Config.Theme), color: activePalette.Blue},
		{label: "Volume", value: fmt.Sprintf("%d%%", m.app.Volume), color: activePalette.Aqua},
		{label: "Quality", value: qualityLabel(m.app.Config.DefaultQuality), color: activePalette.Purple},
		{label: "Crossfade", value: onOff(m.app.Config.CrossfadeEnabled), color: activePalette.Orange},
		{label: "Duration", value: fmt.Sprintf("%dms", m.app.Config.CrossfadeDurationMS), color: activePalette.Yellow},
		{label: "Display", value: displayModeLabel(m.app.Config.DisplayMode), color: activePalette.Aqua},
		{label: "Compact View", value: onOff(m.app.Config.CompactView), color: activePalette.Green},
	}
	lines := make([]string, 0, len(rows))
	for i, row := range rows {
		line := kvLine(row.label, row.value, row.color)
		if i == selected {
			line = trackRow(line, true, row.color)
		}
		lines = append(lines, line)
	}
	return lines
}
