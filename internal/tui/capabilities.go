package tui

import (
	"fmt"
	"strings"

	"github.com/sfate/amp-tui/internal/provider"
)

// setProvider keeps the capabilities and the Settings rows in step with the provider.
func (m *Model) setProvider(p provider.Provider) {
	m.provider = p
	m.caps = provider.CapabilitiesOf(p)
	m.syncSettingsRows()
}

func (m *Model) setNotAvailable() {
	m.app.StatusMessage = fmt.Sprintf("Not available for %s", m.providerName())
}

// gateKey answers "Not available" for a key whose function the provider lacks.
func (m *Model) gateKey(key string) bool {
	var supported bool
	switch key {
	case ",", ".":
		supported = m.caps.Playback
	case "u", "i":
		supported = m.caps.QualityLadder
	case "s":
		supported = m.caps.Favorites
	default:
		return false
	}
	if !supported {
		m.setNotAvailable()
	}
	return !supported
}

func (m Model) controlsHint() string {
	parts := []string{"space play/pause", "n/p next prev"}
	if m.caps.Playback {
		parts = append(parts, ",/. seek")
	}
	if m.caps.QualityLadder {
		parts = append(parts, "u/i quality")
	}
	parts = append(parts, "r repeat")
	if m.caps.Favorites && m.viewingFavorites() {
		parts = append(parts, "s sort")
	}
	parts = append(parts, "+/- volume")
	return " " + strings.Join(parts, " | ") + " "
}
