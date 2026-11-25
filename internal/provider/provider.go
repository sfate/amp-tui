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

// ManagedAudioProvider plays tracks through a service-owned player instead of yielding audio bytes.
type ManagedAudioProvider interface {
	Provider
	ManagedAudio() bool
}

// InfiniteRadio extends a SectionRadio list; offset is the count of tracks already loaded.
type InfiniteRadio interface {
	LoadRadioPage(ctx context.Context, sectionID string, offset int) ([]core.Track, error)
}

type LoginStepKind int

const (
	// LoginStepInput asks the user to type or paste a value.
	LoginStepInput LoginStepKind = iota
	// LoginStepBrowser starts on Enter and waits until Submit returns.
	LoginStepBrowser
)

// LoginStep is one sign-in method; the login screen shows one step at a time and Tab moves to the next.
type LoginStep struct {
	ID   string
	Kind LoginStepKind
	// Prompt and WaitText are user-visible, without trailing punctuation.
	Prompt   string
	WaitText string
	Masked   bool
}

// Authenticator works on a signed-out provider, so the login screen can use it.
type Authenticator interface {
	LoginSteps() []LoginStep
	// Submit turns the input of a step into the credential; input is empty for a browser step.
	Submit(ctx context.Context, stepID, input string) (string, error)
}

// AnonymousProvider may be used without the optional Authenticator credential.
type AnonymousProvider interface {
	SupportsAnonymousAccess() bool
}

// Config is the input of a Factory; the TUI fills it from the config file and the secrets store.
type Config struct {
	Settings config.ProviderSettings
	// Credential is the saved login secret; empty means signed out.
	Credential string
	// SaveCredential stores a replaced secret, such as a refreshed OAuth token.
	SaveCredential func(string) error
}

// ResolvedSourceInvalidator drops a provider's cached URL after playback fails.
type ResolvedSourceInvalidator interface {
	InvalidateResolvedSource(trackID string)
}

// Factory returns a signed-out Provider when cfg has no Credential.
type Factory func(cfg Config) (Provider, error)
