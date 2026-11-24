package deezer

import (
	"bytes"
	"context"
	"crypto/cipher"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/blowfish" //nolint:staticcheck // Tests must mirror Deezer's legacy Blowfish stream encryption.

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
)

func encryptStripes(t *testing.T, trackID string, plaintext []byte) []byte {
	t.Helper()
	key := DeriveBlowfishKey(trackID)
	block, err := blowfish.NewCipher(key[:])
	if err != nil {
		t.Fatalf("new blowfish cipher: %v", err)
	}
	encrypted := append([]byte(nil), plaintext...)
	for index, start := 0, 0; start < len(encrypted); index, start = index+1, start+chunkSize {
		if index%3 != 0 {
			continue
		}
		stripe := encrypted[start:min(start+chunkSize, len(encrypted))]
		n := len(stripe) - len(stripe)%blowfishBlockSize
		cipher.NewCBCEncrypter(block, deezerBlowfishIV).CryptBlocks(stripe[:n], stripe[:n])
	}
	return encrypted
}

func testPlaintext() []byte {
	plain := make([]byte, chunkSize*7+700)
	for i := range plain {
		plain[i] = byte(i % 251)
	}
	return plain
}

type rangeRecorder struct {
	mu     sync.Mutex
	ranges []string
}

func (r *rangeRecorder) record(value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ranges = append(r.ranges, value)
}

func (r *rangeRecorder) last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.ranges) == 0 {
		return ""
	}
	return r.ranges[len(r.ranges)-1]
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	client, err := NewClient("test-arl", Options{GatewayURL: baseURL, MediaURL: baseURL, FlowAPIBaseURL: baseURL})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func TestByteSourceOpenRangeDecryptsFromOffset(t *testing.T) {
	plain := testPlaintext()
	encrypted := encryptStripes(t, "42", plain)
	rec := &rangeRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r.Header.Get("Range"))
		http.ServeContent(w, r, "track", time.Time{}, bytes.NewReader(encrypted))
	}))
	defer server.Close()

	for _, tc := range []struct {
		offset    int64
		wantRange string
	}{
		{offset: 0, wantRange: ""},
		{offset: 2048, wantRange: "bytes=2048-"},
		{offset: 3000, wantRange: "bytes=2048-"},
		{offset: 6144 + 5, wantRange: "bytes=6144-"},
	} {
		src := newByteSource(newTestClient(t, server.URL), server.URL+"/cdn", "42")
		body, err := src.OpenRange(context.Background(), tc.offset)
		if err != nil {
			t.Fatalf("offset %d: open range: %v", tc.offset, err)
		}
		got, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			t.Fatalf("offset %d: read: %v", tc.offset, err)
		}
		if !bytes.Equal(got, plain[tc.offset:]) {
			t.Fatalf("offset %d: decrypted bytes mismatch (got %d bytes, want %d)", tc.offset, len(got), len(plain)-int(tc.offset))
		}
		if rec.last() != tc.wantRange {
			t.Fatalf("offset %d: Range header = %q, want %q", tc.offset, rec.last(), tc.wantRange)
		}
		if src.Size() != int64(len(encrypted)) {
			t.Fatalf("offset %d: size = %d, want %d", tc.offset, src.Size(), len(encrypted))
		}
	}
}

func TestByteSourceOpenRangeFailsWhenServerIgnoresRange(t *testing.T) {
	encrypted := encryptStripes(t, "42", testPlaintext())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(encrypted)
	}))
	defer server.Close()

	src := newByteSource(newTestClient(t, server.URL), server.URL+"/cdn", "42")
	body, err := src.OpenRange(context.Background(), 3000)
	if err == nil {
		_ = body.Close()
		t.Fatal("expected an error when the server ignores Range")
	}
	if !strings.Contains(err.Error(), "Range") {
		t.Fatalf("expected a Range error, got %v", err)
	}

	body, err = src.OpenRange(context.Background(), 0)
	if err != nil {
		t.Fatalf("offset 0 must not need Range: %v", err)
	}
	_ = body.Close()
}

func TestProviderQualityLadder(t *testing.T) {
	p, err := New(provider.Config{})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	var asProvider provider.Provider = p
	streams, ok := asProvider.(provider.StreamSource)
	if !ok {
		t.Fatal("expected Deezer provider to implement StreamSource")
	}
	want := []core.Quality{
		{ID: "Kbps128", Label: "128 kbps", Seekable: true},
		{ID: "Kbps320", Label: "320 kbps", Seekable: true},
		{ID: "Flac", Label: "FLAC", Seekable: false},
	}
	got := streams.Qualities()
	if len(got) != len(want) {
		t.Fatalf("qualities = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("quality %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if streams.DefaultQuality() != want[1] {
		t.Fatalf("default quality = %+v, want %+v", streams.DefaultQuality(), want[1])
	}
}

func TestQualityFormatCodes(t *testing.T) {
	for _, tc := range []struct {
		id   string
		code uint8
	}{
		{id: "Kbps128", code: 1},
		{id: "Kbps320", code: 3},
		{id: "Flac", code: 9},
	} {
		if got := FormatCode(core.Quality{ID: tc.id}); got != tc.code {
			t.Fatalf("FormatCode(%s) = %d, want %d", tc.id, got, tc.code)
		}
		q, ok := QualityFromFormatCode(tc.code)
		if !ok || q.ID != tc.id {
			t.Fatalf("QualityFromFormatCode(%d) = %+v/%t, want %s", tc.code, q, ok, tc.id)
		}
	}
	if FormatCode(core.Quality{ID: "unknown"}) != 0 {
		t.Fatal("expected 0 for an unknown quality")
	}
	if _, ok := QualityFromFormatCode(2); ok {
		t.Fatal("expected no quality for format code 2")
	}
}

// fakeGateway serves the Deezer gateway, media and CDN endpoints for Resolve.
func fakeGateway(t *testing.T, cdn []byte, duration any, mediaFormat *string) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/cdn":
			http.ServeContent(w, r, "track", time.Time{}, bytes.NewReader(cdn))
		case r.URL.Path == "/media":
			var body struct {
				Media []struct {
					Formats []struct {
						Format string `json:"format"`
					} `json:"formats"`
				} `json:"media"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.Media) > 0 && len(body.Media[0].Formats) > 0 {
				*mediaFormat = body.Media[0].Formats[0].Format
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{
				"media": []any{map[string]any{"sources": []any{map[string]any{"url": server.URL + "/cdn"}}}},
			}}})
		case r.URL.Query().Get("method") == "deezer.getUserData":
			_ = json.NewEncoder(w).Encode(map[string]any{"results": map[string]any{
				"checkForm": "api-token",
				"USER":      map[string]any{"USER_ID": 7, "OPTIONS": map[string]any{"license_token": "license"}},
			}})
		case r.URL.Query().Get("method") == "deezer.pageTrack":
			data := map[string]any{
				"SNG_ID":      "42",
				"SNG_TITLE":   "Song\x1b[31m",
				"ART_NAME":    "Artist",
				"ALB_TITLE":   "Album",
				"TRACK_TOKEN": "track-token",
				"ALB_PICTURE": "abc",
			}
			if duration != nil {
				data["DURATION"] = duration
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": map[string]any{"DATA": data}})
		default:
			t.Errorf("unexpected request %s %s", r.URL.Path, r.URL.RawQuery)
			http.NotFound(w, r)
		}
	}))
	return server
}

func newGatewayProvider(t *testing.T, server *httptest.Server) *Provider {
	t.Helper()
	client, err := NewClient("test-arl", Options{
		GatewayURL:     server.URL + "/gw",
		MediaURL:       server.URL + "/media",
		FlowAPIBaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return &Provider{client: client}
}

func TestResolveReturnsPlayableSource(t *testing.T) {
	plain := testPlaintext()
	var mediaFormat string
	server := fakeGateway(t, encryptStripes(t, "42", plain), 215, &mediaFormat)
	defer server.Close()
	p := newGatewayProvider(t, server)

	for _, tc := range []struct {
		quality     core.Quality
		mediaFormat string
		format      core.AudioFormat
		seekable    bool
		bytesPerSec int
	}{
		{quality: p.Qualities()[0], mediaFormat: "MP3_128", format: core.FormatMP3, seekable: true, bytesPerSec: 16_000},
		{quality: p.Qualities()[1], mediaFormat: "MP3_320", format: core.FormatMP3, seekable: true, bytesPerSec: 40_000},
		{quality: p.Qualities()[2], mediaFormat: "FLAC", format: core.FormatFLAC, seekable: false, bytesPerSec: 90_000},
	} {
		src, err := p.Resolve(context.Background(), "42", tc.quality)
		if err != nil {
			t.Fatalf("%s: resolve: %v", tc.quality.ID, err)
		}
		if mediaFormat != tc.mediaFormat {
			t.Fatalf("%s: media format = %q, want %q", tc.quality.ID, mediaFormat, tc.mediaFormat)
		}
		if src.Format != tc.format || src.Seekable != tc.seekable || src.BytesPerSecond != tc.bytesPerSec || src.Quality != tc.quality {
			t.Fatalf("%s: unexpected source %+v", tc.quality.ID, src)
		}
		if src.URL != "" || src.Bytes == nil {
			t.Fatalf("%s: expected a ByteSource and no URL, got %+v", tc.quality.ID, src)
		}
		info := src.Info
		if info.ID != "42" || info.Title != "Song" || info.Artist != "Artist" || info.Album != "Album" || info.DurationMS != 215_000 {
			t.Fatalf("%s: unexpected track info %+v", tc.quality.ID, info)
		}
		if info.AlbumArtURL == nil || !strings.Contains(*info.AlbumArtURL, "/abc/") {
			t.Fatalf("%s: unexpected album art %v", tc.quality.ID, info.AlbumArtURL)
		}
		body, err := src.Bytes.OpenRange(context.Background(), 0)
		if err != nil {
			t.Fatalf("%s: open range: %v", tc.quality.ID, err)
		}
		got, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil || !bytes.Equal(got, plain) {
			t.Fatalf("%s: source bytes mismatch (err %v)", tc.quality.ID, err)
		}
	}
}

func TestResolveWithoutDurationLeavesItZero(t *testing.T) {
	var mediaFormat string
	server := fakeGateway(t, encryptStripes(t, "42", testPlaintext()), nil, &mediaFormat)
	defer server.Close()
	p := newGatewayProvider(t, server)

	src, err := p.Resolve(context.Background(), "42", p.DefaultQuality())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if src.Info.DurationMS != 0 {
		t.Fatalf("expected unknown duration, got %d", src.Info.DurationMS)
	}
}

func TestResolveRejectsUnknownQualityAndSignedOut(t *testing.T) {
	p, err := New(provider.Config{})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, err := p.Resolve(context.Background(), "42", p.DefaultQuality()); err == nil {
		t.Fatal("expected signed-out Resolve to fail")
	}

	var mediaFormat string
	server := fakeGateway(t, nil, 1, &mediaFormat)
	defer server.Close()
	signedIn := newGatewayProvider(t, server)
	if _, err := signedIn.Resolve(context.Background(), "42", core.Quality{ID: "Opus"}); err == nil {
		t.Fatal("expected an unknown quality to fail")
	}
}
