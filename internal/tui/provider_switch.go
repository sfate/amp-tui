package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/secrets"
)

// switchProvider steps through the registry from the Settings row and saves the choice.
func (m *Model) switchProvider(direction int) tea.Cmd {
	ids := m.registry.IDs()
	if len(ids) < 2 {
		m.app.StatusMessage = "No other provider available"
		return nil
	}
	idx := max(slices.Index(ids, m.providerID), 0)
	next := ids[(idx+direction+len(ids))%len(ids)]
	m.configProvider = ""
	cmd := m.activateProvider(next)
	if err := m.writeConfig(); err != nil {
		m.app.StatusMessage = fmt.Sprintf("Save config error: %v", err)
	}
	return cmd
}

// logout keeps the provider selected and shows its login screen.
func (m *Model) logout() tea.Cmd {
	if err := writesBlocked(m.configLoadErr); err != nil {
		m.app.StatusMessage = fmt.Sprintf("Sign out failed: %v", err)
		return nil
	}
	err := m.secrets.Delete(string(m.providerID), secrets.CredentialKey)
	if err != nil && !errors.Is(err, secrets.ErrNotFound) {
		m.app.StatusMessage = fmt.Sprintf("Sign out failed: %v", err)
		return nil
	}
	name := m.providerName()
	cmd := m.activateProvider(m.providerID)
	m.app.StatusMessage = fmt.Sprintf("Signed out of %s", name)
	return cmd
}

// activateProvider is the one path for startup and switches: reset, build from the factory, then bootstrap or log in.
func (m *Model) activateProvider(id provider.ID) tea.Cmd {
	m.resetProviderState()
	m.providerID = id
	m.app.Config.Provider = string(id)
	if m.engineFactory != nil {
		selected := m.engineFactory(m.app.Config.Engine, id)
		if m.engine == nil || m.engine.Name() != selected.Name() {
			m.engine = selected
		}
	}
	m.factory, _ = m.registry.Lookup(id)
	m.setCatalog(catalogInfo{name: m.registry.Name(id)})
	m.setProvider(nil)
	if m.factory == nil {
		m.ready = true
		m.app.StatusMessage = "No music provider configured"
		return nil
	}

	credential, credErr := m.secrets.Get(string(id), secrets.CredentialKey)
	if errors.Is(credErr, secrets.ErrNotFound) {
		credErr = nil
	}
	instance, err := m.factory(m.providerConfig(credential))
	if err != nil && credential != "" {
		// A rejected credential still needs a signed-out instance for the login steps.
		instance, _ = m.factory(m.providerConfig(""))
	}
	if instance != nil {
		m.setCatalog(catalogInfoOf(instance))
	}
	anonymousAccess := false
	if anonymous, ok := instance.(provider.AnonymousProvider); ok {
		anonymousAccess = anonymous.SupportsAnonymousAccess()
	}

	switch {
	case err == nil && (m.catalog.auth == nil || strings.TrimSpace(credential) != "" || anonymousAccess):
		m.setProvider(instance)
		m.runtime = newPlayerRuntime(m.engine, instance)
		m.ready = false
		m.app.StatusMessage = m.loadingStatus()
		return tea.Batch(bootstrapCmd(instance), loadingTickCmd(), m.listenMediaControlCmd())
	case m.catalog.auth == nil:
		m.ready = true
		m.app.StatusMessage = fmt.Sprintf("Could not start %s: %v", m.providerName(), err)
		return nil
	}

	m.ready = true
	m.openLogin()
	switch {
	case err != nil:
		m.app.StatusMessage = fmt.Sprintf("Login required: %v", err)
	case credErr != nil:
		m.app.StatusMessage = fmt.Sprintf("Login required: could not read the saved login: %v", credErr)
	}
	return nil
}

// resetProviderState drops playback, queue and caches, because track IDs belong to one provider.
func (m *Model) resetProviderState() {
	if m.session != nil {
		m.session.Stop()
		m.session = nil
	}
	m.shutdownRuntime()
	m.runtime = nil
	m.pendingPlayback = pendingPlayback{requestID: m.pendingPlayback.requestID + 1}
	m.nextPlaybackID++
	m.currentPlayID = 0
	m.currentTrackID = ""
	m.currentQuality = core.Quality{}
	m.playbackRetries = 0
	m.pauseRequested = false
	m.progress = playbackProgress{}
	m.prebuffer = prebufferState{statuses: map[string]PrebufferStatus{}, ready: []string{}}
	m.artwork = artworkState{cache: map[string]string{}, cacheOrder: []string{}}
	m.visualizer.reset()
	m.search.activeID = 0
	m.bootstrapFailed = false
	m.login = loginState{}

	a := m.app
	a.NowPlaying = nil
	a.IsPlaying = false
	a.AutoTransitionArmed = false
	a.QueueTracks = []app.Track{}
	a.QueueIndex = nil
	a.QueueState.Select(intPtr(0))
	a.Playlists = []app.Playlist{}
	a.PlaylistState.Select(intPtr(0))
	a.CurrentTracks = []app.Track{}
	a.CurrentPlaylistID = nil
	a.MainState.Select(intPtr(0))
	a.SearchPlaylists = []app.Playlist{}
	a.SearchArtists = []app.Artist{}
	a.ShowingSearchResult = false
	a.IsSearching = false
	a.SearchLoading = false
	a.SearchQuery = ""
	a.FlowNextIndex = 0
	a.FlowLoadingMore = false
	a.IsFlowQueue = false
	a.FlowSeenTrackIDs = map[string]struct{}{}
	a.FlowSeenTrackIDOrder = []string{}
	a.ViewingSettings = false
	a.ActivePanel = app.ActivePanelNavigation
}
