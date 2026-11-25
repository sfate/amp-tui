// Package applemusic controls the macOS Music app and reads its local library.
package applemusic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/sanitize"
)

const ID provider.ID = "applemusic"

const (
	librarySection = "library"
	qualityID      = "music-app"
	maxArtworkSize = 2 << 20
)

var sections = []provider.Section{{ID: librarySection, Label: "Library", Kind: provider.SectionTracks}}

type scriptRunner interface {
	Run(ctx context.Context, operation string, args ...string) ([]byte, error)
}

type Provider struct {
	runner scriptRunner
}

var (
	_ provider.Provider             = (*Provider)(nil)
	_ provider.StreamSource         = (*Provider)(nil)
	_ provider.ManagedAudioProvider = (*Provider)(nil)
)

func New(_ provider.Config) *Provider { return newProvider(provider.Config{}, execScriptRunner{}) }

func newProvider(_ provider.Config, runner scriptRunner) *Provider { return &Provider{runner: runner} }

func (p *Provider) ID() provider.ID { return ID }

func (p *Provider) Name() string { return "Apple Music" }

func (p *Provider) Sections() []provider.Section { return append([]provider.Section(nil), sections...) }

func (p *Provider) ManagedAudio() bool { return true }

func (p *Provider) Bootstrap(ctx context.Context) (provider.Library, error) {
	output, err := p.runner.Run(ctx, "playlists")
	if err != nil {
		return provider.Library{}, err
	}
	var records []playlistRecord
	if err := json.Unmarshal(output, &records); err != nil {
		return provider.Library{}, fmt.Errorf("parse Music.app playlists: %w", err)
	}
	playlists := make([]core.Playlist, 0, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.Title) == "" {
			continue
		}
		playlists = append(playlists, core.Playlist{ID: record.ID, Title: clean(record.Title)})
	}
	return provider.Library{Playlists: playlists}, nil
}

func (p *Provider) LoadSection(ctx context.Context, sectionID string) ([]core.Track, error) {
	if sectionID != librarySection {
		return nil, fmt.Errorf("unknown Apple Music section %q", sectionID)
	}
	output, err := p.runner.Run(ctx, "library_tracks")
	if err != nil {
		return nil, err
	}
	return parseTracks(output)
}

func (p *Provider) LoadPlaylist(ctx context.Context, id string) ([]core.Track, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("apple music playlist ID is empty")
	}
	output, err := p.runner.Run(ctx, "playlist_tracks", id)
	if err != nil {
		return nil, err
	}
	return parseTracks(output)
}

func (p *Provider) Search(ctx context.Context, query string) (provider.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return provider.SearchResult{}, nil
	}
	output, err := p.runner.Run(ctx, "search_tracks", query)
	if err != nil {
		return provider.SearchResult{}, err
	}
	tracks, err := parseTracks(output)
	if err != nil {
		return provider.SearchResult{}, err
	}
	return provider.SearchResult{Tracks: tracks}, nil
}

func (p *Provider) Qualities() []core.Quality {
	return []core.Quality{{ID: qualityID, Label: "Music.app", Seekable: true}}
}

func (p *Provider) DefaultQuality() core.Quality { return p.Qualities()[0] }

func (p *Provider) Resolve(ctx context.Context, trackID string, quality core.Quality) (provider.Source, error) {
	if quality.ID != qualityID {
		return provider.Source{}, fmt.Errorf("unknown Apple Music playback mode %q", quality.ID)
	}
	if strings.TrimSpace(trackID) == "" {
		return provider.Source{}, errors.New("apple music track ID is empty")
	}
	output, err := p.runner.Run(ctx, "track", trackID)
	if err != nil {
		return provider.Source{}, err
	}
	var record trackRecord
	if err := json.Unmarshal(output, &record); err != nil {
		return provider.Source{}, fmt.Errorf("parse Music.app track: %w", err)
	}
	if record.ID == "" || record.Title == "" {
		return provider.Source{}, errors.New("music.app returned incomplete track metadata")
	}
	info := trackInfo(record)
	if art, err := p.runner.Run(ctx, "artwork", trackID); err == nil && len(art) > 0 && len(art) <= maxArtworkSize {
		mime := http.DetectContentType(art)
		encoded := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(art)
		info.AlbumArtURL = &encoded
	}
	return provider.Source{Info: info, Quality: p.DefaultQuality(), Seekable: true}, nil
}

func parseTracks(output []byte) ([]core.Track, error) {
	var records []trackRecord
	if err := json.Unmarshal(output, &records); err != nil {
		return nil, fmt.Errorf("parse Music.app tracks: %w", err)
	}
	tracks := make([]core.Track, 0, len(records))
	for _, record := range records {
		if record.ID == "" || record.Title == "" {
			continue
		}
		tracks = append(tracks, core.Track{
			ID: record.ID, Title: clean(record.Title), Artist: clean(record.Artist), Album: clean(record.Album),
		})
	}
	return tracks, nil
}

func trackInfo(record trackRecord) core.TrackInfo {
	info := core.TrackInfo{
		ID: record.ID, Title: clean(record.Title), Artist: clean(record.Artist), Album: clean(record.Album),
	}
	if record.Duration > 0 {
		info.DurationMS = uint64(record.Duration * 1000)
	}
	return info
}

func clean(value string) string { return sanitize.Text(value) }

type playlistRecord struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type trackRecord struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Duration float64 `json:"duration"`
}

type playbackState struct {
	State    string      `json:"state"`
	Track    trackRecord `json:"track"`
	Position float64     `json:"position"`
}
