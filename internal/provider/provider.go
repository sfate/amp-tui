// Package provider defines the contract between the TUI and a music service.
package provider

import (
	"context"
	"errors"

	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/core"
)

// ErrUnauthenticated means the provider rejected or lacks the saved login.
var ErrUnauthenticated = errors.New("session is not authenticated; please sign in again")

type ID string

type SectionKind int

const (
	// SectionTracks is a plain track list.
	SectionTracks SectionKind = iota
	// SectionFavorites is a track list the TUI can sort by added date.
	SectionFavorites
	// SectionRadio is an endless list that InfiniteRadio extends page by page.
	SectionRadio
)

type Section struct {
	ID    string
	Label string
	Kind  SectionKind
}

type SearchResult struct {
	Tracks    []core.Track
	Playlists []core.Playlist
	Artists   []core.Artist
}

type Library struct {
	Playlists []core.Playlist
}

// Provider returns sanitized metadata, so the TUI can render it as is.
type Provider interface {
	ID() ID
	Name() string
	Sections() []Section
	Bootstrap(ctx context.Context) (Library, error)
	LoadSection(ctx context.Context, sectionID string) ([]core.Track, error)
	LoadPlaylist(ctx context.Context, id string) ([]core.Track, error)
	Search(ctx context.Context, query string) (SearchResult, error)
}

// InfiniteRadio extends a SectionRadio list; offset is the count of tracks already loaded.
type InfiniteRadio interface {
	LoadRadioPage(ctx context.Context, sectionID string, offset int) ([]core.Track, error)
}

// LoginPrompt holds the user-visible login texts, without trailing punctuation.
type LoginPrompt struct {
	ConfirmText     string
	BrowserWaitText string
	PasteHint       string
	SecretName      string
}

// Authenticator works on a signed-out provider, so the login screen can use it.
type Authenticator interface {
	LoginPrompt() LoginPrompt
	NormalizeSecret(input string) (string, error)
	MaskSecret(secret string) string
	BrowserLogin(ctx context.Context) (string, error)
}

// Config is the input of a Factory; the TUI fills it from the config file and the secrets store.
type Config struct {
	Settings config.ProviderSettings
	// Credential is the saved login secret; empty means signed out.
	Credential string
	// SaveCredential stores a replaced secret, such as a refreshed OAuth token.
	SaveCredential func(string) error
}

// Factory returns a signed-out Provider when cfg has no Credential.
type Factory func(cfg Config) (Provider, error)
