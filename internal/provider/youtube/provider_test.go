package youtube

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
)

type runnerFunc func(context.Context, string, ...string) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, binary string, args ...string) ([]byte, error) {
	return f(ctx, binary, args...)
}

func TestYouTubeMusicSearchSongsAndAlbums(t *testing.T) {
	var mu sync.Mutex
	var calls [][]string
	runner := runnerFunc(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		mu.Lock()
		calls = append(calls, append([]string(nil), args...))
		mu.Unlock()
		target := args[len(args)-1]
		switch {
		case strings.Contains(target, "#songs"):
			return []byte(`{"entries":[{"id":"abcdefghijk","url":"https://music.youtube.com/watch?v=abcdefghijk","title":"Flat song"}]}`), nil
		case strings.Contains(target, "#albums"):
			return []byte(`{"entries":[{"id":"MPREb_album1","url":"https://music.youtube.com/browse/MPREb_album1"}]}`), nil
		case strings.Contains(target, "watch?v=abcdefghijk"):
			return []byte(`{"id":"abcdefghijk","title":"Song","artist":"Artist","album":"Record"}`), nil
		case strings.Contains(target, "/browse/MPREb_album1"):
			return []byte(`{"id":"MPREb_album1","title":"Album - Record","entries":[{"id":"songid12345","title":"One"}]}`), nil
		default:
			return nil, errors.New("unexpected target: " + target)
		}
	})
	p := newProvider(provider.Config{}, "yt-dlp", runner)
	results, err := p.Search(context.Background(), "A & B")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results.Tracks) != 1 || results.Tracks[0].Title != "Song" || results.Tracks[0].Artist != "Artist" || results.Tracks[0].Album != "Record" {
		t.Fatalf("song results = %+v", results.Tracks)
	}
	if len(results.Playlists) != 1 || results.Playlists[0] != (core.Playlist{ID: "https://music.youtube.com/browse/MPREb_album1", Title: "Album - Record"}) {
		t.Fatalf("album results = %+v", results.Playlists)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 4 {
		t.Fatalf("yt-dlp calls = %d, want songs, albums and two metadata lookups", len(calls))
	}
	var sawSongs, sawAlbums bool
	for _, call := range calls {
		target := call[len(call)-1]
		sawSongs = sawSongs || strings.Contains(target, "#songs")
		sawAlbums = sawAlbums || strings.Contains(target, "#albums")
	}
	if !sawSongs || !sawAlbums {
		t.Fatalf("yt-dlp calls did not request #songs and #albums: %q", calls)
	}
}

func TestResolveUsesAudioOnlyURLCachesAndInvalidates(t *testing.T) {
	calls := 0
	runner := runnerFunc(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls++
		joined := strings.Join(args, " ")
		for _, want := range []string{"--format bestaudio", "--skip-download", "--no-playlist"} {
			if !strings.Contains(joined, want) {
				t.Errorf("Resolve args %q missing %q", joined, want)
			}
		}
		return []byte(`{"id":"abcdefghijk","title":"Song","artist":"Artist","album":"Record","duration":182.5,"thumbnail":"https://img.example/art.jpg","url":"https://audio.example/signed","ext":"m4a","http_headers":{"User-Agent":"fixture-agent","Referer":"https://music.youtube.com/"}}`), nil
	})
	p := newProvider(provider.Config{}, "yt-dlp", runner)
	quality := p.DefaultQuality()
	first, err := p.Resolve(context.Background(), "abcdefghijk", quality)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	second, err := p.Resolve(context.Background(), "abcdefghijk", quality)
	if err != nil {
		t.Fatalf("cached Resolve: %v", err)
	}
	if calls != 1 || second.URL != first.URL {
		t.Fatalf("calls=%d URLs=%q/%q, want one cached URL", calls, first.URL, second.URL)
	}
	if first.Bytes != nil || first.URL != "https://audio.example/signed" || first.Format != core.FormatAAC || first.HTTPHeaders["Referer"] == "" {
		t.Fatalf("resolved source = %+v", first)
	}
	if first.Info.DurationMS != 182_500 || first.Info.AlbumArtURL == nil || *first.Info.AlbumArtURL != "https://img.example/art.jpg" {
		t.Fatalf("resolved info = %+v", first.Info)
	}
	p.InvalidateResolvedSource("abcdefghijk")
	if _, err := p.Resolve(context.Background(), "abcdefghijk", quality); err != nil {
		t.Fatalf("Resolve after invalidation: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls after invalidation = %d, want 2", calls)
	}
}

func TestSignedInSectionsAndCookieLogin(t *testing.T) {
	runner := runnerFunc(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if !contains(args, "--cookies-from-browser") || !contains(args, "chrome:Default") {
			return nil, errors.New("browser cookie args missing")
		}
		return []byte(`{"entries":[{"id":"abcdefghijk","title":"Song","artist":"Artist"}]}`), nil
	})
	p := newProvider(provider.Config{}, "yt-dlp", runner)
	secret, err := p.Submit(context.Background(), stepBrowser, "chrome:Default")
	if err != nil || secret != "browser:chrome:Default" {
		t.Fatalf("Submit = %q, %v", secret, err)
	}
	loggedIn := newProvider(provider.Config{Credential: secret}, "yt-dlp", runner)
	sections := loggedIn.Sections()
	if len(sections) != 3 || sections[0].ID != "recommended" || sections[1].Kind != provider.SectionFavorites || sections[2].ID != "watch-later" {
		t.Fatalf("signed-in sections = %+v", sections)
	}
	if got := p.Sections(); len(got) != 0 {
		t.Fatalf("anonymous sections = %+v, want search-only", got)
	}
	tracks, err := loggedIn.LoadSection(context.Background(), "liked")
	if err != nil || len(tracks) != 1 || tracks[0].ID != "abcdefghijk" {
		t.Fatalf("LoadSection = %+v, %v", tracks, err)
	}
}

func TestCookiesFileLoginValidatesPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(path, []byte("# Netscape HTTP Cookie File\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := runnerFunc(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if !contains(args, "--cookies") || !contains(args, path) {
			t.Fatalf("cookie file args = %q", args)
		}
		return []byte(`{"entries":[]}`), nil
	})
	p := newProvider(provider.Config{}, "yt-dlp", runner)
	secret, err := p.Submit(context.Background(), stepCookiesFile, path)
	if err != nil || secret != "file:"+path {
		t.Fatalf("Submit = %q, %v", secret, err)
	}
	if _, err := p.Submit(context.Background(), stepCookiesFile, filepath.Join(dir, "missing.txt")); err == nil {
		t.Fatal("expected missing cookies file error")
	}
}

func TestURLValidationIsYouTubeOnly(t *testing.T) {
	for _, test := range []struct {
		value string
		want  bool
	}{{"https://www.youtube.com/playlist?list=PL123", true}, {"https://music.youtube.com/browse/MPREb_123", true}, {"https://youtu.be/abcdefghijk", true}, {"http://youtube.com/playlist?list=PL123", false}, {"https://youtube.com:8080/playlist?list=PL123", false}, {"https://youtube.com@evil.example/playlist?list=PL123", false}, {"https://example.com/playlist?list=PL123", false}, {"file:///etc/passwd", false}} {
		_, got := youtubeTarget(test.value)
		if got != test.want {
			t.Errorf("youtubeTarget(%q) = %t, want %t", test.value, got, test.want)
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
