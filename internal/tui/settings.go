package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/colorscheme"
	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/secrets"
)

type settingsRowID string

const (
	settingProvider    settingsRowID = "provider"
	settingAccount     settingsRowID = "account"
	settingEngine      settingsRowID = "engine"
	settingTheme       settingsRowID = "theme"
	settingVolume      settingsRowID = "volume"
	settingQuality     settingsRowID = "quality"
	settingCrossfade   settingsRowID = "crossfade"
	settingDuration    settingsRowID = "duration"
	settingDisplay     settingsRowID = "display"
	settingCompactView settingsRowID = "compact view"
)

// settingsRows puts the provider rows first, then the global rows, then rows of the active provider.
func (m Model) settingsRows() []settingsRowID {
	rows := []settingsRowID{settingProvider}
	if m.catalog.auth != nil {
		rows = append(rows, settingAccount)
	}
	if !m.caps.ManagedAudio {
		rows = append(rows, settingEngine)
	}
	rows = append(rows, settingTheme, settingVolume)
	if !m.caps.ManagedAudio {
		rows = append(rows, settingCrossfade)
	}
	rows = append(rows, settingDuration, settingDisplay, settingCompactView)
	if m.caps.QualityLadder {
		rows = append(rows, settingQuality)
	}
	return rows
}

func (m *Model) syncSettingsRows() {
	m.app.SettingsItemCount = len(m.settingsRows())
	if derefOrZero(m.app.SettingsState.Selected()) >= m.app.SettingsItemCount {
		m.app.SettingsState.Select(intPtr(max(0, m.app.SettingsItemCount-1)))
	}
}

// enterSelectedSetting toggles the session on the Account row; other rows step forward.
func (m *Model) enterSelectedSetting() tea.Cmd {
	// A switch stops playback, and Settings opens on this row, so Enter must not switch.
	if m.selectedSettingRow() == settingProvider {
		m.app.StatusMessage = "Use ←/→ to change provider"
		return nil
	}
	if m.selectedSettingRow() != settingAccount {
		return m.adjustSelectedSetting(1)
	}
	if m.signedIn() {
		return m.logout()
	}
	m.startLogin()
	return nil
}

func (m Model) signedIn() bool {
	return m.catalog.auth != nil && m.provider != nil
}

func (m Model) selectedSettingRow() settingsRowID {
	rows := m.settingsRows()
	idx := derefOrZero(m.app.SettingsState.Selected())
	if idx < 0 || idx >= len(rows) {
		return ""
	}
	return rows[idx]
}

func (m *Model) adjustSelectedSetting(direction int) tea.Cmd {
	switch m.selectedSettingRow() {
	case settingProvider:
		return m.switchProvider(direction)
	case settingEngine:
		m.app.Config.Engine = nextPlaybackEngine(m.app.Config.Engine, direction)
		if m.session != nil {
			m.session.Stop()
			m.session = nil
		}
		m.shutdownRuntime()
		m.nextPlaybackID++
		m.currentPlayID = 0
		m.app.IsPlaying = false
		m.app.NowPlaying = nil
		m.progress.active = false
		m.visualizer.reset()
		if m.engineFactory != nil {
			m.engine = m.engineFactory(m.app.Config.Engine, m.providerID)
			m.runtime = newPlayerRuntime(m.engine, m.provider)
		}
		m.app.StatusMessage = fmt.Sprintf("Engine: %s (playback stopped)", playbackEngineLabel(m.app.Config.Engine))
		m.syncMediaControl()
		m.persistConfig()
	case settingTheme:
		m.app.Config.Theme = colorscheme.Next(m.app.Config.Theme, direction)
		applyTheme(m.app.Config.Theme)
		m.app.StatusMessage = fmt.Sprintf("Theme: %s", colorscheme.Label(m.app.Config.Theme))
		m.persistConfig()
	case settingVolume:
		m.adjustVolume(direction * 5)
	case settingQuality:
		next := m.stepQuality(m.configuredQuality(), direction, true)
		settings := m.app.Config.ProviderSettings(string(m.providerID))
		settings.DefaultQuality = next.ID
		m.app.Config.SetProviderSettings(string(m.providerID), settings)
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
	return nil
}

func (m *Model) persistConfig() {
	if err := m.writeConfig(); err != nil {
		m.app.StatusMessage = fmt.Sprintf("Save config error: %v", err)
	}
}

// writeConfig refuses to save while the config file on disk failed to load, so the file is not replaced by defaults.
func (m *Model) writeConfig() error {
	if err := writesBlocked(m.configLoadErr); err != nil {
		return err
	}
	if m.saveConfig == nil {
		return nil
	}
	cfg := m.app.Config
	if m.configProvider != "" {
		cfg.Provider = m.configProvider
	}
	return m.saveConfig(cfg)
}

// storeCredential follows the writeConfig rule, so a broken config never ends up half saved.
func (m *Model) storeCredential(secret string) error {
	return saveCredential(m.secrets, m.providerID, m.configLoadErr, secret)
}

func saveCredential(store secrets.Store, id provider.ID, loadErr error, secret string) error {
	if err := writesBlocked(loadErr); err != nil {
		return err
	}
	return store.Set(string(id), secrets.CredentialKey, secret)
}

func writesBlocked(loadErr error) error {
	if loadErr != nil {
		return fmt.Errorf("fix or remove the config file first: %w", loadErr)
	}
	return nil
}

// providerConfig is the factory input for the active provider with the given credential.
func (m Model) providerConfig(credential string) provider.Config {
	return newProviderConfig(m.app.Config, m.providerID, credential, m.secrets, m.configLoadErr)
}

func newProviderConfig(cfg config.Config, id provider.ID, credential string, store secrets.Store, loadErr error) provider.Config {
	return provider.Config{
		Settings:       cfg.ProviderSettings(string(id)),
		Credential:     credential,
		SaveCredential: func(secret string) error { return saveCredential(store, id, loadErr, secret) },
	}
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
	case settingProvider:
		name := m.registry.Name(m.providerID)
		if name == "" {
			name = m.providerName()
		}
		return "Provider", name, activePalette.Purple
	case settingAccount:
		return "Account", ternary(m.signedIn(), "Signed in", "Not signed in"), activePalette.Green
	case settingEngine:
		return "Engine", playbackEngineLabel(m.app.Config.Engine), activePalette.Purple
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

func nextPlaybackEngine(current config.PlaybackEngine, direction int) config.PlaybackEngine {
	choices := []config.PlaybackEngine{config.EngineAuto, config.EngineMPV, config.EngineNative}
	idx := 0
	for i, choice := range choices {
		if choice == config.NormalizeEngine(current) {
			idx = i
			break
		}
	}
	idx = (idx + direction + len(choices)) % len(choices)
	return choices[idx]
}

func playbackEngineLabel(engine config.PlaybackEngine) string {
	switch config.NormalizeEngine(engine) {
	case config.EngineMPV:
		return "mpv"
	case config.EngineNative:
		return "Native"
	default:
		return "Auto"
	}
}
