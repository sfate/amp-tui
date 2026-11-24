package tui

import (
	"fmt"

	"github.com/sfate/amp-tui/internal/colorscheme"
	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/core"
)

type settingsRowID string

const (
	settingTheme       settingsRowID = "theme"
	settingVolume      settingsRowID = "volume"
	settingQuality     settingsRowID = "quality"
	settingCrossfade   settingsRowID = "crossfade"
	settingDuration    settingsRowID = "duration"
	settingDisplay     settingsRowID = "display"
	settingCompactView settingsRowID = "compact view"
)

func (m Model) settingsRows() []settingsRowID {
	rows := []settingsRowID{settingTheme, settingVolume}
	if m.caps.QualityLadder {
		rows = append(rows, settingQuality)
	}
	return append(rows, settingCrossfade, settingDuration, settingDisplay, settingCompactView)
}

func (m Model) selectedSettingRow() settingsRowID {
	rows := m.settingsRows()
	idx := derefOrZero(m.app.SettingsState.Selected())
	if idx < 0 || idx >= len(rows) {
		return ""
	}
	return rows[idx]
}

func (m *Model) adjustSelectedSetting(direction int) {
	switch m.selectedSettingRow() {
	case settingTheme:
		m.app.Config.Theme = colorscheme.Next(m.app.Config.Theme, direction)
		applyTheme(m.app.Config.Theme)
		m.app.StatusMessage = fmt.Sprintf("Theme: %s", colorscheme.Label(m.app.Config.Theme))
		m.persistConfig()
	case settingVolume:
		m.adjustVolume(direction * 5)
	case settingQuality:
		next := m.stepQuality(m.configuredQuality(), direction, true)
		m.app.Config.DefaultQuality = next.ID
		m.app.StatusMessage = fmt.Sprintf("Quality: %s", qualityLabel(next))
		m.persistConfig()
	case settingCrossfade:
		m.app.Config.CrossfadeEnabled = !m.app.Config.CrossfadeEnabled
		m.app.StatusMessage = fmt.Sprintf("Crossfade: %s", onOff(m.app.Config.CrossfadeEnabled))
		m.persistConfig()
	case settingDuration:
		m.app.Config.CrossfadeDurationMS = nextCrossfadeDuration(m.app.Config.CrossfadeDurationMS, direction)
		m.app.StatusMessage = fmt.Sprintf("Crossfade duration: %dms", m.app.Config.CrossfadeDurationMS)
		m.persistConfig()
	case settingDisplay:
		m.app.Config.DisplayMode = nextDisplayMode(m.app.Config.DisplayMode, direction)
		m.app.Config.DisplayEnabled = m.app.Config.DisplayMode != config.DisplayModeOff
		m.app.StatusMessage = fmt.Sprintf("Display: %s", displayModeLabel(m.app.Config.DisplayMode))
		m.persistConfig()
	case settingCompactView:
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

func qualityLabel(q core.Quality) string {
	if q.Label == "" {
		return "-"
	}
	return q.Label
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
	rows := m.settingsRows()
	lines := make([]string, 0, len(rows))
	for i, id := range rows {
		label, value, color := m.settingsRowView(id)
		line := kvLine(label, value, color)
		if i == selected {
			line = trackRow(line, true, color)
		}
		lines = append(lines, line)
	}
	return lines
}

func (m Model) settingsRowView(id settingsRowID) (label, value, color string) {
	switch id {
	case settingTheme:
		return "Theme", colorscheme.Label(m.app.Config.Theme), activePalette.Blue
	case settingVolume:
		return "Volume", fmt.Sprintf("%d%%", m.app.Volume), activePalette.Aqua
	case settingQuality:
		return "Quality", qualityLabel(m.configuredQuality()), activePalette.Purple
	case settingCrossfade:
		return "Crossfade", onOff(m.app.Config.CrossfadeEnabled), activePalette.Orange
	case settingDuration:
		return "Duration", fmt.Sprintf("%dms", m.app.Config.CrossfadeDurationMS), activePalette.Yellow
	case settingDisplay:
		return "Display", displayModeLabel(m.app.Config.DisplayMode), activePalette.Aqua
	default:
		return "Compact View", onOff(m.app.Config.CompactView), activePalette.Green
	}
}
