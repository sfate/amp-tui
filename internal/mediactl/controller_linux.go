//go:build linux

package mediactl

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
	"github.com/sfate/amp-tui/internal/playback"
)

const (
	mprisBusName  = "org.mpris.MediaPlayer2.amp_tui"
	mprisPath     = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	mprisRoot     = "org.mpris.MediaPlayer2"
	mprisPlayerIF = "org.mpris.MediaPlayer2.Player"
)

type mprisController struct {
	mu     sync.RWMutex
	conn   *dbus.Conn
	props  *prop.Properties
	state  playback.MediaState
	events chan playback.MediaCommand
}

func newController() Controller {
	return &mprisController{events: make(chan playback.MediaCommand, 32)}
}

func (c *mprisController) MediaCommands() <-chan playback.MediaCommand {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.ensureStartedLocked()
	return c.events
}

func (c *mprisController) UpdateMediaState(state playback.MediaState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureStartedLocked(); err != nil {
		return
	}
	old := c.state
	c.state = state
	if old.Playing != state.Playing || old.Stopped != state.Stopped {
		c.props.SetMust(mprisPlayerIF, "PlaybackStatus", playbackStatus(state))
	}
	if old.TrackID != state.TrackID || old.Title != state.Title || old.Artist != state.Artist || old.DurationMS != state.DurationMS || old.AlbumArtURL != state.AlbumArtURL {
		c.props.SetMust(mprisPlayerIF, "Metadata", metadataForState(state))
	}
	if old.Volume != state.Volume {
		c.props.SetMust(mprisPlayerIF, "Volume", float64(state.Volume)/100)
	}
	if old.PositionMS != state.PositionMS {
		c.props.SetMust(mprisPlayerIF, "Position", millisecondsToMicroseconds(state.PositionMS))
	}
	if old.CanNext != state.CanNext {
		c.props.SetMust(mprisPlayerIF, "CanGoNext", state.CanNext)
	}
	if old.CanPrevious != state.CanPrevious {
		c.props.SetMust(mprisPlayerIF, "CanGoPrevious", state.CanPrevious)
	}
	if old.CanSeek != state.CanSeek {
		c.props.SetMust(mprisPlayerIF, "CanSeek", state.CanSeek)
	}
	if old.LoopStatus != state.LoopStatus {
		c.props.SetMust(mprisPlayerIF, "LoopStatus", state.LoopStatus)
	}
}

func (c *mprisController) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
		c.props = nil
	}
}

func (c *mprisController) ensureStartedLocked() error {
	if c.conn != nil {
		if c.conn.Connected() {
			return nil
		}
		_ = c.conn.Close()
		c.conn = nil
		c.props = nil
	}
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("connect to the session D-Bus: %w", err)
	}
	reply, err := conn.RequestName(mprisBusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("request MPRIS bus name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner && reply != dbus.RequestNameReplyAlreadyOwner {
		_ = conn.Close()
		return errors.New("another amp-tui process already owns the MPRIS bus name")
	}
	controller := mprisPlayer{controller: c}
	if err := conn.Export(&controller, mprisPath, mprisPlayerIF); err != nil {
		_ = conn.Close()
		return fmt.Errorf("export MPRIS player methods: %w", err)
	}
	root := mprisRootObject{}
	if err := conn.Export(&root, mprisPath, mprisRoot); err != nil {
		_ = conn.Close()
		return fmt.Errorf("export MPRIS root methods: %w", err)
	}
	properties, err := prop.Export(conn, mprisPath, mprisProperties(c))
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("export MPRIS properties: %w", err)
	}
	introspection := introspect.NewIntrospectable(&introspect.Node{Interfaces: []introspect.Interface{
		{Name: mprisRoot, Methods: introspect.Methods(&root), Properties: properties.Introspection(mprisRoot)},
		{Name: mprisPlayerIF, Methods: introspect.Methods(&controller), Signals: []introspect.Signal{{Name: "Seeked", Args: []introspect.Arg{{Name: "Position", Type: "x"}}}}, Properties: properties.Introspection(mprisPlayerIF)},
	}})
	if err := conn.Export(introspection, mprisPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		_ = conn.Close()
		return fmt.Errorf("export MPRIS introspection: %w", err)
	}
	c.conn, c.props = conn, properties
	c.props.SetMust(mprisPlayerIF, "Metadata", metadataForState(c.state))
	return nil
}

func mprisProperties(c *mprisController) prop.Map {
	return prop.Map{
		mprisRoot: {
			"CanQuit":             {Value: false, Emit: prop.EmitConst},
			"CanRaise":            {Value: false, Emit: prop.EmitConst},
			"HasTrackList":        {Value: false, Emit: prop.EmitConst},
			"Identity":            {Value: "amp-tui", Emit: prop.EmitConst},
			"DesktopEntry":        {Value: "amp-tui", Emit: prop.EmitConst},
			"SupportedUriSchemes": {Value: []string{}, Emit: prop.EmitConst},
			"SupportedMimeTypes":  {Value: []string{}, Emit: prop.EmitConst},
		},
		mprisPlayerIF: {
			"PlaybackStatus": {Value: "Stopped", Emit: prop.EmitTrue},
			"LoopStatus":     {Value: "None", Writable: true, Emit: prop.EmitTrue, Callback: c.loopStatusCallback()},
			"Rate":           {Value: 1.0, Writable: true, Emit: prop.EmitTrue, Callback: fixedRateCallback},
			"Shuffle":        {Value: false, Writable: true, Emit: prop.EmitTrue, Callback: noShuffleCallback},
			"Metadata":       {Value: map[string]dbus.Variant{}, Emit: prop.EmitTrue},
			"Volume":         {Value: 0.0, Writable: true, Emit: prop.EmitTrue, Callback: c.volumeCallback()},
			"Position":       {Value: int64(0), Emit: prop.EmitFalse},
			"MinimumRate":    {Value: 1.0, Emit: prop.EmitConst},
			"MaximumRate":    {Value: 1.0, Emit: prop.EmitConst},
			"CanGoNext":      {Value: false, Emit: prop.EmitTrue},
			"CanGoPrevious":  {Value: false, Emit: prop.EmitTrue},
			"CanPlay":        {Value: true, Emit: prop.EmitConst},
			"CanPause":       {Value: true, Emit: prop.EmitConst},
			"CanSeek":        {Value: false, Emit: prop.EmitTrue},
			"CanControl":     {Value: true, Emit: prop.EmitConst},
		},
	}
}

func (c *mprisController) volumeCallback() func(*prop.Change) *dbus.Error {
	return func(change *prop.Change) *dbus.Error {
		volume, ok := change.Value.(float64)
		if !ok {
			return prop.ErrInvalidArg
		}
		volume = math.Max(0, math.Min(1, volume))
		if !c.publish(playback.MediaCommand{Kind: playback.MediaSetVolume, Volume: float32(volume)}) {
			return dbus.MakeFailedError(errors.New("amp-tui media command queue is full"))
		}
		return nil
	}
}

func (c *mprisController) loopStatusCallback() func(*prop.Change) *dbus.Error {
	return func(change *prop.Change) *dbus.Error {
		status, ok := change.Value.(string)
		if !ok || (status != "None" && status != "Track" && status != "Playlist") {
			return prop.ErrInvalidArg
		}
		if !c.publish(playback.MediaCommand{Kind: playback.MediaSetLoopStatus, LoopStatus: status}) {
			return dbus.MakeFailedError(errors.New("amp-tui media command queue is full"))
		}
		return nil
	}
}

func fixedRateCallback(change *prop.Change) *dbus.Error {
	rate, ok := change.Value.(float64)
	if !ok {
		return prop.ErrInvalidArg
	}
	if rate != 1 {
		return dbus.NewError("org.mpris.MediaPlayer2.Player.Error.NotSupported", nil)
	}
	return nil
}

func noShuffleCallback(change *prop.Change) *dbus.Error {
	shuffle, ok := change.Value.(bool)
	if !ok {
		return prop.ErrInvalidArg
	}
	if shuffle {
		return dbus.NewError("org.mpris.MediaPlayer2.Player.Error.NotSupported", nil)
	}
	return nil
}

func (c *mprisController) publish(command playback.MediaCommand) bool {
	select {
	case c.events <- command:
		return true
	default:
		return false
	}
}

func (c *mprisController) currentState() playback.MediaState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *mprisController) emitSeeked(positionMS uint64) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn != nil {
		_ = conn.Emit(mprisPath, mprisPlayerIF+".Seeked", millisecondsToMicroseconds(positionMS))
	}
}

type mprisPlayer struct{ controller *mprisController }

func (p *mprisPlayer) Play() *dbus.Error {
	return p.publish(playback.MediaCommand{Kind: playback.MediaPlay})
}
func (p *mprisPlayer) Pause() *dbus.Error {
	return p.publish(playback.MediaCommand{Kind: playback.MediaPause})
}
func (p *mprisPlayer) PlayPause() *dbus.Error {
	return p.publish(playback.MediaCommand{Kind: playback.MediaToggle})
}
func (p *mprisPlayer) Stop() *dbus.Error {
	return p.publish(playback.MediaCommand{Kind: playback.MediaStop})
}
func (p *mprisPlayer) Next() *dbus.Error {
	return p.publish(playback.MediaCommand{Kind: playback.MediaNext})
}
func (p *mprisPlayer) Previous() *dbus.Error {
	return p.publish(playback.MediaCommand{Kind: playback.MediaPrevious})
}

func (*mprisPlayer) OpenUri(string) *dbus.Error {
	return dbus.NewError("org.mpris.MediaPlayer2.Player.Error.NotSupported", nil)
}

//nolint:govet // MPRIS defines Seek(offset in microseconds) with a D-Bus error result.
func (p *mprisPlayer) Seek(offset int64) *dbus.Error {
	position := int64(millisecondsToMicroseconds(p.controller.currentState().PositionMS))
	if offset > 0 && position > math.MaxInt64-offset {
		position = math.MaxInt64
	} else {
		position += offset
	}
	if position < 0 {
		position = 0
	}
	positionMS := uint64(position / 1000)
	if err := p.publish(playback.MediaCommand{Kind: playback.MediaSetPosition, PositionMS: positionMS}); err != nil {
		return err
	}
	p.controller.emitSeeked(positionMS)
	return nil
}

func (p *mprisPlayer) SetPosition(trackID dbus.ObjectPath, position int64) *dbus.Error {
	state := p.controller.currentState()
	if state.TrackID == "" || trackID != trackPath(state.TrackID) {
		return nil
	}
	if position < 0 {
		position = 0
	}
	positionMS := uint64(position / 1000)
	if err := p.publish(playback.MediaCommand{Kind: playback.MediaSetPosition, PositionMS: positionMS}); err != nil {
		return err
	}
	p.controller.emitSeeked(positionMS)
	return nil
}

func (p *mprisPlayer) publish(command playback.MediaCommand) *dbus.Error {
	if !p.controller.publish(command) {
		return dbus.MakeFailedError(errors.New("amp-tui media command queue is full"))
	}
	return nil
}

type mprisRootObject struct{}

func (*mprisRootObject) Raise() *dbus.Error { return nil }
func (*mprisRootObject) Quit() *dbus.Error  { return nil }

func playbackStatus(state playback.MediaState) string {
	if state.Stopped {
		return "Stopped"
	}
	if state.Playing {
		return "Playing"
	}
	return "Paused"
}

func metadataForState(state playback.MediaState) map[string]dbus.Variant {
	if state.TrackID == "" {
		return map[string]dbus.Variant{}
	}
	metadata := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(trackPath(state.TrackID)),
		"xesam:title":   dbus.MakeVariant(state.Title),
	}
	if state.DurationMS > 0 {
		metadata["mpris:length"] = dbus.MakeVariant(millisecondsToMicroseconds(state.DurationMS))
	}
	if state.Artist != "" {
		metadata["xesam:artist"] = dbus.MakeVariant([]string{state.Artist})
	}
	if state.AlbumArtURL != "" {
		metadata["mpris:artUrl"] = dbus.MakeVariant(state.AlbumArtURL)
	}
	return metadata
}

func trackPath(trackID string) dbus.ObjectPath {
	digest := sha256.Sum256([]byte(trackID))
	return dbus.ObjectPath("/org/amp_tui/Track/" + hex.EncodeToString(digest[:16]))
}

func millisecondsToMicroseconds(milliseconds uint64) int64 {
	if milliseconds > math.MaxInt64/1000 {
		return math.MaxInt64
	}
	return int64(milliseconds * 1000)
}
