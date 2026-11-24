package provider

import (
	"context"
	"testing"

	"github.com/sfate/amp-tui/internal/core"
)

type capStream struct {
	stubProvider
	sections  []Section
	qualities []core.Quality
}

func (c capStream) Sections() []Section          { return c.sections }
func (c capStream) Qualities() []core.Quality    { return c.qualities }
func (c capStream) DefaultQuality() core.Quality { return c.qualities[0] }
func (c capStream) Resolve(context.Context, string, core.Quality) (Source, error) {
	return Source{}, nil
}

type capFull struct{ capStream }

func (capFull) LoadRadioPage(context.Context, string, int) ([]core.Track, error) { return nil, nil }
func (capFull) LoginSteps() []LoginStep                                          { return nil }
func (capFull) Submit(context.Context, string, string) (string, error)           { return "", nil }

func TestCapabilitiesOfNilIsZero(t *testing.T) {
	if got := CapabilitiesOf(nil); got != (Capabilities{}) {
		t.Fatalf("CapabilitiesOf(nil) = %+v, want zero", got)
	}
}

func TestCapabilitiesOfBareProviderIsAllFalse(t *testing.T) {
	if got := CapabilitiesOf(stubProvider{id: "bare"}); got != (Capabilities{}) {
		t.Fatalf("CapabilitiesOf(bare) = %+v, want zero", got)
	}
}

func TestCapabilitiesOfFullProviderIsAllTrue(t *testing.T) {
	p := capFull{capStream{
		stubProvider: stubProvider{id: "full"},
		sections:     []Section{{ID: "radio", Kind: SectionRadio}, {ID: "liked", Kind: SectionFavorites}},
		qualities:    []core.Quality{{ID: "low"}, {ID: "high"}},
	}}
	want := Capabilities{Playback: true, QualityLadder: true, Login: true, Radio: true, Favorites: true}
	if got := CapabilitiesOf(p); got != want {
		t.Fatalf("CapabilitiesOf(full) = %+v, want %+v", got, want)
	}
}

func TestCapabilitiesOfSingleQualityHasNoLadder(t *testing.T) {
	p := capStream{stubProvider: stubProvider{id: "one"}, qualities: []core.Quality{{ID: "only"}}}
	want := Capabilities{Playback: true}
	if got := CapabilitiesOf(p); got != want {
		t.Fatalf("CapabilitiesOf(one quality) = %+v, want %+v", got, want)
	}
}

func TestRadioNeedsSectionAndInfiniteRadio(t *testing.T) {
	noSection := capFull{capStream{stubProvider: stubProvider{id: "a"}, qualities: []core.Quality{{ID: "q"}}}}
	if CapabilitiesOf(noSection).Radio {
		t.Fatal("expected Radio false without a SectionRadio section")
	}
	noPager := capStream{stubProvider: stubProvider{id: "b"}, sections: []Section{{ID: "radio", Kind: SectionRadio}}}
	if CapabilitiesOf(noPager).Radio {
		t.Fatal("expected Radio false without InfiniteRadio")
	}
}
