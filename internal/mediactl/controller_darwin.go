//go:build darwin

package mediactl

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sfate/amp-tui/internal/playback"
)

type controller struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	done   chan error
	events chan playback.MediaCommand
}

func newController() Controller {
	return &controller{events: make(chan playback.MediaCommand, 16)}
}

func (c *controller) MediaCommands() <-chan playback.MediaCommand {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.ensureStartedLocked()
	return c.events
}

func (c *controller) UpdateMediaState(state playback.MediaState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ensureStartedLocked() != nil {
		return
	}
	_ = c.sendLocked(strings.Join([]string{
		"nowplaying", sanitize(state.TrackID), sanitize(state.Title), sanitize(state.Artist),
		strconv.FormatUint(state.DurationMS, 10), strconv.FormatUint(state.PositionMS, 10), sanitize(state.AlbumArtURL),
	}, "\t"))
	stateName := "stopped"
	if state.Playing {
		stateName = "playing"
	} else if !state.Stopped {
		stateName = "paused"
	}
	_ = c.sendLocked("state\t" + stateName)
	_ = c.sendLocked(fmt.Sprintf("position\t%d\t%d", state.PositionMS, state.DurationMS))
	_ = c.sendLocked(fmt.Sprintf("capabilities\t%s\t%s\t%s", boolField(state.CanNext), boolField(state.CanPrevious), boolField(state.CanSeek)))
}

func (c *controller) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin != nil {
		_ = c.sendLocked("quit")
		_ = c.stdin.Close()
		c.stdin = nil
	}
	if c.cmd != nil && c.cmd.Process != nil {
		select {
		case <-c.done:
		case <-time.After(2 * time.Second):
			_ = c.cmd.Process.Kill()
			<-c.done
		}
		c.cmd = nil
		c.done = nil
	}
}

func (c *controller) ensureStartedLocked() error {
	if c.stdin != nil && c.cmd != nil && c.cmd.Process != nil {
		return nil
	}
	helperPath, err := EnsureHelper()
	if err != nil {
		return err
	}
	cmd := exec.Command(helperPath, "--control-only")
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd, c.stdin = cmd, stdin
	c.done = make(chan error, 1)
	done := c.done
	go c.readLoop(stdout)
	go func() {
		done <- cmd.Wait()
		c.mu.Lock()
		if c.cmd == cmd {
			c.cmd = nil
			c.stdin = nil
			c.done = nil
		}
		c.mu.Unlock()
	}()
	return nil
}

func (c *controller) sendLocked(line string) error {
	if c.stdin == nil {
		return errors.New("media helper stdin is unavailable")
	}
	_, err := io.WriteString(c.stdin, line+"\n")
	return err
}

func (c *controller) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "\t")
		if len(parts) < 2 || parts[0] != "remote" {
			continue
		}
		command, ok := parseRemote(parts)
		if !ok {
			continue
		}
		select {
		case c.events <- command:
		default:
		}
	}
}

func parseRemote(parts []string) (playback.MediaCommand, bool) {
	if len(parts) < 2 {
		return playback.MediaCommand{}, false
	}
	command := playback.MediaCommand{}
	switch parts[1] {
	case "play":
		command.Kind = playback.MediaPlay
	case "pause":
		command.Kind = playback.MediaPause
	case "toggle":
		command.Kind = playback.MediaToggle
	case "next":
		command.Kind = playback.MediaNext
	case "previous":
		command.Kind = playback.MediaPrevious
	case "setPosition":
		if len(parts) < 3 {
			return playback.MediaCommand{}, false
		}
		command.Kind = playback.MediaSetPosition
		position, err := strconv.ParseUint(parts[2], 10, 64)
		if err != nil {
			return playback.MediaCommand{}, false
		}
		command.PositionMS = position
	default:
		return playback.MediaCommand{}, false
	}
	return command, true
}

func sanitize(value string) string {
	return strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(value)
}

func boolField(value bool) string {
	if value {
		return "1"
	}
	return "0"
}
