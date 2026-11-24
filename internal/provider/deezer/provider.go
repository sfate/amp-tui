package deezer

import (
	"context"
	"fmt"
	"strings"

	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
)

const ID provider.ID = "deezer"

const (
	sectionHome      = "home"
	sectionFlow      = "flow"
	sectionExplore   = "explore"
	sectionFavorites = "favorites"
)

var sections = []provider.Section{
	{ID: sectionHome, Label: "Home", Kind: provider.SectionTracks},
	{ID: sectionFlow, Label: "Flow", Kind: provider.SectionRadio},
	{ID: sectionExplore, Label: "Explore", Kind: provider.SectionTracks},
	{ID: sectionFavorites, Label: "Favorites", Kind: provider.SectionFavorites},
}

// Provider is the Deezer catalog; client is nil while signed out.
type Provider struct {
	client *Client
}

var (
	_ provider.Provider      = (*Provider)(nil)
	_ provider.InfiniteRadio = (*Provider)(nil)
	_ provider.Authenticator = (*Provider)(nil)
)

func Register(reg *provider.Registry) {
	reg.Register(ID, "Deezer", func(cfg config.Config) (provider.Provider, error) {
		return New(cfg)
	})
}

// New returns a signed-out provider when cfg has no ARL.
func New(cfg config.Config) (*Provider, error) {
	if strings.TrimSpace(cfg.ARL) == "" {
		return &Provider{}, nil
	}
	client, err := NewClient(cfg.ARL, Options{})
	if err != nil {
		return nil, err
	}
	return &Provider{client: client}, nil
}

func (p *Provider) ID() provider.ID { return ID }

func (p *Provider) Name() string { return "Deezer" }

func (p *Provider) Sections() []provider.Section {
	return append([]provider.Section(nil), sections...)
}

func (p *Provider) Bootstrap(ctx context.Context) (provider.Library, error) {
	if p.client == nil {
		return provider.Library{}, ErrUnauthenticated
	}
	if _, err := p.client.FetchAPIToken(ctx); err != nil {
		return provider.Library{}, fmt.Errorf("fetch Deezer session: %w", err)
	}
	playlists, err := p.client.FetchUserPlaylists(ctx, p.client.UserID())
	if err != nil {
		return provider.Library{}, fmt.Errorf("fetch playlists: %w", err)
	}
	return provider.Library{Playlists: mapPlaylists(playlists)}, nil
}

func (p *Provider) LoadSection(ctx context.Context, sectionID string) ([]core.Track, error) {
	if p.client == nil {
		return nil, ErrUnauthenticated
	}
	var tracks []Track
	var err error
	switch sectionID {
	case sectionHome:
		tracks, err = p.client.FetchHomeTracks(ctx)
	case sectionFlow:
		tracks, err = p.client.FetchFlowTracks(ctx, 0)
	case sectionExplore:
		tracks, err = p.client.FetchExploreTracks(ctx)
	case sectionFavorites:
		tracks, err = p.client.FetchFavoriteTracks(ctx)
	default:
		return nil, fmt.Errorf("unknown Deezer section %q", sectionID)
	}
	return mapTracks(tracks), err
}

func (p *Provider) LoadRadioPage(ctx context.Context, sectionID string, offset int) ([]core.Track, error) {
	if p.client == nil {
		return nil, ErrUnauthenticated
	}
	if sectionID != sectionFlow {
		return nil, fmt.Errorf("deezer section %q is not a radio", sectionID)
	}
	tracks, err := p.client.FetchFlowTracks(ctx, offset)
	return mapTracks(tracks), err
}

func (p *Provider) LoadPlaylist(ctx context.Context, id string) ([]core.Track, error) {
	if p.client == nil {
		return nil, ErrUnauthenticated
	}
	tracks, err := p.client.FetchPlaylistTracks(ctx, id)
	return mapTracks(tracks), err
}

func (p *Provider) Search(ctx context.Context, query string) (provider.SearchResult, error) {
	if p.client == nil {
		return provider.SearchResult{}, ErrUnauthenticated
	}
	results, err := p.client.FetchSearchResults(ctx, query)
	if err != nil {
		return provider.SearchResult{}, err
	}
	return provider.SearchResult{
		Tracks:    mapTracks(results.Tracks),
		Playlists: mapPlaylists(results.Playlists),
		Artists:   mapArtists(results.Artists),
	}, nil
}
