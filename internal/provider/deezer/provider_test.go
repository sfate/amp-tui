package deezer

import (
	"context"
	"errors"
	"testing"

	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/provider"
)

func TestProviderSectionsAndCapabilities(t *testing.T) {
	p, err := New(config.Default())
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
	p, err := New(config.Default())
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
	p, err := New(config.Config{ARL: "token"})
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

func TestAuthenticatorNormalizesAndMasksSecrets(t *testing.T) {
	p, _ := New(config.Default())
	got, err := p.NormalizeSecret("Cookie: sid=session; arl=token-value")
	if err != nil || got != "token-value" {
		t.Fatalf("normalize = %q, %v", got, err)
	}
	if p.MaskSecret("abcd1234wxyz") != "abcd****wxyz" {
		t.Fatalf("unexpected mask %q", p.MaskSecret("abcd1234wxyz"))
	}
	if p.MaskSecret("sid=x; arl=secret") != "Cookie header pasted" {
		t.Fatalf("expected cookie header to stay hidden, got %q", p.MaskSecret("sid=x; arl=secret"))
	}
	if p.LoginPrompt().ConfirmText != "Press Enter to open Deezer login in your browser" {
		t.Fatalf("unexpected confirm text %q", p.LoginPrompt().ConfirmText)
	}
}

func TestDeezerHasEveryCapability(t *testing.T) {
	p, err := New(config.Default())
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	want := provider.Capabilities{Playback: true, QualityLadder: true, Login: true, Radio: true, Favorites: true}
	if got := provider.CapabilitiesOf(p); got != want {
		t.Fatalf("CapabilitiesOf(deezer) = %+v, want %+v", got, want)
	}
}
