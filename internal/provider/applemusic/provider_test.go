package applemusic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
)

type fakeScriptRunner struct {
	mu    sync.Mutex
	calls []scriptCall
	fn    func(context.Context, string, ...string) ([]byte, error)
}

type scriptCall struct {
	operation string
	args      []string
}

func (r *fakeScriptRunner) Run(ctx context.Context, operation string, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.calls = append(r.calls, scriptCall{operation: operation, args: append([]string(nil), args...)})
	r.mu.Unlock()
	if r.fn == nil {
		return nil, errors.New("unexpected script operation " + operation)
	}
	return r.fn(ctx, operation, args...)
}

func TestBootstrapAndLibraryCatalogUsePersistentIDs(t *testing.T) {
	runner := &fakeScriptRunner{fn: func(_ context.Context, operation string, args ...string) ([]byte, error) {
		switch operation {
		case "playlists":
			return []byte(`[{"id":"PL123","title":"Road Mix"}]`), nil
		case "library_tracks":
			return []byte(`[{"id":"TRK123","title":"Song","artist":"Artist","album":"Record","duration":81.25}]`), nil
		case "playlist_tracks":
			if len(args) != 1 || args[0] != "PL123" {
				t.Fatalf("playlist args = %q", args)
			}
			return []byte(`[{"id":"TRK123","title":"Song","artist":"Artist","album":"Record","duration":81.25}]`), nil
		default:
			return nil, errors.New("unexpected operation " + operation)
		}
	}}
	p := newProvider(provider.Config{}, runner)
	if p.ID() != ID || !provider.CapabilitiesOf(p).ManagedAudio {
		t.Fatalf("provider identity/capabilities = %q/%+v", p.ID(), provider.CapabilitiesOf(p))
	}
	if got := p.Sections(); len(got) != 1 || got[0].ID != "library" {
		t.Fatalf("sections = %+v", got)
	}
	library, err := p.Bootstrap(context.Background())
	if err != nil || len(library.Playlists) != 1 || library.Playlists[0] != (core.Playlist{ID: "PL123", Title: "Road Mix"}) {
		t.Fatalf("Bootstrap = %+v, %v", library, err)
	}
	tracks, err := p.LoadSection(context.Background(), "library")
	if err != nil || len(tracks) != 1 || tracks[0].ID != "TRK123" || tracks[0].Title != "Song" {
		t.Fatalf("LoadSection = %+v, %v", tracks, err)
	}
	tracks, err = p.LoadPlaylist(context.Background(), "PL123")
	if err != nil || len(tracks) != 1 || tracks[0].ID != "TRK123" {
		t.Fatalf("LoadPlaylist = %+v, %v", tracks, err)
	}
}

func TestSearchIsLimitedToTheMusicAppLibrary(t *testing.T) {
	runner := &fakeScriptRunner{fn: func(_ context.Context, operation string, args ...string) ([]byte, error) {
		if operation != "search_tracks" || len(args) != 1 || args[0] != "Björk" {
			t.Fatalf("script call = %q %q", operation, args)
		}
		return []byte(`[{"id":"TRK456","title":"Jóga","artist":"Björk","album":"Homogenic","duration":250}]`), nil
	}}
	p := newProvider(provider.Config{}, runner)
	result, err := p.Search(context.Background(), "Björk")
	if err != nil || len(result.Tracks) != 1 || result.Tracks[0].ID != "TRK456" || len(result.Playlists) != 0 || len(result.Artists) != 0 {
		t.Fatalf("Search = %+v, %v", result, err)
	}
}

func TestResolveReturnsManagedSourceAndArtworkDataURI(t *testing.T) {
	artwork := []byte{0xff, 0xd8, 0xff, 0x00, 0x01, 0x02}
	runner := &fakeScriptRunner{fn: func(_ context.Context, operation string, args ...string) ([]byte, error) {
		switch operation {
		case "track":
			if len(args) != 1 || args[0] != "TRK789" {
				t.Fatalf("track args = %q", args)
			}
			return []byte(`{"id":"TRK789","title":"One","artist":"A","album":"B","duration":12.5}`), nil
		case "artwork":
			return artwork, nil
		default:
			return nil, errors.New("unexpected operation " + operation)
		}
	}}
	p := newProvider(provider.Config{}, runner)
	source, err := p.Resolve(context.Background(), "TRK789", p.DefaultQuality())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantArt := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(artwork)
	if source.URL != "" || source.Bytes != nil || !source.Seekable || source.Info.ID != "TRK789" || source.Info.DurationMS != 12_500 || source.Info.AlbumArtURL == nil || *source.Info.AlbumArtURL != wantArt {
		t.Fatalf("managed source = %+v", source)
	}
	if !strings.HasPrefix(wantArt, "data:image/jpeg;base64,") {
		t.Fatalf("test fixture data URI = %q", wantArt)
	}
}

func TestProviderJSONFixtureDecodesTrackList(t *testing.T) {
	var tracks []trackRecord
	if err := json.Unmarshal([]byte(`[{"id":"A","title":"Title"}]`), &tracks); err != nil || len(tracks) != 1 || tracks[0].ID != "A" {
		t.Fatalf("fixture decode = %+v, %v", tracks, err)
	}
}
