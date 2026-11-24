package tui

import (
	"context"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
)

// plainProvider has no InfiniteRadio and no Authenticator.
type plainProvider struct {
	sections []provider.Section
	tracks   []core.Track
}

func (p *plainProvider) ID() provider.ID              { return "plain" }
func (p *plainProvider) Name() string                 { return "Plain" }
func (p *plainProvider) Sections() []provider.Section { return p.sections }
func (p *plainProvider) Bootstrap(context.Context) (provider.Library, error) {
	return provider.Library{}, nil
}
func (p *plainProvider) LoadSection(context.Context, string) ([]core.Track, error) {
	return append([]core.Track(nil), p.tracks...), nil
}
func (p *plainProvider) LoadPlaylist(context.Context, string) ([]core.Track, error) {
	return nil, nil
}
func (p *plainProvider) Search(context.Context, string) (provider.SearchResult, error) {
	return provider.SearchResult{}, nil
}

func TestNavItemsComeFromProviderSections(t *testing.T) {
	prov := &plainProvider{sections: []provider.Section{{ID: "library", Label: "Library", Kind: provider.SectionTracks}}}
	model := NewWithProvider(config.Default(), prov)

	if want := []string{"Library", "Settings"}; !slices.Equal(model.app.NavItems, want) {
		t.Fatalf("nav items = %v, want %v", model.app.NavItems, want)
	}
	model.app.ActivePanel = app.ActivePanelNavigation
	for range 5 {
		model.app.HandleDown()
	}
	if got := derefOrZero(model.app.NavState.Selected()); got != 1 {
		t.Fatalf("expected HandleDown to stop at Settings (1), got %d", got)
	}

	model.app.NavState.Select(intPtr(1))
	model.handleEnter()
	if !model.app.ViewingSettings {
		t.Fatal("expected last nav item to open Settings")
	}
}

func TestFavoritesSectionKindGetsAddedDateSort(t *testing.T) {
	older, newer := int64(100), int64(200)
	prov := &plainProvider{sections: []provider.Section{
		{ID: "library", Label: "Library", Kind: provider.SectionTracks},
		{ID: "liked", Label: "Liked", Kind: provider.SectionFavorites},
	}}
	model := NewWithProvider(config.Default(), prov)
	tracks := []app.Track{
		{ID: "old", Title: "Old", AddedAtMS: &older},
		{ID: "new", Title: "New", AddedAtMS: &newer},
	}

	model.loadCollection(collectionID("liked"), "Liked", tracks)
	if model.app.CurrentTracks[0].ID != "new" {
		t.Fatalf("expected newest favorite first, got %+v", model.app.CurrentTracks)
	}
	if !model.toggleFavoritesSort() || model.app.CurrentTracks[0].ID != "old" {
		t.Fatalf("expected sort toggle on favorites section, got %+v", model.app.CurrentTracks)
	}

	model.loadCollection(collectionID("library"), "Library", tracks)
	if model.app.CurrentTracks[0].ID != "old" || model.toggleFavoritesSort() {
		t.Fatal("expected plain tracks section to keep provider order and refuse sort")
	}
}

func TestLoginKeyWithoutAuthenticatorShowsNotAvailable(t *testing.T) {
	prov := &plainProvider{sections: []provider.Section{{ID: "library", Label: "Library", Kind: provider.SectionTracks}}}
	model := NewWithProvider(config.Default(), prov)
	model.ready = true

	next, _ := model.Update(tea.KeyPressMsg(tea.Key{Text: "O"}))
	updated := next.(Model)

	if updated.login.active {
		t.Fatal("expected login to stay closed without an authenticator")
	}
	if updated.app.StatusMessage != "Login not available for Plain" {
		t.Fatalf("unexpected status %q", updated.app.StatusMessage)
	}
}

func TestRadioSectionWithoutInfiniteRadioDoesNotLoadMore(t *testing.T) {
	prov := &plainProvider{sections: []provider.Section{{ID: "radio", Label: "Radio", Kind: provider.SectionRadio}}}
	model := NewWithProvider(config.Default(), prov)
	model.app.QueueTracks = []app.Track{{ID: "1"}}
	model.app.QueueIndex = intPtr(0)
	model.app.IsFlowQueue = true
	model.app.FlowCollectionID = collectionID("radio")

	if cmd := model.loadMoreFlow(true); cmd != nil {
		t.Fatal("expected no continuation without InfiniteRadio")
	}
	if model.app.FlowLoadingMore {
		t.Fatal("expected loading flag to stay clear")
	}
}
