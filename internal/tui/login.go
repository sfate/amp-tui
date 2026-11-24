package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
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
	m.provider = msg.provider
	m.setCatalog(catalogInfoOf(msg.provider))
	m.runtime = newPlayerRuntime(msg.provider)
	m.login.active = false
	m.login.loading = false
	m.login.input = ""
	m.ready = false
	m.app.StatusMessage = fmt.Sprintf("Login saved. Loading %s library...", m.providerName())
	if err := m.writeConfig(); err != nil {
		m.app.StatusMessage = fmt.Sprintf("Login validated, but save failed: %v", err)
		m.ready = true
		return nil
	}
	return tea.Batch(bootstrapCmd(m.provider), loadingTickCmd(), m.listenMediaControlCmd())
}

func (m *Model) onLoginFailed(msg loginFailedMsg) tea.Cmd {
	m.login.loading = false
	m.app.StatusMessage = msg.message
	return nil
}

func (m *Model) startLogin() {
	if m.catalog.auth == nil {
		m.app.StatusMessage = fmt.Sprintf("Login not available for %s", m.providerName())
		return
	}
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
	m.app.StatusMessage = m.catalog.auth.LoginPrompt().ConfirmText
	m.syncMediaControl()
}

func (m *Model) handleLoginInput(msg tea.KeyPressMsg) tea.Cmd {
	authn := m.catalog.auth
	if m.login.loading {
		switch msg.String() {
		case "ctrl+c", "q":
			return tea.Quit
		}
		return nil
	}
	if authn == nil {
		switch msg.String() {
		case "ctrl+c", "q":
			return tea.Quit
		case "esc":
			m.cancelLogin()
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
			m.app.StatusMessage = authn.LoginPrompt().BrowserWaitText + "..."
			return browserLoginCmd(m.app.Config, authn, m.factory)
		case "p", "P":
			m.app.StatusMessage = authn.LoginPrompt().PasteHint
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
			m.app.StatusMessage = authn.LoginPrompt().PasteHint
			return nil
		}
		secret, err := authn.NormalizeSecret(input)
		if err != nil {
			m.app.StatusMessage = fmt.Sprintf("Login input error: %v", err)
			return nil
		}
		cfg := m.app.Config
		cfg.ARL = secret
		m.login.loading = true
		m.app.StatusMessage = fmt.Sprintf("Validating login %s...", authn.MaskSecret(secret))
		return validateLoginCmd(cfg, m.factory)
	case "o", "O":
		m.login.confirmOpen = true
		m.login.input = ""
		m.app.StatusMessage = authn.LoginPrompt().ConfirmText
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
	if m.provider == nil {
		m.login.active = true
		m.login.confirmOpen = true
		m.app.StatusMessage = fmt.Sprintf("Login is required before loading %s", m.providerName())
	}
}

func (m Model) renderLoginScreen() string {
	lines := []string{
		centerText(paint("amp-tui", activePalette.Purple, ""), m.width),
		"",
		centerText(paint("Sign in with "+m.providerName(), activePalette.TextStrong, ""), m.width),
	}
	lines = append(lines, m.loginBodyLines()...)

	status := strings.TrimSpace(m.app.StatusMessage)
	if status == "" && m.catalog.auth != nil {
		status = m.catalog.auth.LoginPrompt().ConfirmText
	}
	lines = append(lines, "", centerText(paint(status, activePalette.TextMuted, ""), m.width))
	return verticalCenter(strings.Join(lines, "\n"), m.height)
}

func (m Model) loginBodyLines() []string {
	authn := m.catalog.auth
	if authn == nil {
		return []string{centerText(paint(fmt.Sprintf("Login not available for %s.", m.providerName()), activePalette.Text, ""), m.width)}
	}
	prompt := authn.LoginPrompt()
	switch {
	case m.login.loading:
		return []string{
			centerText(paint(prompt.BrowserWaitText+".", activePalette.Aqua, ""), m.width),
			centerText(paint("This window will continue automatically after login succeeds.", activePalette.TextMuted, ""), m.width),
		}
	case m.login.confirmOpen:
		return []string{
			centerText(paint(prompt.ConfirmText+".", activePalette.Text, ""), m.width),
			centerText(paint("Press Esc to return if you already have a working session.", activePalette.TextMuted, ""), m.width),
		}
	}
	input := authn.MaskSecret(m.login.input)
	if input == "" {
		input = "Waiting for login input"
	}
	return []string{
		centerText(paint(prompt.PasteHint+".", activePalette.Text, ""), m.width),
		centerText(paint("Press Ctrl+U to clear, Esc to return.", activePalette.TextMuted, ""), m.width),
		"",
		centerText(paint(input, activePalette.Aqua, ""), m.width),
	}
}
