//go:build darwin || linux

package mpv

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/playback/playbacktest"
	"github.com/sfate/amp-tui/internal/provider"
)

func TestMPVEngineMeetsContract(t *testing.T) {
	playbacktest.RunEngineContract(t, func(t *testing.T) playbacktest.Harness {
		fake := newFakeMPV(t, false)
		return playbacktest.Harness{Engine: newWithBinary(fake.binary), EndTrack: fake.endTrack}
	})
}

func TestMPVRestartsAfterSocketEOFAtLastPosition(t *testing.T) {
	fake := newFakeMPV(t, true)
	engine := newWithBinary(fake.binary)
	changed := make(chan uint64, 1)
	source := provider.Source{
		Info:     core.TrackInfo{ID: "restart-track", DurationMS: 60_000},
		Quality:  core.Quality{ID: "restart", Seekable: true},
		Format:   core.FormatMP3,
		Bytes:    contractBytes("audio"),
		Seekable: true,
	}
	session, err := engine.Start(context.Background(), source, playback.StartOptions{}, playback.EventHandler{
		OnTrackChanged: func(_ core.TrackInfo, _ core.Quality, initial uint64) { changed <- initial },
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(session.Stop)
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("first mpv process did not start playback")
	}
	fake.waitProcessCount(t, 2)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got, err := os.ReadFile(fake.state + ".start"); err == nil && string(got) == "start=2.000" {
			fake.endTrack(t)
			if err := session.Wait(); err != nil {
				t.Fatalf("Wait after EOF: %v", err)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("restarted mpv did not resume at the last time-pos")
}

type contractBytes []byte

func (b contractBytes) Size() int64 { return int64(len(b)) }
func (b contractBytes) OpenRange(_ context.Context, offset int64) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(b[offset:])), nil
}

// TestMPVHelperProcess is a subprocess fixture that speaks enough JSON IPC to exercise sessions.
func TestMPVHelperProcess(t *testing.T) {
	if os.Getenv("AMP_TUI_MPV_HELPER") != "1" {
		return
	}
	statePath := os.Getenv("AMP_TUI_MPV_STATE")
	args := argsAfterSeparator(os.Args)
	socket := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "--input-ipc-server=") {
			socket = strings.TrimPrefix(arg, "--input-ipc-server=")
		}
	}
	if socket == "" {
		os.Exit(3)
	}
	if err := os.WriteFile(statePath, []byte(socket), 0o600); err != nil {
		os.Exit(4)
	}
	countPath := statePath + ".count"
	countData, _ := os.ReadFile(countPath)
	count, _ := strconv.Atoi(string(countData))
	count++
	if err := os.WriteFile(countPath, []byte(strconv.Itoa(count)), 0o600); err != nil {
		os.Exit(4)
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		os.Exit(5)
	}
	defer func() { _ = listener.Close() }()
	conn, err := listener.Accept()
	if err != nil {
		os.Exit(6)
	}
	defer func() { _ = conn.Close() }()
	var writeMu sync.Mutex
	write := func(value any) {
		data, _ := json.Marshal(value)
		writeMu.Lock()
		_, _ = conn.Write(append(data, '\n'))
		writeMu.Unlock()
	}
	go func() {
		for {
			if _, err := os.Stat(filepath.Join(filepath.Dir(socket), "end")); err == nil {
				write(map[string]any{"event": "end-file", "reason": "eof"})
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var message struct {
			RequestID int64             `json:"request_id"`
			Command   []json.RawMessage `json:"command"`
		}
		if json.Unmarshal(scanner.Bytes(), &message) != nil || len(message.Command) == 0 {
			continue
		}
		var command string
		_ = json.Unmarshal(message.Command[0], &command)
		write(map[string]any{"request_id": message.RequestID, "error": "success"})
		switch command {
		case "loadfile":
			write(map[string]any{"event": "playback-restart"})
			write(map[string]any{"event": "property-change", "name": "time-pos", "data": 2.0})
			if os.Getenv("AMP_TUI_MPV_RESTART") == "1" {
				var start string
				if len(message.Command) > 3 {
					_ = json.Unmarshal(message.Command[3], &start)
				}
				_ = os.WriteFile(statePath+".start", []byte(start), 0o600)
				if count == 1 {
					time.Sleep(10 * time.Millisecond)
					return
				}
			}
		case "seek":
			var seconds string
			if len(message.Command) > 1 {
				_ = json.Unmarshal(message.Command[1], &seconds)
			}
			position, _ := strconv.ParseFloat(seconds, 64)
			write(map[string]any{"event": "property-change", "name": "time-pos", "data": position})
		case "quit":
			return
		}
	}
}

type fakeMPV struct {
	binary string
	state  string
}

func newFakeMPV(t *testing.T, restart bool) fakeMPV {
	t.Helper()
	directory := t.TempDir()
	state := filepath.Join(directory, "socket-path")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(directory, "fake-mpv")
	restartEnv := ""
	if restart {
		restartEnv = "AMP_TUI_MPV_RESTART=1 "
	}
	script := fmt.Sprintf("#!/bin/sh\nAMP_TUI_MPV_HELPER=1 AMP_TUI_MPV_STATE=%s %sexec %s -test.run=^TestMPVHelperProcess$ -- \"$@\"\n", shellQuote(state), restartEnv, shellQuote(executable))
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return fakeMPV{binary: binary, state: state}
}

func (f fakeMPV) waitProcessCount(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(f.state + ".count")
		if err == nil {
			count, _ := strconv.Atoi(string(data))
			if count >= want {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("fake mpv process count did not reach %d", want)
}

func (f fakeMPV) endTrack(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(f.state)
		if err == nil {
			if err := os.WriteFile(filepath.Join(filepath.Dir(string(data)), "end"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("fake mpv did not start")
}

func argsAfterSeparator(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:]
		}
	}
	return nil
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
