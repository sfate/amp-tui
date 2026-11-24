package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/provider"
)

func bootstrapCmd(p provider.Provider) tea.Cmd {
	if p == nil {
		return nil
	}
	return func() tea.Msg {
		library, err := p.Bootstrap(context.Background())
		if err != nil {
			return bootstrapFailedMsg{err: err}
		}
		return bootstrapLoadedMsg{playlists: library.Playlists}
	}
}

func validateLoginCmd(cfg provider.Config, factory provider.Factory) tea.Cmd {
	return func() tea.Msg {
		return signIn(cfg, factory)
	}
}

func browserLoginCmd(cfg provider.Config, authn provider.Authenticator, factory provider.Factory) tea.Cmd {
	return func() tea.Msg {
		secret, err := authn.BrowserLogin(context.Background())
		if err != nil {
			return loginFailedMsg{message: fmt.Sprintf("Browser login failed: %v", err)}
		}
		cfg.Credential = secret
		return signIn(cfg, factory)
	}
}

// signIn builds a provider from cfg and proves the credential with a bootstrap call.
func signIn(cfg provider.Config, factory provider.Factory) tea.Msg {
	if factory == nil {
		return loginFailedMsg{message: "Login error: no music provider configured"}
	}
	p, err := factory(cfg)
	if err != nil {
		return loginFailedMsg{message: fmt.Sprintf("Login error: %v", err)}
	}
	if _, err := p.Bootstrap(context.Background()); err != nil {
		return loginFailedMsg{message: fmt.Sprintf("Login validation failed: %v", err)}
	}
	return loginSucceededMsg{secret: cfg.Credential, provider: p}
}

func listenMediaControlCmd(events <-chan MediaControlCommand) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		command, ok := <-events
		if !ok {
			return nil
		}
		return mediaControlCommandMsg{command: command}
	}
}

func listenPlaybackEventCmd(events <-chan tea.Msg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		return <-events
	}
}

func playbackTickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return playbackTickMsg{}
	})
}

func loadingTickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return loadingTickMsg{}
	})
}

func prebufferTickCmd() tea.Cmd {
	return tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg {
		return prebufferTickMsg{}
	})
}

func loadSectionCmd(p provider.Provider, sec provider.Section, autoplay bool) tea.Cmd {
	if p == nil {
		return nil
	}
	return func() tea.Msg {
		tracks, err := p.LoadSection(context.Background(), sec.ID)
		if err != nil {
			return loadFailedMsg{message: fmt.Sprintf("%s error: %v", sec.Label, err)}
		}
		return collectionLoadedMsg{
			id:       collectionID(sec.ID),
			title:    sec.Label,
			tracks:   tracks,
			isFlow:   sec.Kind == provider.SectionRadio,
			autoplay: autoplay,
		}
	}
}

func loadRadioPageCmd(radio provider.InfiniteRadio, sec provider.Section, offset int, autoplay bool) tea.Cmd {
	return func() tea.Msg {
		tracks, err := radio.LoadRadioPage(context.Background(), sec.ID, offset)
		if err != nil {
			return loadFailedMsg{message: fmt.Sprintf("%s error: %v", sec.Label, err)}
		}
		return collectionLoadedMsg{id: collectionID(sec.ID), title: sec.Label, tracks: tracks, isFlow: true, append: true, autoplay: autoplay}
	}
}

func loadPlaylistCmd(p provider.Provider, id string, title string) tea.Cmd {
	if p == nil {
		return nil
	}
	return func() tea.Msg {
		tracks, err := p.LoadPlaylist(context.Background(), id)
		if err != nil {
			return loadFailedMsg{message: fmt.Sprintf("Playlist error: %v", err)}
		}
		return collectionLoadedMsg{id: id, title: title, tracks: tracks}
	}
}

func searchCmd(p provider.Provider, requestID int, query string) tea.Cmd {
	if p == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
		defer cancel()
		results, err := p.Search(ctx, query)
		if err != nil {
			return searchFailedMsg{requestID: requestID, message: fmt.Sprintf("Search error: %v", err)}
		}
		return searchLoadedMsg{
			requestID: requestID,
			query:     query,
			tracks:    results.Tracks,
			playlists: results.Playlists,
			artists:   results.Artists,
		}
	}
}
