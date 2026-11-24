package tui

import (
	"context"
	"fmt"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/auth"
	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/deezer"
	"github.com/sfate/amp-tui/internal/player"
)

func bootstrapCmd(loader Loader) tea.Cmd {
	if loader == nil {
		return nil
	}
	return func() tea.Msg {
		data, err := loader.Bootstrap(context.Background())
		if err != nil {
			return bootstrapFailedMsg{err: err}
		}
		return bootstrapLoadedMsg{playlists: data.Playlists}
	}
}

func validateLoginCmd(cfg config.Config, loaderFactory func(config.Config) (Loader, error)) tea.Cmd {
	if loaderFactory == nil {
		loaderFactory = defaultLoaderFactory
	}
	return func() tea.Msg {
		loader, err := loaderFactory(cfg)
		if err != nil {
			return loginFailedMsg{message: fmt.Sprintf("Login error: %v", err)}
		}
		if _, err := loader.Bootstrap(context.Background()); err != nil {
			return loginFailedMsg{message: fmt.Sprintf("Login validation failed: %v", err)}
		}
		return loginSucceededMsg{cfg: cfg, loader: loader}
	}
}

func browserLoginCmd(cfg config.Config, loaderFactory func(config.Config) (Loader, error)) tea.Cmd {
	return func() tea.Msg {
		arl, err := auth.BrowserLogin(context.Background(), auth.BrowserLoginOptions{})
		if err != nil {
			return loginFailedMsg{message: fmt.Sprintf("Browser login failed: %v", err)}
		}
		cfg.ARL = arl
		if loaderFactory == nil {
			loaderFactory = defaultLoaderFactory
		}
		loader, err := loaderFactory(cfg)
		if err != nil {
			return loginFailedMsg{message: fmt.Sprintf("Login error: %v", err)}
		}
		if _, err := loader.Bootstrap(context.Background()); err != nil {
			return loginFailedMsg{message: fmt.Sprintf("Login validation failed: %v", err)}
		}
		return loginSucceededMsg{cfg: cfg, loader: loader}
	}
}

func startPlaybackCmdWithEvents(playID int, trackID string, runtime PlayerRuntime, quality deezer.AudioQuality, seekMS uint64, enableVisualizer bool, events chan tea.Msg) tea.Cmd {
	if runtime == nil {
		return nil
	}
	return func() tea.Msg {
		var visualizerMu sync.Mutex
		var lastVisualizerSent time.Time
		handler := player.EventHandler{
			OnTrackChanged: func(meta deezer.TrackMetadata, q deezer.AudioQuality, initialMS uint64) {
				events <- playbackTrackChangedMsg{playID: playID, meta: meta, quality: q, initialMS: initialMS}
			},
			OnBufferingProgress: func(percent uint8, stage player.BufferingStage) {
				events <- bufferingProgressMsg{playID: playID, percent: percent, stage: stage}
			},
			OnPlaybackProgress: func(currentMS, totalMS uint64) {
				events <- playbackProgressMsg{playID: playID, currentMS: currentMS, totalMS: totalMS}
			},
			OnError: func(err error) {
				events <- playbackErrorMsg{playID: playID, err: err}
			},
		}
		if enableVisualizer {
			handler.OnAudioBands = func(bands []uint8) {
				visualizerMu.Lock()
				if !lastVisualizerSent.IsZero() && time.Since(lastVisualizerSent) < visualizerUIUpdateMinInterval {
					visualizerMu.Unlock()
					return
				}
				lastVisualizerSent = time.Now()
				visualizerMu.Unlock()

				copied := append([]uint8(nil), bands...)
				select {
				case events <- playbackVisualizerMsg{playID: playID, bands: copied}:
				default:
				}
			}
		}
		session, err := runtime.Start(trackID, quality, seekMS, handler)
		if err != nil {
			return playbackStartFailedMsg{playID: playID, trackID: trackID, quality: quality, err: err}
		}
		return playbackStartedMsg{playID: playID, session: session}
	}
}

func waitPlaybackCmd(playID int, session PlaybackSession) tea.Cmd {
	if session == nil {
		return nil
	}
	return func() tea.Msg {
		return playbackFinishedMsg{playID: playID, err: session.Wait()}
	}
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

func loadHomeCmd(loader Loader) tea.Cmd {
	if loader == nil {
		return nil
	}
	return func() tea.Msg {
		tracks, err := loader.LoadHome(context.Background())
		if err != nil {
			return loadFailedMsg{message: fmt.Sprintf("Home error: %v", err)}
		}
		return collectionLoadedMsg{id: "__home__", title: "Home", tracks: tracks}
	}
}

func loadFlowCmd(loader Loader, index int, append bool, autoplay bool) tea.Cmd {
	if loader == nil {
		return nil
	}
	return func() tea.Msg {
		tracks, err := loader.LoadFlow(context.Background(), index)
		if err != nil {
			return loadFailedMsg{message: fmt.Sprintf("Flow error: %v", err)}
		}
		return collectionLoadedMsg{id: "__flow__", title: "Flow", tracks: tracks, isFlow: true, append: append, autoplay: autoplay}
	}
}

func loadExploreCmd(loader Loader) tea.Cmd {
	if loader == nil {
		return nil
	}
	return func() tea.Msg {
		tracks, err := loader.LoadExplore(context.Background())
		if err != nil {
			return loadFailedMsg{message: fmt.Sprintf("Explore error: %v", err)}
		}
		return collectionLoadedMsg{id: "__explore__", title: "Explore", tracks: tracks}
	}
}

func loadFavoritesCmd(loader Loader) tea.Cmd {
	if loader == nil {
		return nil
	}
	return func() tea.Msg {
		tracks, err := loader.LoadFavorites(context.Background())
		if err != nil {
			return loadFailedMsg{message: fmt.Sprintf("Favorites error: %v", err)}
		}
		return collectionLoadedMsg{id: "__favorites__", title: "Favorites", tracks: tracks}
	}
}

func loadPlaylistCmd(loader Loader, id string, title string) tea.Cmd {
	if loader == nil {
		return nil
	}
	return func() tea.Msg {
		tracks, err := loader.LoadPlaylist(context.Background(), id)
		if err != nil {
			return loadFailedMsg{message: fmt.Sprintf("Playlist error: %v", err)}
		}
		return collectionLoadedMsg{id: id, title: title, tracks: tracks}
	}
}

func searchCmd(loader Loader, requestID int, query string) tea.Cmd {
	if loader == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
		defer cancel()
		results, err := loader.Search(ctx, query)
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
