//go:build linux

package mediactl

import (
	"bufio"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
	"github.com/sfate/amp-tui/internal/playback"
)

func TestMPRISPlayerMethodsPublishMediaCommands(t *testing.T) {
	controller := &mprisController{events: make(chan playback.MediaCommand, 16)}
	player := mprisPlayer{controller: controller}
	for _, call := range []func() *dbus.Error{player.Play, player.Pause, player.PlayPause, player.Stop, player.Next, player.Previous} {
		if err := call(); err != nil {
			t.Fatalf("MPRIS method: %v", err)
		}
	}

	want := []playback.MediaCommandKind{
		playback.MediaPlay, playback.MediaPause, playback.MediaToggle,
		playback.MediaStop, playback.MediaNext, playback.MediaPrevious,
	}
	for _, kind := range want {
		if got := <-controller.events; got.Kind != kind {
			t.Errorf("command = %+v, want kind %d", got, kind)
		}
	}
}

func TestMPRISSeekAndSetPositionUseMilliseconds(t *testing.T) {
	controller := &mprisController{
		events: make(chan playback.MediaCommand, 4),
		state:  playback.MediaState{TrackID: "track-1", PositionMS: 2_000},
	}
	player := mprisPlayer{controller: controller}
	if err := player.Seek(-500_000); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if got := <-controller.events; got != (playback.MediaCommand{Kind: playback.MediaSetPosition, PositionMS: 1_500}) {
		t.Fatalf("Seek command = %+v", got)
	}

	if err := player.SetPosition(trackPath("track-1"), 9_750_000); err != nil {
		t.Fatalf("SetPosition: %v", err)
	}
	if got := <-controller.events; got != (playback.MediaCommand{Kind: playback.MediaSetPosition, PositionMS: 9_750}) {
		t.Fatalf("SetPosition command = %+v", got)
	}
	if err := player.SetPosition(trackPath("different-track"), 10_000_000); err != nil {
		t.Fatalf("SetPosition for stale track: %v", err)
	}
	select {
	case got := <-controller.events:
		t.Fatalf("stale SetPosition sent command %+v", got)
	default:
	}
}

func TestMPRISVolumeCallbackPublishesClampedValue(t *testing.T) {
	controller := &mprisController{events: make(chan playback.MediaCommand, 1)}
	callback := controller.volumeCallback()
	if err := callback(&prop.Change{Value: 1.7}); err != nil {
		t.Fatalf("volume callback: %v", err)
	}
	if got := <-controller.events; got.Kind != playback.MediaSetVolume || got.Volume != 1 {
		t.Fatalf("volume command = %+v", got)
	}
}

func TestMPRISExportsPlayerctlCommandsAndProperties(t *testing.T) {
	startPrivateSessionBus(t)
	controller := newController().(*mprisController)
	defer controller.Close()
	controller.UpdateMediaState(playback.MediaState{
		Playing: true, TrackID: "track-1", Title: "Song", Artist: "Artist",
		PositionMS: 2_000, DurationMS: 90_000, Volume: 55, CanSeek: true, CanNext: true, CanPrevious: true,
	})

	client, err := dbus.SessionBus()
	if err != nil {
		t.Fatalf("connect D-Bus client: %v", err)
	}
	defer func() { _ = client.Close() }()
	object := client.Object(mprisBusName, mprisPath)
	var status dbus.Variant
	if err := object.Call("org.freedesktop.DBus.Properties.Get", 0, mprisPlayerIF, "PlaybackStatus").Store(&status); err != nil {
		t.Fatalf("read PlaybackStatus: %v", err)
	}
	if got := status.Value(); got != "Playing" {
		t.Fatalf("PlaybackStatus = %v, want Playing", got)
	}
	var metadata dbus.Variant
	if err := object.Call("org.freedesktop.DBus.Properties.Get", 0, mprisPlayerIF, "Metadata").Store(&metadata); err != nil {
		t.Fatalf("read Metadata: %v", err)
	}
	trackMetadata := metadata.Value().(map[string]dbus.Variant)
	if got := trackMetadata["xesam:title"].Value(); got != "Song" {
		t.Fatalf("Metadata title = %v, want Song", got)
	}

	if err := object.Call(mprisPlayerIF+".PlayPause", 0).Err; err != nil {
		t.Fatalf("PlayPause: %v", err)
	}
	assertMPRISCommand(t, controller.events, playback.MediaCommand{Kind: playback.MediaToggle})
	if err := object.Call(mprisPlayerIF+".SetPosition", 0, trackPath("track-1"), int64(12_000_000)).Err; err != nil {
		t.Fatalf("SetPosition: %v", err)
	}
	assertMPRISCommand(t, controller.events, playback.MediaCommand{Kind: playback.MediaSetPosition, PositionMS: 12_000})
	if err := object.Call("org.freedesktop.DBus.Properties.Set", 0, mprisPlayerIF, "Volume", dbus.MakeVariant(0.75)).Err; err != nil {
		t.Fatalf("set Volume: %v", err)
	}
	assertMPRISCommand(t, controller.events, playback.MediaCommand{Kind: playback.MediaSetVolume, Volume: 0.75})
	if err := object.Call("org.freedesktop.DBus.Properties.Set", 0, mprisPlayerIF, "LoopStatus", dbus.MakeVariant("Playlist")).Err; err != nil {
		t.Fatalf("set LoopStatus: %v", err)
	}
	assertMPRISCommand(t, controller.events, playback.MediaCommand{Kind: playback.MediaSetLoopStatus, LoopStatus: "Playlist"})

	playerctl, err := exec.LookPath("playerctl")
	if err == nil {
		position, err := exec.Command(playerctl, "--player=amp_tui", "position").CombinedOutput()
		if err != nil || strings.TrimSpace(string(position)) == "" {
			t.Fatalf("playerctl position: %v: %s", err, position)
		}
		for _, test := range []struct {
			args []string
			want playback.MediaCommand
		}{
			{[]string{"play"}, playback.MediaCommand{Kind: playback.MediaPlay}},
			{[]string{"pause"}, playback.MediaCommand{Kind: playback.MediaPause}},
			{[]string{"play-pause"}, playback.MediaCommand{Kind: playback.MediaToggle}},
			{[]string{"next"}, playback.MediaCommand{Kind: playback.MediaNext}},
			{[]string{"previous"}, playback.MediaCommand{Kind: playback.MediaPrevious}},
			{[]string{"position", "12"}, playback.MediaCommand{Kind: playback.MediaSetPosition, PositionMS: 12_000}},
		} {
			args := append([]string{"--player=amp_tui"}, test.args...)
			if output, err := exec.Command(playerctl, args...).CombinedOutput(); err != nil {
				t.Fatalf("playerctl %v: %v: %s", test.args, err, output)
			}
			assertMPRISCommand(t, controller.events, test.want)
		}
	}
}

func startPrivateSessionBus(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("dbus-daemon is unavailable")
	}
	command := exec.Command("dbus-daemon", "--session", "--nofork", "--nopidfile", "--print-address=1")
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = nil
	if err := command.Start(); err != nil {
		t.Skipf("start private session bus: %v", err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("read private session bus address: %v", err)
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", line[:len(line)-1])
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	})
}

func assertMPRISCommand(t *testing.T, events <-chan playback.MediaCommand, want playback.MediaCommand) {
	t.Helper()
	select {
	case got := <-events:
		if got != want {
			t.Fatalf("MPRIS command = %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for MPRIS command %+v", want)
	}
}
