package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestIPCClientMatchesCommandRepliesAndForwardsEvents(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := newIPCClient(clientConn)
	defer client.close()
	go func() {
		defer func() { _ = serverConn.Close() }()
		scanner := bufio.NewScanner(serverConn)
		if !scanner.Scan() {
			return
		}
		var request struct {
			RequestID int64 `json:"request_id"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			return
		}
		_, _ = fmt.Fprintf(serverConn, `{"request_id":%d,"error":"success"}`+"\n", request.RequestID)
		_, _ = serverConn.Write([]byte(`{"event":"playback-restart"}` + "\n"))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.command(ctx, "set_property", "pause", false); err != nil {
		t.Fatalf("IPC command: %v", err)
	}
	select {
	case event := <-client.events:
		if event.Event != "playback-restart" {
			t.Fatalf("event = %q, want playback-restart", event.Event)
		}
	case <-ctx.Done():
		t.Fatal("IPC event did not arrive")
	}
}
