package mpv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/sfate/amp-tui/internal/provider"
)

type sourceServer struct {
	URL    string
	server *http.Server
	listen net.Listener
	once   sync.Once
}

func newSourceServer(ctx context.Context, source provider.Source) (*sourceServer, error) {
	if source.Bytes == nil {
		return nil, errors.New("mpv requires a byte source")
	}
	if source.Bytes.Size() <= 0 {
		body, err := source.Bytes.OpenRange(ctx, 0)
		if err != nil {
			return nil, fmt.Errorf("inspect audio source size: %w", err)
		}
		_ = body.Close()
	}
	size := source.Bytes.Size()
	if size < 0 {
		return nil, errors.New("audio source did not report a size; mpv range playback needs a known content length")
	}

	var tokenBytes [24]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return nil, fmt.Errorf("generate stream token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes[:])
	path := "/" + token + "/audio"
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for mpv audio stream: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		reader := &sourceReadSeeker{ctx: r.Context(), source: source.Bytes, size: size}
		defer func() { _ = reader.Close() }()
		http.ServeContent(w, r, "audio"+source.Format.Extension(), time.Time{}, reader)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	baseURL := "http://" + listener.Addr().String() + path
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		_ = server.Close()
		return nil, fmt.Errorf("build local stream URL: %w", err)
	}
	return &sourceServer{URL: baseURL, server: server, listen: listener}, nil
}

func (s *sourceServer) Close() error {
	var err error
	s.once.Do(func() { err = s.server.Close() })
	return err
}

type sourceReadSeeker struct {
	ctx    context.Context
	source provider.ByteSource
	size   int64
	pos    int64
	body   io.ReadCloser
}

func (s *sourceReadSeeker) Read(p []byte) (int, error) {
	if s.pos >= s.size {
		return 0, io.EOF
	}
	if s.body == nil {
		body, err := s.source.OpenRange(s.ctx, s.pos)
		if err != nil {
			return 0, err
		}
		s.body = body
	}
	if int64(len(p)) > s.size-s.pos {
		p = p[:s.size-s.pos]
	}
	n, err := s.body.Read(p)
	s.pos += int64(n)
	return n, err
}

func (s *sourceReadSeeker) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = s.pos + offset
	case io.SeekEnd:
		next = s.size + offset
	default:
		return s.pos, errors.New("invalid seek origin")
	}
	if next < 0 {
		return s.pos, errors.New("negative seek position")
	}
	if s.body != nil {
		_ = s.body.Close()
		s.body = nil
	}
	s.pos = next
	return next, nil
}

func (s *sourceReadSeeker) Close() error {
	if s.body == nil {
		return nil
	}
	err := s.body.Close()
	s.body = nil
	return err
}
