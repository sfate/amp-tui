package provider

import (
	"context"
	"testing"

	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/core"
)

type stubProvider struct{ id ID }

func (s stubProvider) ID() ID              { return s.id }
func (s stubProvider) Name() string        { return string(s.id) }
func (s stubProvider) Sections() []Section { return nil }
func (s stubProvider) Bootstrap(context.Context) (Library, error) {
	return Library{}, nil
}
func (s stubProvider) LoadSection(context.Context, string) ([]core.Track, error) { return nil, nil }
func (s stubProvider) LoadPlaylist(context.Context, string) ([]core.Track, error) {
	return nil, nil
}
func (s stubProvider) Search(context.Context, string) (SearchResult, error) {
	return SearchResult{}, nil
}

func stubFactory(id ID) Factory {
	return func(config.Config) (Provider, error) { return stubProvider{id: id}, nil }
}

func TestRegistryLookupUnknownIDReturnsFalse(t *testing.T) {
	reg := NewRegistry()
	reg.Register("one", "One", stubFactory("one"))

	if _, ok := reg.Lookup("missing"); ok {
		t.Fatal("expected unknown provider ID to be missing")
	}
	f, ok := reg.Lookup("one")
	if !ok {
		t.Fatal("expected registered provider ID to resolve")
	}
	p, err := f(config.Default())
	if err != nil || p.ID() != "one" {
		t.Fatalf("unexpected provider %v, err %v", p, err)
	}
}

func TestRegistryDefaultIsFirstRegistered(t *testing.T) {
	reg := NewRegistry()
	if reg.Default() != "" {
		t.Fatalf("expected empty default for empty registry, got %q", reg.Default())
	}
	reg.Register("one", "One", stubFactory("one"))
	reg.Register("two", "Two", stubFactory("two"))
	if reg.Default() != "one" {
		t.Fatalf("expected first registered provider as default, got %q", reg.Default())
	}
	if reg.Name("two") != "Two" {
		t.Fatalf("expected provider name, got %q", reg.Name("two"))
	}
}

func TestRegistryRejectsDuplicateID(t *testing.T) {
	reg := NewRegistry()
	reg.Register("one", "One", stubFactory("one"))
	defer func() {
		if recover() == nil {
			t.Fatal("expected duplicate registration to panic")
		}
	}()
	reg.Register("one", "Again", stubFactory("one"))
}
