package deezer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sfate/amp-tui/internal/provider"
)

func TestProviderSectionsAndCapabilities(t *testing.T) {
	p, err := New(provider.Config{})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	want := []provider.Section{
		{ID: "home", Label: "Home", Kind: provider.SectionTracks},
		{ID: "flow", Label: "Flow", Kind: provider.SectionRadio},
		{ID: "explore", Label: "Explore", Kind: provider.SectionTracks},
		{ID: "favorites", Label: "Favorites", Kind: provider.SectionFavorites},
	}
	got := p.Sections()
	if len(got) != len(want) {
		t.Fatalf("sections = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("section %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	var asProvider provider.Provider = p
	if _, ok := asProvider.(provider.InfiniteRadio); !ok {
		t.Fatal("expected Deezer provider to implement InfiniteRadio")
	}
	if _, ok := asProvider.(provider.Authenticator); !ok {
		t.Fatal("expected Deezer provider to implement Authenticator")
	}
	if p.ID() != ID || p.Name() != "Deezer" {
		t.Fatalf("unexpected identity %q %q", p.ID(), p.Name())
	}
}

func TestSignedOutProviderRejectsCatalogCalls(t *testing.T) {
	p, err := New(provider.Config{})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, err := p.Bootstrap(context.Background()); !errors.Is(err, provider.ErrUnauthenticated) {
		t.Fatalf("expected ErrUnauthenticated, got %v", err)
	}
	if _, err := p.LoadSection(context.Background(), "home"); !errors.Is(err, provider.ErrUnauthenticated) {
		t.Fatalf("expected ErrUnauthenticated, got %v", err)
	}
}

func TestLoadSectionRejectsUnknownSection(t *testing.T) {
	p, err := New(provider.Config{Credential: "token"})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, err := p.LoadSection(context.Background(), "missing"); err == nil {
		t.Fatal("expected unknown section error")
	}
}

func TestErrUnauthenticatedMatchesProviderError(t *testing.T) {
	if !errors.Is(ErrUnauthenticated, provider.ErrUnauthenticated) {
		t.Fatal("expected Deezer ErrUnauthenticated to match provider.ErrUnauthenticated")
	}
}

func TestRegisterAddsDeezerFactory(t *testing.T) {
	reg := provider.NewRegistry()
	Register(reg)
	if reg.Default() != ID {
		t.Fatalf("expected Deezer as default provider, got %q", reg.Default())
	}
	if _, ok := reg.Lookup(ID); !ok {
		t.Fatal("expected Deezer factory to be registered")
	}
}

func TestLoginStepsAreBrowserThenMaskedARL(t *testing.T) {
	p, _ := New(provider.Config{})
	steps := p.LoginSteps()
	if len(steps) != 2 || steps[0].Kind != provider.LoginStepBrowser || steps[1].Kind != provider.LoginStepInput || !steps[1].Masked {
		t.Fatalf("unexpected steps %+v", steps)
	}
	if steps[0].Prompt != "Press Enter to open Deezer login in your browser" || !strings.Contains(steps[1].Prompt, "ARL") {
		t.Fatalf("unexpected prompts %+v", steps)
	}
	got, err := p.Submit(context.Background(), steps[1].ID, "Cookie: sid=session; arl=token-value")
	if err != nil || got != "token-value" {
		t.Fatalf("submit ARL = %q, %v", got, err)
	}
	if _, err := p.Submit(context.Background(), "unknown", "x"); err == nil {
		t.Fatal("expected an error for an unknown step")
	}
}

func TestDeezerHasEveryCapability(t *testing.T) {
	p, err := New(provider.Config{})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	want := provider.Capabilities{Playback: true, QualityLadder: true, Login: true, Radio: true, Favorites: true}
	if got := provider.CapabilitiesOf(p); got != want {
		t.Fatalf("CapabilitiesOf(deezer) = %+v, want %+v", got, want)
	}
}
