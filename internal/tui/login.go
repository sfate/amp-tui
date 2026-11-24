package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/auth"
)

type loginState struct {
	active      bool
	input       string
	loading     bool
	confirmOpen bool
}

func (m *Model) onLoginSucceeded(msg loginSucceededMsg) tea.Cmd {
	m.bootstrapFailed = false
	m.app.Config = msg.cfg
	m.loader = msg.loader
	m.runtime = newPlayerRuntime(msg.loader)
	m.login.active = false
	m.login.loading = false
	m.login.input = ""
	m.ready = false
	m.app.StatusMessage = "Login saved. Loading Deezer library..."
	if err := m.writeConfig(); err != nil {
		m.app.StatusMessage = fmt.Sprintf("Login validated, but save failed: %v", err)
		m.ready = true
		return nil
	}
	return tea.Batch(bootstrapCmd(m.loader), loadingTickCmd(), m.listenMediaControlCmd())
}

func (m *Model) onLoginFailed(msg loginFailedMsg) tea.Cmd {
	m.login.loading = false
	m.app.StatusMessage = msg.message
	return nil
}

func (m *Model) startLogin() {
	if m.session != nil {
		m.session.Stop()
		m.session = nil
	}
	m.login.active = true
	m.login.loading = false
	m.login.input = ""
	m.login.confirmOpen = true
	m.app.IsPlaying = false
	m.app.IsSearching = false
	m.app.SearchLoading = false
	m.app.StatusMessage = "Press Enter to open Deezer login in your browser"
	m.syncMediaControl()
}

func (m *Model) handleLoginInput(msg tea.KeyPressMsg) tea.Cmd {
	if m.login.loading {
		switch msg.String() {
		case "ctrl+c", "q":
			return tea.Quit
		}
		return nil
	}

	if m.login.confirmOpen {
		switch msg.String() {
		case "ctrl+c", "q":
			return tea.Quit
		case "esc":
			m.cancelLogin()
			return nil
		case "enter":
			m.login.confirmOpen = false
			m.login.loading = true
			m.app.StatusMessage = "Complete Deezer login in the browser..."
			return browserLoginCmd(m.app.Config, m.loaderFactory)
		case "p", "P":
			m.app.StatusMessage = "Paste ARL or Cookie header, then press Enter"
			return nil
		}
		return nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return tea.Quit
	case "esc":
		m.cancelLogin()
		return nil
	case "ctrl+u":
		m.login.input = ""
		m.app.StatusMessage = "Login input cleared"
		return nil
	case "backspace":
		m.login.input = trimLastRune(m.login.input)
		return nil
	case "enter":
		input := strings.TrimSpace(m.login.input)
		if input == "" {
			m.app.StatusMessage = "Paste ARL or Cookie header, then press Enter"
			return nil
		}
		arl, err := auth.NormalizeARL(input)
		if err != nil {
			m.app.StatusMessage = fmt.Sprintf("Login input error: %v", err)
			return nil
		}
		cfg := m.app.Config
		cfg.ARL = arl
		m.login.loading = true
		m.app.StatusMessage = fmt.Sprintf("Validating login %s...", auth.MaskARL(arl))
		return validateLoginCmd(cfg, m.loaderFactory)
	case "o", "O":
		m.login.confirmOpen = true
		m.login.input = ""
		m.app.StatusMessage = "Press Enter to open Deezer login in your browser"
		return nil
	}

	if len(msg.Text) > 0 {
		m.login.input += msg.Text
	}
	return nil
}

func (m *Model) cancelLogin() {
	m.login.active = false
	m.login.loading = false
	m.login.input = ""
	m.login.confirmOpen = false
	m.app.StatusMessage = "Library"
	if m.bootstrapFailed {
		m.app.StatusMessage = "Library could not load. Press Enter to retry or O to sign in"
	}
	if m.loader == nil {
		m.login.active = true
		m.login.confirmOpen = true
		m.app.StatusMessage = "Login is required before loading Deezer"
	}
}

func (m Model) renderLoginScreen() string {
	status := strings.TrimSpace(m.app.StatusMessage)
	if status == "" {
		status = "Press Enter to open Deezer login in your browser"
	}

	lines := []string{
		centerText(paint("amp-tui", activePalette.Purple, ""), m.width),
		"",
		centerText(paint("Sign in with Deezer", activePalette.TextStrong, ""), m.width),
	}

	switch {
	case m.login.loading:
		lines = append(lines,
			centerText(paint("Complete Deezer login in the browser.", activePalette.Aqua, ""), m.width),
			centerText(paint("This window will continue automatically after login succeeds.", activePalette.TextMuted, ""), m.width),
		)
	case m.login.confirmOpen:
		lines = append(lines,
			centerText(paint("Press Enter to open Deezer login in your browser.", activePalette.Text, ""), m.width),
			centerText(paint("Press Esc to return if you already have a working session.", activePalette.TextMuted, ""), m.width),
		)
	default:
		input := auth.MaskARL(m.login.input)
		if strings.Contains(m.login.input, "arl=") {
			input = "Cookie header pasted"
		}
		if input == "" {
			input = "Waiting for login input"
		}
		lines = append(lines,
			centerText(paint("Paste ARL or Cookie header, then press Enter.", activePalette.Text, ""), m.width),
			centerText(paint("Press Ctrl+U to clear, Esc to return.", activePalette.TextMuted, ""), m.width),
			"",
			centerText(paint(input, activePalette.Aqua, ""), m.width),
		)
	}
	lines = append(lines, "", centerText(paint(status, activePalette.TextMuted, ""), m.width))
	return verticalCenter(strings.Join(lines, "\n"), m.height)
}
