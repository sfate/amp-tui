package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/provider"
)

const maskedInputLimit = 32

// loginState renders the provider's login steps; step indexes Authenticator.LoginSteps.
type loginState struct {
	active  bool
	step    int
	input   string
	loading bool
}

func (m Model) loginSteps() []provider.LoginStep {
	if m.catalog.auth == nil {
		return nil
	}
	return m.catalog.auth.LoginSteps()
}

func (m Model) currentLoginStep() (provider.LoginStep, bool) {
	steps := m.loginSteps()
	if m.login.step < 0 || m.login.step >= len(steps) {
		return provider.LoginStep{}, false
	}
	return steps[m.login.step], true
}

// openLogin shows the first step; callers check that the provider has an Authenticator.
func (m *Model) openLogin() {
	m.login = loginState{active: true}
	if step, ok := m.currentLoginStep(); ok {
		m.app.StatusMessage = step.Prompt
	}
}

func (m *Model) moveLoginStep(direction int) {
	steps := m.loginSteps()
	if len(steps) < 2 {
		return
	}
	m.login.step = (m.login.step + direction + len(steps)) % len(steps)
	m.login.input = ""
	m.app.StatusMessage = steps[m.login.step].Prompt
}

func (m *Model) onLoginSucceeded(msg loginSucceededMsg) tea.Cmd {
	m.bootstrapFailed = false
	m.app.Config.Provider = string(m.providerID)
	m.setProvider(msg.provider)
	m.setCatalog(catalogInfoOf(msg.provider))
	m.runtime = newPlayerRuntime(msg.provider)
	m.login = loginState{}
	m.ready = false
	m.app.StatusMessage = fmt.Sprintf("Login saved. Loading %s library...", m.providerName())
	err := m.storeCredential(msg.secret)
	if err == nil {
		err = m.writeConfig()
	}
	if err != nil {
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
	m.openLogin()
	m.app.IsPlaying = false
	m.app.IsSearching = false
	m.app.SearchLoading = false
	m.syncMediaControl()
}

func (m *Model) handleLoginInput(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	step, ok := m.currentLoginStep()
	typing := ok && step.Kind == provider.LoginStepInput && !m.login.loading
	if key == "ctrl+c" || (key == "q" && !typing) {
		return tea.Quit
	}
	if m.login.loading {
		return nil
	}
	switch key {
	case "esc":
		m.cancelLogin()
		return nil
	case "tab":
		m.moveLoginStep(1)
		return nil
	case "shift+tab":
		m.moveLoginStep(-1)
		return nil
	}
	if !ok {
		return nil
	}
	if step.Kind == provider.LoginStepBrowser {
		if key == "enter" {
			return m.submitLoginStep(step, "")
		}
		return nil
	}

	switch key {
	case "ctrl+u":
		m.login.input = ""
		m.app.StatusMessage = "Login input cleared"
	case "backspace":
		m.login.input = trimLastRune(m.login.input)
	case "enter":
		input := strings.TrimSpace(m.login.input)
		if input == "" {
			m.app.StatusMessage = step.Prompt
			return nil
		}
		return m.submitLoginStep(step, input)
	default:
		m.login.input += msg.Text
	}
	return nil
}

func (m *Model) submitLoginStep(step provider.LoginStep, input string) tea.Cmd {
	m.login.loading = true
	m.app.StatusMessage = loginWaitText(step) + "..."
	return submitLoginCmd(m.providerConfig(""), m.catalog.auth, step.ID, input, m.factory)
}

func loginWaitText(step provider.LoginStep) string {
	if step.WaitText == "" {
		return "Checking login"
	}
	return step.WaitText
}

func (m *Model) cancelLogin() {
	m.login = loginState{}
	m.app.StatusMessage = "Library"
	if m.bootstrapFailed {
		m.app.StatusMessage = "Library could not load. Press Enter to retry or O to sign in"
	}
	if m.provider == nil {
		m.login.active = true
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
	if step, ok := m.currentLoginStep(); status == "" && ok {
		status = step.Prompt
	}
	lines = append(lines, "", centerText(paint(status, activePalette.TextMuted, ""), m.width))
	return verticalCenter(strings.Join(lines, "\n"), m.height)
}

func (m Model) loginBodyLines() []string {
	line := func(text, color string) string { return centerText(paint(text, color, ""), m.width) }
	step, ok := m.currentLoginStep()
	if !ok {
		return []string{line(fmt.Sprintf("Login not available for %s.", m.providerName()), activePalette.Text)}
	}
	var lines []string
	switch {
	case m.login.loading:
		return []string{
			line(loginWaitText(step)+".", activePalette.Aqua),
			line("This window will continue automatically after login succeeds.", activePalette.TextMuted),
		}
	case step.Kind == provider.LoginStepBrowser:
		lines = []string{
			line(step.Prompt+".", activePalette.Text),
			line("Press Esc to return if you already have a working session.", activePalette.TextMuted),
		}
	default:
		lines = []string{
			line(step.Prompt+".", activePalette.Text),
			line("Press Ctrl+U to clear, Esc to return.", activePalette.TextMuted),
			"",
			line(loginInputView(m.login.input, step.Masked), activePalette.Aqua),
		}
	}
	if len(m.loginSteps()) > 1 {
		lines = append(lines, "", line("Press Tab for another sign-in method.", activePalette.TextMuted))
	}
	return lines
}

// loginInputView caps the mask length, so a long pasted secret does not wrap the screen.
func loginInputView(input string, masked bool) string {
	switch {
	case input == "":
		return "Waiting for login input"
	case masked:
		return strings.Repeat("•", min(utf8.RuneCountInString(input), maskedInputLimit))
	default:
		return input
	}
}
