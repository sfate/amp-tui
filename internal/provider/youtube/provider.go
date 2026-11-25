package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/sanitize"
)

const ID provider.ID = "youtube"

const (
	searchLimit       = 12
	playlistLimit     = 500
	resolvedURLMaxAge = 5 * time.Hour
	stepCookiesFile   = "cookies_file"
	stepBrowser       = "browser"
)

var anonymousSections = []provider.Section{}
var signedInSections = []provider.Section{
	{ID: "recommended", Label: "Recommended", Kind: provider.SectionTracks},
	{ID: "liked", Label: "Liked", Kind: provider.SectionFavorites},
	{ID: "watch-later", Label: "Watch later", Kind: provider.SectionTracks},
}

type cachedSource struct {
	source    provider.Source
	expiresAt time.Time
}

// Provider implements audio-only YouTube Music catalogs and yt-dlp URL resolution.
type Provider struct {
	binary     string
	runner     commandRunner
	credential string

	mu       sync.Mutex
	resolved map[string]cachedSource
}

var (
	_ provider.Provider                  = (*Provider)(nil)
	_ provider.StreamSource              = (*Provider)(nil)
	_ provider.Authenticator             = (*Provider)(nil)
	_ provider.AnonymousProvider         = (*Provider)(nil)
	_ provider.ResolvedSourceInvalidator = (*Provider)(nil)
)

func Register(reg *provider.Registry) {
	reg.Register(ID, "YouTube", func(cfg provider.Config) (provider.Provider, error) {
		return New(cfg), nil
	})
}

func New(cfg provider.Config) *Provider {
	binary, _ := exec.LookPath("yt-dlp")
	return newProvider(cfg, binary, execRunner{})
}

func newProvider(cfg provider.Config, binary string, runner commandRunner) *Provider {
	return &Provider{binary: binary, runner: runner, credential: strings.TrimSpace(cfg.Credential), resolved: make(map[string]cachedSource)}
}

func (p *Provider) ID() provider.ID { return ID }

func (p *Provider) Name() string { return "YouTube" }

func (p *Provider) SupportsAnonymousAccess() bool { return true }

func (p *Provider) Sections() []provider.Section {
	if !p.authenticated() {
		return append([]provider.Section(nil), anonymousSections...)
	}
	return append([]provider.Section(nil), signedInSections...)
}

func (p *Provider) Bootstrap(ctx context.Context) (provider.Library, error) {
	if !p.authenticated() {
		return provider.Library{}, nil
	}
	if _, err := p.loadFlat(ctx, ":ytfav", 1); err != nil {
		return provider.Library{}, fmt.Errorf("validate YouTube cookies with :ytfav: %w", err)
	}
	feed, err := p.loadFlat(ctx, "https://www.youtube.com/feed/playlists", playlistLimit)
	if err != nil {
		return provider.Library{}, fmt.Errorf("load YouTube playlists: %w", err)
	}
	playlists := make([]core.Playlist, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		if id := playlistURL(entry); id != "" {
			playlists = append(playlists, core.Playlist{ID: id, Title: clean(entry.Title)})
		}
	}
	return provider.Library{Playlists: playlists}, nil
}

func (p *Provider) LoadSection(ctx context.Context, sectionID string) ([]core.Track, error) {
	if !p.authenticated() {
		return nil, errors.New("sign in to YouTube to browse this section")
	}
	targets := map[string]string{
		"recommended": ":ytrec",
		"liked":       ":ytfav",
		"watch-later": ":ytwatchlater",
	}
	target, ok := targets[sectionID]
	if !ok {
		return nil, fmt.Errorf("unknown YouTube section %q", sectionID)
	}
	listing, err := p.loadFlat(ctx, target, playlistLimit)
	if err != nil {
		return nil, err
	}
	return tracksFromEntries(listing.Entries), nil
}

func (p *Provider) LoadPlaylist(ctx context.Context, id string) ([]core.Track, error) {
	target, ok := youtubeTarget(id)
	if !ok {
		return nil, errors.New("YouTube playlist URL must use a supported youtube.com domain")
	}
	listing, err := p.loadFlat(ctx, target, playlistLimit)
	if err != nil {
		return nil, err
	}
	return tracksFromEntries(listing.Entries), nil
}

func (p *Provider) Search(ctx context.Context, query string) (provider.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return provider.SearchResult{}, nil
	}
	if target, ok := youtubeTarget(query); ok {
		if isVideoURL(target) {
			item, err := p.printEntry(ctx, target, false)
			if err != nil {
				return provider.SearchResult{}, err
			}
			return provider.SearchResult{Tracks: []core.Track{coreTrack(item)}}, nil
		}
		listing, err := p.loadFlat(ctx, target, playlistLimit)
		if err != nil {
			return provider.SearchResult{}, err
		}
		return provider.SearchResult{Tracks: tracksFromEntries(listing.Entries)}, nil
	}
	searchURL := musicSearchURL(query, "songs")
	songs, err := p.loadFlat(ctx, searchURL, searchLimit)
	if err != nil {
		return provider.SearchResult{}, err
	}
	tracks := enrichTracks(ctx, p, tracksFromEntries(songs.Entries))
	albumResults, err := p.loadFlat(ctx, musicSearchURL(query, "albums"), searchLimit)
	if err != nil {
		return provider.SearchResult{}, err
	}
	albums := enrichAlbums(ctx, p, albumResults.Entries)
	return provider.SearchResult{Tracks: tracks, Playlists: albums}, nil
}

func (p *Provider) Qualities() []core.Quality {
	return []core.Quality{{ID: "best-audio", Label: "Best audio", Seekable: true}}
}

func (p *Provider) DefaultQuality() core.Quality { return p.Qualities()[0] }

func (p *Provider) Resolve(ctx context.Context, trackID string, quality core.Quality) (provider.Source, error) {
	if quality.ID != "best-audio" {
		return provider.Source{}, fmt.Errorf("unknown YouTube quality %q", quality.ID)
	}
	if cached, ok := p.cached(trackID); ok {
		return cached, nil
	}
	target, ok := youtubeTarget(trackID)
	if !ok {
		if !validVideoID(trackID) {
			return provider.Source{}, errors.New("invalid YouTube track ID")
		}
		target = "https://www.youtube.com/watch?v=" + url.QueryEscape(trackID)
	}
	args := append(p.baseArgs(), p.cookieArgs()...)
	args = append(args,
		"--skip-download", "--no-playlist", "--format", "bestaudio",
		"--print", `%(.{id,title,artist,uploader,album,duration,thumbnail,url,ext,http_headers})j`, target,
	)
	output, err := p.runner.Run(ctx, p.binary, args...)
	if err != nil {
		return provider.Source{}, err
	}
	meta, err := parsePrintedEntry(output)
	if err != nil {
		return provider.Source{}, err
	}
	if meta.URL == "" {
		return provider.Source{}, errors.New("yt-dlp did not resolve an audio-only URL")
	}
	info := trackInfo(meta)
	source := provider.Source{
		Info: info, Quality: p.DefaultQuality(), Format: audioFormat(meta.Ext),
		URL: meta.URL, HTTPHeaders: meta.HTTPHeaders, Seekable: true,
	}
	p.mu.Lock()
	p.resolved[trackID] = cachedSource{source: source, expiresAt: time.Now().Add(resolvedURLMaxAge)}
	p.mu.Unlock()
	return source, nil
}

func (p *Provider) InvalidateResolvedSource(trackID string) {
	p.mu.Lock()
	delete(p.resolved, trackID)
	p.mu.Unlock()
}

func (p *Provider) LoginSteps() []provider.LoginStep {
	return []provider.LoginStep{
		{ID: stepCookiesFile, Kind: provider.LoginStepInput, Prompt: "Path to a Netscape cookies.txt file", WaitText: "Checking YouTube cookies"},
		{ID: stepBrowser, Kind: provider.LoginStepInput, Prompt: "Browser[:profile], for example chrome:Default", WaitText: "Checking YouTube browser cookies"},
	}
}

func (p *Provider) Submit(ctx context.Context, stepID, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" || strings.ContainsAny(input, "\r\n\t") {
		return "", errors.New("enter a valid cookie file path or browser profile")
	}
	var credential string
	switch stepID {
	case stepCookiesFile:
		path, err := normalizeCookiePath(input)
		if err != nil {
			return "", err
		}
		credential = "file:" + path
	case stepBrowser:
		if strings.HasPrefix(input, "-") {
			return "", errors.New("browser profile must not start with a dash")
		}
		credential = "browser:" + input
	default:
		return "", fmt.Errorf("unknown YouTube login step %q", stepID)
	}
	testProvider := newProvider(provider.Config{Credential: credential}, p.binary, p.runner)
	if _, err := testProvider.loadFlat(ctx, ":ytfav", 1); err != nil {
		return "", fmt.Errorf("YouTube cookie check failed: %w", err)
	}
	return credential, nil
}

func (p *Provider) loadFlat(ctx context.Context, target string, limit int) (listing, error) {
	args := append(p.baseArgs(), p.cookieArgs()...)
	args = append(args, "--flat-playlist", "--dump-single-json", "--skip-download", "--playlist-end", fmt.Sprint(limit), target)
	output, err := p.runner.Run(ctx, p.binary, args...)
	if err != nil {
		return listing{}, err
	}
	var result listing
	if err := json.Unmarshal(output, &result); err != nil {
		return listing{}, fmt.Errorf("parse yt-dlp JSON: %w", err)
	}
	return result, nil
}

func (p *Provider) printEntry(ctx context.Context, target string, audio bool) (entry, error) {
	args := append(p.baseArgs(), p.cookieArgs()...)
	args = append(args, "--skip-download", "--no-playlist")
	if audio {
		args = append(args, "--format", "bestaudio")
	}
	args = append(args, "--print", `%(.{id,title,artist,uploader,album,duration,thumbnail,url,ext,http_headers})j`, target)
	output, err := p.runner.Run(ctx, p.binary, args...)
	if err != nil {
		return entry{}, err
	}
	return parsePrintedEntry(output)
}

func (p *Provider) baseArgs() []string {
	return []string{"--ignore-config", "--quiet", "--no-warnings", "--no-progress", "--no-colors", "--remote-components", "ejs:npm"}
}

func (p *Provider) cookieArgs() []string {
	if strings.HasPrefix(p.credential, "file:") {
		return []string{"--cookies", strings.TrimPrefix(p.credential, "file:")}
	}
	if strings.HasPrefix(p.credential, "browser:") {
		return []string{"--cookies-from-browser", strings.TrimPrefix(p.credential, "browser:")}
	}
	return nil
}

func (p *Provider) authenticated() bool { return len(p.cookieArgs()) > 0 }

func (p *Provider) cached(trackID string) (provider.Source, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.resolved[trackID]
	if !ok {
		return provider.Source{}, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(p.resolved, trackID)
		return provider.Source{}, false
	}
	return entry.source, true
}

type listing struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Entries []entry `json:"entries"`
}

type entry struct {
	ID          string            `json:"id"`
	URL         string            `json:"url"`
	Webpage     string            `json:"webpage_url"`
	Title       string            `json:"title"`
	Artist      string            `json:"artist"`
	Uploader    string            `json:"uploader"`
	Album       string            `json:"album"`
	Duration    float64           `json:"duration"`
	Thumbnail   string            `json:"thumbnail"`
	Ext         string            `json:"ext"`
	HTTPHeaders map[string]string `json:"http_headers"`
}

func tracksFromEntries(entries []entry) []core.Track {
	tracks := make([]core.Track, 0, len(entries))
	for _, item := range entries {
		id := item.ID
		if id == "" {
			id = item.URL
		}
		if id == "" || item.Title == "" {
			continue
		}
		artist := item.Artist
		if artist == "" {
			artist = item.Uploader
		}
		tracks = append(tracks, core.Track{ID: id, Title: clean(item.Title), Artist: clean(artist), Album: clean(item.Album)})
	}
	return tracks
}

func coreTrack(item entry) core.Track {
	info := trackInfo(item)
	return core.Track{ID: info.ID, Title: info.Title, Artist: info.Artist, Album: info.Album}
}

func enrichTracks(ctx context.Context, p *Provider, tracks []core.Track) []core.Track {
	if len(tracks) == 0 {
		return tracks
	}
	type result struct {
		index int
		item  entry
	}
	jobs := make(chan int)
	results := make(chan result, len(tracks))
	workers := min(4, len(tracks))
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range jobs {
				item, err := p.printEntry(ctx, videoURL(tracks[index].ID), false)
				if err == nil {
					results <- result{index: index, item: item}
				}
			}
		}()
	}
	go func() {
		for index := range tracks {
			jobs <- index
		}
		close(jobs)
		wait.Wait()
		close(results)
	}()
	for found := range results {
		tracks[found.index] = coreTrack(found.item)
	}
	return tracks
}

func enrichAlbums(ctx context.Context, p *Provider, entries []entry) []core.Playlist {
	if len(entries) == 0 {
		return nil
	}
	type result struct {
		index int
		album core.Playlist
	}
	jobs := make(chan int)
	results := make(chan result, len(entries))
	workers := min(4, len(entries))
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range jobs {
				entry := entries[index]
				id := playlistURL(entry)
				if id == "" {
					continue
				}
				listing, err := p.loadFlat(ctx, id, 1)
				title := entry.Title
				if err == nil && listing.Title != "" {
					title = listing.Title
				}
				if title == "" {
					title = "Album"
				}
				results <- result{index: index, album: core.Playlist{ID: id, Title: clean(title)}}
			}
		}()
	}
	go func() {
		for index := range entries {
			jobs <- index
		}
		close(jobs)
		wait.Wait()
		close(results)
	}()
	ordered := make([]core.Playlist, len(entries))
	seen := make([]bool, len(entries))
	for found := range results {
		ordered[found.index] = found.album
		seen[found.index] = true
	}
	out := make([]core.Playlist, 0, len(entries))
	for index, album := range ordered {
		if seen[index] {
			out = append(out, album)
		}
	}
	return out
}

func parsePrintedEntry(output []byte) (entry, error) {
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	var item entry
	if err := decoder.Decode(&item); err != nil {
		return entry{}, fmt.Errorf("parse yt-dlp metadata: %w", err)
	}
	if item.ID == "" || item.Title == "" {
		return entry{}, errors.New("yt-dlp returned incomplete track metadata")
	}
	return item, nil
}

func trackInfo(item entry) core.TrackInfo {
	artist := item.Artist
	if artist == "" {
		artist = item.Uploader
	}
	info := core.TrackInfo{ID: item.ID, Title: clean(item.Title), Artist: clean(artist), Album: clean(item.Album)}
	if item.Duration > 0 {
		info.DurationMS = uint64(item.Duration * 1000)
	}
	if item.Thumbnail != "" {
		thumbnail := item.Thumbnail
		info.AlbumArtURL = &thumbnail
	}
	return info
}

func clean(value string) string { return sanitize.Text(value) }

func audioFormat(ext string) core.AudioFormat {
	switch strings.ToLower(ext) {
	case "m4a", "aac":
		return core.FormatAAC
	case "mp3":
		return core.FormatMP3
	default:
		return core.FormatOpus
	}
}

func musicSearchURL(query, section string) string {
	u := url.URL{Scheme: "https", Host: "music.youtube.com", Path: "/search", Fragment: section}
	values := url.Values{}
	values.Set("q", query)
	u.RawQuery = values.Encode()
	return u.String()
}

func playlistURL(item entry) string {
	for _, candidate := range []string{item.Webpage, item.URL} {
		if _, ok := youtubeTarget(candidate); ok {
			return candidate
		}
	}
	if strings.HasPrefix(item.ID, "MPREb_") || strings.HasPrefix(item.ID, "OLAK") {
		return "https://music.youtube.com/browse/" + url.PathEscape(item.ID)
	}
	if strings.HasPrefix(item.ID, "PL") || strings.HasPrefix(item.ID, "UU") || strings.HasPrefix(item.ID, "RD") {
		return "https://www.youtube.com/playlist?list=" + url.QueryEscape(item.ID)
	}
	return ""
}

func youtubeTarget(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil || u.Port() != "" {
		return "", false
	}
	switch strings.ToLower(u.Hostname()) {
	case "youtube.com", "www.youtube.com", "music.youtube.com", "youtu.be", "m.youtube.com":
	default:
		return "", false
	}
	if strings.EqualFold(u.Hostname(), "youtu.be") && strings.Trim(u.Path, "/") != "" {
		return u.String(), true
	}
	switch u.Path {
	case "/watch", "/playlist", "/feed/playlists":
		return u.String(), true
	default:
		if strings.HasPrefix(u.Path, "/browse/") && len(strings.TrimPrefix(u.Path, "/browse/")) > 0 {
			return u.String(), true
		}
		return "", false
	}
}

func isVideoURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Path == "/watch" || strings.EqualFold(u.Hostname(), "youtu.be")
}

func videoURL(id string) string {
	if _, ok := youtubeTarget(id); ok {
		return id
	}
	return "https://www.youtube.com/watch?v=" + url.QueryEscape(id)
}

func validVideoID(id string) bool {
	if len(id) != 11 {
		return false
	}
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func normalizeCookiePath(input string) (string, error) {
	if strings.HasPrefix(input, "~") {
		if input != "~" && !strings.HasPrefix(input, "~/") {
			return "", errors.New("cookie file path must use ~/ for the home directory")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		input = filepath.Join(home, strings.TrimPrefix(input, "~/"))
	}
	path, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("cookies.txt file does not exist or is not a regular file")
	}
	return path, nil
}
