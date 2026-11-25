package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type ipcMessage struct {
	Event     string `json:"event"`
	Name      string `json:"name"`
	Data      any    `json:"data"`
	Reason    string `json:"reason"`
	FileError string `json:"file_error"`
	RequestID int64  `json:"request_id"`
	Error     string `json:"error"`
}

type ipcReply struct {
	err string
}

type ipcClient struct {
	conn   net.Conn
	events chan ipcMessage

	mu      sync.Mutex
	writes  sync.Mutex
	pending map[int64]chan ipcReply
	closed  bool
	nextID  atomic.Int64
}

func newIPCClient(conn net.Conn) *ipcClient {
	c := &ipcClient{conn: conn, events: make(chan ipcMessage, 128), pending: make(map[int64]chan ipcReply)}
	go c.readLoop()
	return c
}

func (c *ipcClient) command(ctx context.Context, args ...any) error {
	id := c.nextID.Add(1)
	reply := make(chan ipcReply, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("mpv IPC connection is closed")
	}
	c.pending[id] = reply
	c.mu.Unlock()
	message, err := json.Marshal(struct {
		Command   []any `json:"command"`
		RequestID int64 `json:"request_id"`
	}{Command: args, RequestID: id})
	if err != nil {
		c.removePending(id)
		return err
	}
	c.writes.Lock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = c.conn.Write(append(message, '\n'))
	_ = c.conn.SetWriteDeadline(time.Time{})
	c.writes.Unlock()
	if err != nil {
		c.removePending(id)
		return fmt.Errorf("write mpv IPC command: %w", err)
	}
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case response := <-reply:
		if response.err != "success" {
			return fmt.Errorf("mpv IPC command failed: %s", response.err)
		}
		return nil
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case <-timer.C:
		c.removePending(id)
		return errors.New("timed out waiting for mpv IPC response")
	}
}

func (c *ipcClient) removePending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *ipcClient) close() {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		_ = c.conn.Close()
	}
	c.mu.Unlock()
}

func (c *ipcClient) readLoop() {
	defer close(c.events)
	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		var message ipcMessage
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			continue
		}
		if message.Event != "" {
			select {
			case c.events <- message:
			default:
			}
			continue
		}
		if message.RequestID != 0 {
			c.mu.Lock()
			reply := c.pending[message.RequestID]
			delete(c.pending, message.RequestID)
			c.mu.Unlock()
			if reply != nil {
				select {
				case reply <- ipcReply{err: message.Error}:
				default:
				}
			}
		}
	}
	c.mu.Lock()
	c.closed = true
	for id, reply := range c.pending {
		delete(c.pending, id)
		select {
		case reply <- ipcReply{err: "IPC socket closed"}:
		default:
		}
	}
	c.mu.Unlock()
}
