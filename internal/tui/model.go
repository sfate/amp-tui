package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/colorscheme"
	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
)

// Model is the Bubble Tea root model; each concern keeps its state and handlers in its own file.
type Model struct {
	app             *app.App
	provider        provider.Provider
	factory         provider.Factory
	catalog         catalogInfo
	saveConfig      func(config.Config) error
	configLoadErr   error
	bootstrapFailed bool
	ready           bool
	loadingFrame    int
	width           int
	height          int

	favoritesSortAsc bool

	runtime         PlayerRuntime
	session         PlaybackSession
	playbackEvents  chan tea.Msg
	nextPlaybackID  int
	currentPlayID   int
	currentTrackID  string
	currentQuality  core.Quality
	playbackRetries int
	pauseRequested  bool
	pendingPlayback pendingPlayback
	progress        playbackProgress

	login      loginState
	search     searchState
	prebuffer  prebufferState
	artwork    artworkState
	visualizer visualizerState
	perf       perfState
}

func New(opts ...Option) Model {
	cfg, err := config.Load()
	model := NewWithConfig(cfg, opts...)
	if err != nil {
		model.configLoadErr = err
		model.app.StatusMessage = fmt.Sprintf("Config error, settings will not be saved: %v", err)
	}
	return model
}

// NewWithConfig signs in to the default registry provider when cfg holds a secret.
func NewWithConfig(cfg config.Config, opts ...Option) Model {
	cfg = normalizeConfig(cfg)
	o := applyOptions(opts)
	id := o.registry.Default()
	factory, _ := o.registry.Lookup(id)
	info := catalogInfo{name: o.registry.Name(id)}

	var session provider.Provider
	var factoryErr error
	if factory != nil {
		instance, err := factory(cfg)
		if err != nil {
			factoryErr = err
		} else {
			info = catalogInfoOf(instance)
			if strings.TrimSpace(cfg.ARL) != "" {
				session = instance
			}
		}
	}

	model := newModel(app.New(cfg), session, factory)
	model.setCatalog(info)
	loginActive := session == nil
	switch {
	case factory == nil:
		model.app.StatusMessage = "No music provider configured"
	case factoryErr != nil:
		model.app.StatusMessage = fmt.Sprintf("Login required: %v", factoryErr)
	case loginActive && info.auth != nil:
		model.app.StatusMessage = "Login required: paste " + info.auth.LoginPrompt().SecretName
	case loginActive:
		model.app.StatusMessage = fmt.Sprintf("Login not available for %s", model.providerName())
	default:
		model.app.StatusMessage = model.loadingStatus()
	}
	model.login.active = loginActive
	model.login.confirmOpen = loginActive
	return model
}

func NewWithProvider(cfg config.Config, p provider.Provider) Model {
	model := newModel(app.New(normalizeConfig(cfg)), p, nil)
	model.setCatalog(catalogInfoOf(p))
	model.app.StatusMessage = model.loadingStatus()
	if p == nil {
		model.app.StatusMessage = "No music provider configured"
	}
	return model
}

func NewWithProviderAndRuntime(cfg config.Config, p provider.Provider, runtime PlayerRuntime) Model {
	model := NewWithProvider(cfg, p)
	model.runtime = runtime
	return model
}

// normalizeConfig also applies the theme, because rendering reads the package palette.
func normalizeConfig(cfg config.Config) config.Config {
	cfg.Theme = colorscheme.Normalize(cfg.Theme)
	cfg.DisplayMode = config.NormalizeDisplayMode(cfg.DisplayMode, cfg.DisplayEnabled)
	cfg.DisplayEnabled = cfg.DisplayMode != config.DisplayModeOff
	applyTheme(cfg.Theme)
	return cfg
}

func newModel(state *app.App, p provider.Provider, factory provider.Factory) Model {
	return Model{
		app:            state,
		provider:       p,
		runtime:        newPlayerRuntime(p),
		playbackEvents: make(chan tea.Msg, 32),
		factory:        factory,
		saveConfig:     config.Save,
		artwork:        artworkState{cache: map[string]string{}, cacheOrder: []string{}},
		prebuffer:      prebufferState{statuses: map[string]PrebufferStatus{}, ready: []string{}},
		perf:           perfState{logPath: perfLogPathFromEnv(), msgCounts: map[string]int{}},
		ready:          p == nil,
	}
}

func (m Model) Init() tea.Cmd {
	mediaCmd := m.listenMediaControlCmd()
	if m.login.active || m.provider == nil {
		return mediaCmd
	}
	return tea.Batch(bootstrapCmd(m.provider), loadingTickCmd(), mediaCmd)
}

// Update routes each message to the handler of its concern.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.recordPerfMsg(msg)
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case loginSucceededMsg:
		cmd = m.onLoginSucceeded(msg)
	case loginFailedMsg:
		cmd = m.onLoginFailed(msg)
	case bootstrapFailedMsg:
		cmd = m.onBootstrapFailed(msg)
	case bootstrapLoadedMsg:
		cmd = m.onBootstrapLoaded(msg)
	case collectionLoadedMsg:
		cmd = m.onCollectionLoaded(msg)
	case loadFailedMsg:
		cmd = m.onLoadFailed(msg)
	case loadingTickMsg:
		cmd = m.onLoadingTick()
	case prebufferStatusMsg:
		cmd = m.onPrebufferStatus(msg)
	case prebufferTickMsg:
		cmd = m.onPrebufferTick()
	case mediaControlCommandMsg:
		cmd = tea.Batch(m.handleMediaControlCommand(msg.command), m.listenMediaControlCmd())
	case searchLoadedMsg:
		cmd = m.onSearchLoaded(msg)
	case searchFailedMsg:
		cmd = m.onSearchFailed(msg)
	case playbackStartedMsg:
		cmd = m.onPlaybackStarted(msg)
	case playbackStartFailedMsg:
		cmd = m.onPlaybackStartFailed(msg)
	case bufferingProgressMsg:
		cmd = m.onBufferingProgress(msg)
	case playbackTrackChangedMsg:
		cmd = m.onPlaybackTrackChanged(msg)
	case playbackProgressMsg:
		cmd = m.onPlaybackProgress(msg)
	case playbackErrorMsg:
		cmd = m.onPlaybackError(msg)
	case playbackFinishedMsg:
		cmd = m.onPlaybackFinished(msg)
	case artworkLoadedMsg:
		cmd = m.onArtworkLoaded(msg)
	case playbackTickMsg:
		cmd = m.onPlaybackTick()
	case playbackVisualizerMsg:
		cmd = m.onPlaybackVisualizer(msg)
	case scheduledPlaybackMsg:
		cmd = m.onScheduledPlayback(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		cmd = m.onKeyPress(msg)
	}
	return m, cmd
}

func (m Model) View() tea.View {
	renderStarted := time.Now()
	defer func() {
		m.recordRenderDuration(time.Since(renderStarted))
	}()

	if m.width == 0 || m.height == 0 {
		view := tea.NewView("Loading amp-tui...")
		view.AltScreen = true
		view.WindowTitle = "amp-tui"
		return view
	}

	if m.login.active {
		view := tea.NewView(fillBackground(m.renderLoginScreen(), m.width))
		view.AltScreen = true
		view.WindowTitle = "amp-tui login"
		return view
	}

	if !m.ready {
		view := tea.NewView(fillBackground(m.renderLoadingScreen(), m.width))
		view.AltScreen = true
		view.WindowTitle = "amp-tui"
		return view
	}

	header := m.renderHeader()
	searchBar := m.renderSearchBar()
	footer := m.renderPlaybar()
	sidebarWidth := min(28, max(22, m.width/5))
	middleAvailable := max(72, m.width-sidebarWidth-4)
	contentHeight := max(10, m.height-20)
	if m.shouldUseCompactView(contentHeight) {
		contentHeight = max(1, m.height-lineCount(header)-lineCount(searchBar)-lineCount(footer)-3)
		queueWidth := middleAvailable / 2
		statusWidth := middleAvailable - queueWidth
		renderBody := func(height int) string {
			return joinColumns(
				m.renderSidebar(sidebarWidth, height),
				m.renderQueue(queueWidth, height),
				m.renderCompactStatusColumn(statusWidth, height),
			)
		}
		body := renderBody(contentHeight)
		content := strings.Join([]string{
			header,
			searchBar,
			body,
			footer,
		}, "\n")
		for gap := m.height - lineCount(content); gap > 0; gap = m.height - lineCount(content) {
			contentHeight += gap
			body = renderBody(contentHeight)
			content = strings.Join([]string{
				header,
				searchBar,
				body,
				footer,
			}, "\n")
		}
		content = fillViewport(content, m.width, m.height)
		view := tea.NewView(content)
		view.AltScreen = true
		view.WindowTitle = "amp-tui"
		return view
	}
	queueWidth := middleAvailable / 2
	mainWidth := middleAvailable - queueWidth
	body := joinColumns(
		m.renderSidebar(sidebarWidth, contentHeight),
		m.renderQueue(queueWidth, contentHeight),
		m.renderMain(mainWidth, contentHeight),
	)
	status := m.renderStatusArea()

	content := strings.Join([]string{
		header,
		searchBar,
		body,
		status,
		footer,
	}, "\n")
	content = fillViewport(content, m.width, m.height)
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "amp-tui"
	return view
}

func (m Model) shouldUseCompactView(normalQueueHeight int) bool {
	if !m.app.Config.CompactView || len(m.app.QueueTracks) == 0 {
		return false
	}
	return normalQueueVisibleTrackRows(normalQueueHeight) < 3
}

func normalQueueVisibleTrackRows(height int) int {
	const queueRowsBeforeTracks = 7
	const panelBorders = 2
	return max(0, height-panelBorders-queueRowsBeforeTracks)
}
