package mpv

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
)

type testByteSource []byte

func (b testByteSource) Size() int64 { return int64(len(b)) }

func (b testByteSource) OpenRange(_ context.Context, offset int64) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(b[offset:])), nil
}

func TestSourceServerServesRangeRequests(t *testing.T) {
	data := []byte("0123456789abcdef")
	server, err := newSourceServer(context.Background(), provider.Source{
		Info:   core.TrackInfo{ID: "range-test"},
		Format: core.FormatMP3,
		Bytes:  testByteSource(data),
	})
	if err != nil {
		t.Fatalf("new source server: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	request, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Range", "bytes=4-9")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET range: %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		t.Fatalf("read range: %v", readErr)
	}
	if response.StatusCode != http.StatusPartialContent || !bytes.Equal(body, data[4:10]) {
		t.Fatalf("range response = %d %q, want 206 %q", response.StatusCode, body, data[4:10])
	}
	if got := response.Header.Get("Content-Range"); got != "bytes 4-9/16" {
		t.Fatalf("Content-Range = %q, want bytes 4-9/16", got)
	}
}

func TestSourceServerUsesUnpredictablePathToken(t *testing.T) {
	source := provider.Source{Info: core.TrackInfo{ID: "token-test"}, Bytes: testByteSource("body")}
	first, err := newSourceServer(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	second, err := newSourceServer(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	firstURL, err := url.Parse(first.URL)
	if err != nil {
		t.Fatal(err)
	}
	secondURL, err := url.Parse(second.URL)
	if err != nil {
		t.Fatal(err)
	}
	if firstURL.Path == secondURL.Path {
		t.Fatal("source servers reused the same tokenized URL")
	}
	wrongURL := *firstURL
	wrongURL.Path = firstURL.Path[:len(firstURL.Path)-len("audio")] + "wrong"
	response, err := http.Get(wrongURL.String())
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown token status = %d, want 404", response.StatusCode)
	}
}
