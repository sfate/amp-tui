package deezer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/sanitize"
)

// qualitySpec ties a ladder step to the Deezer media format behind it.
type qualitySpec struct {
	quality     core.Quality
	media       AudioQuality
	format      core.AudioFormat
	formatCode  uint8
	bytesPerSec int
}

// Quality IDs match the config values written before the provider contract existed.
var qualitySpecs = []qualitySpec{
	{quality: core.Quality{ID: "Kbps128", Label: "128 kbps", Seekable: true}, media: AudioQuality128, format: core.FormatMP3, formatCode: 1, bytesPerSec: 16_000},
	{quality: core.Quality{ID: "Kbps320", Label: "320 kbps", Seekable: true}, media: AudioQuality320, format: core.FormatMP3, formatCode: 3, bytesPerSec: 40_000},
	{quality: core.Quality{ID: "Flac", Label: "FLAC", Seekable: false}, media: AudioQualityFlac, format: core.FormatFLAC, formatCode: 9, bytesPerSec: 90_000},
}

const defaultQualityIndex = 1

var _ provider.StreamSource = (*Provider)(nil)

func (p *Provider) Qualities() []core.Quality {
	out := make([]core.Quality, 0, len(qualitySpecs))
	for _, spec := range qualitySpecs {
		out = append(out, spec.quality)
	}
	return out
}

func (p *Provider) DefaultQuality() core.Quality {
	return qualitySpecs[defaultQualityIndex].quality
}

func (p *Provider) Resolve(ctx context.Context, trackID string, q core.Quality) (provider.Source, error) {
	if p.client == nil {
		return provider.Source{}, ErrUnauthenticated
	}
	spec, ok := specByID(q.ID)
	if !ok {
		return provider.Source{}, fmt.Errorf("unknown Deezer quality %q", q.ID)
	}
	if _, err := p.client.FetchAPIToken(ctx); err != nil {
		return provider.Source{}, err
	}
	meta, err := p.client.FetchTrackMetadata(ctx, trackID)
	if err != nil {
		return provider.Source{}, err
	}
	signedURL, err := p.client.FetchMediaURL(ctx, MediaRequest{TrackToken: meta.TrackToken, Quality: spec.media})
	if err != nil {
		return provider.Source{}, err
	}
	return provider.Source{
		Info:           trackInfo(meta),
		Quality:        spec.quality,
		Format:         spec.format,
		Bytes:          newByteSource(p.client, signedURL, meta.ID),
		Seekable:       spec.quality.Seekable,
		BytesPerSecond: spec.bytesPerSec,
	}, nil
}

// FormatCode is the Deezer CDN format number of q, or 0 for an unknown quality.
func FormatCode(q core.Quality) uint8 {
	spec, _ := specByID(q.ID)
	return spec.formatCode
}

func QualityFromFormatCode(code uint8) (core.Quality, bool) {
	for _, spec := range qualitySpecs {
		if spec.formatCode == code {
			return spec.quality, true
		}
	}
	return core.Quality{}, false
}

func specByID(id string) (qualitySpec, bool) {
	for _, spec := range qualitySpecs {
		if spec.quality.ID == id {
			return spec, true
		}
	}
	return qualitySpec{}, false
}

func trackInfo(meta TrackMetadata) core.TrackInfo {
	info := core.TrackInfo{
		ID:          meta.ID,
		Title:       sanitize.Text(meta.Title),
		Artist:      sanitize.Text(meta.Artist),
		Album:       sanitize.Text(meta.Album),
		AlbumArtURL: meta.AlbumArtURL,
	}
	if meta.DurationSecs != nil {
		info.DurationMS = *meta.DurationSecs * 1000
	}
	return info
}

// byteSource reads a signed CDN URL and decrypts its Blowfish stripes.
type byteSource struct {
	client *Client
	url    string
	key    [16]byte
	size   atomic.Int64
}

func newByteSource(client *Client, signedURL, trackID string) *byteSource {
	src := &byteSource{client: client, url: signedURL, key: DeriveBlowfishKey(trackID)}
	src.size.Store(-1)
	return src
}

func (b *byteSource) Size() int64 {
	return b.size.Load()
}

// OpenRange starts at the stripe that holds offset, because decryption works per stripe.
func (b *byteSource) OpenRange(ctx context.Context, offset int64) (io.ReadCloser, error) {
	if offset < 0 {
		return nil, fmt.Errorf("negative stream offset %d", offset)
	}
	aligned := offset - offset%chunkSize
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.url, nil)
	if err != nil {
		return nil, fmt.Errorf("build signed stream request: %w", err)
	}
	b.client.applyHeaders(req, true)
	if aligned > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", aligned))
	}

	resp, err := b.client.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open signed Deezer audio stream: %w", err)
	}
	switch {
	case aligned > 0 && resp.StatusCode != http.StatusPartialContent:
		_ = resp.Body.Close()
		return nil, fmt.Errorf("deezer CDN ignored the Range request for offset %d (status %d)", aligned, resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		_ = resp.Body.Close()
		return nil, fmt.Errorf("signed Deezer audio stream returned status %d", resp.StatusCode)
	}
	if total, ok := responseTotalSize(resp, aligned); ok {
		b.size.Store(total)
	}

	return &stripeReader{
		body:  resp.Body,
		key:   b.key[:],
		index: int(aligned / chunkSize),
		skip:  int(offset - aligned),
		buf:   make([]byte, chunkSize),
	}, nil
}

func responseTotalSize(resp *http.Response, aligned int64) (int64, bool) {
	if resp.StatusCode == http.StatusPartialContent {
		contentRange := resp.Header.Get("Content-Range")
		if slash := strings.LastIndexByte(contentRange, '/'); slash >= 0 {
			if total, err := strconv.ParseInt(contentRange[slash+1:], 10, 64); err == nil {
				return total, true
			}
		}
	}
	if resp.ContentLength < 0 {
		return 0, false
	}
	return aligned + resp.ContentLength, true
}

// stripeReader decrypts whole 2048-byte stripes, so each Read may hold one stripe back.
type stripeReader struct {
	body  io.ReadCloser
	key   []byte
	index int
	skip  int
	buf   []byte
	data  []byte
	err   error
}

func (r *stripeReader) Read(p []byte) (int, error) {
	for len(r.data) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		r.fill()
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func (r *stripeReader) fill() {
	n, err := io.ReadFull(r.body, r.buf)
	if n > 0 {
		stripe := r.buf[:n]
		if decErr := DecryptChunkInPlaceWithKey(r.key, r.index, stripe); decErr != nil {
			r.err = decErr
			return
		}
		r.index++
		skip := min(r.skip, n)
		r.skip -= skip
		r.data = stripe[skip:]
	}
	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		r.err = io.EOF
	case err != nil:
		r.err = fmt.Errorf("read Deezer audio stream: %w", err)
	}
}

func (r *stripeReader) Close() error {
	return r.body.Close()
}
