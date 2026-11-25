package enginefactory

import (
	"runtime"

	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/playback/mpv"
	"github.com/sfate/amp-tui/internal/playback/native"
)

func Select(setting config.PlaybackEngine) playback.Engine {
	return selectEngine(setting, runtime.GOOS == "darwin", mpv.Available, func() playback.Engine { return mpv.New() }, func() playback.Engine { return native.New() })
}

func selectEngine(setting config.PlaybackEngine, preferNative bool, available func() bool, newMPV, newNative func() playback.Engine) playback.Engine {
	switch config.NormalizeEngine(setting) {
	case config.EngineMPV:
		return newMPV()
	case config.EngineNative:
		return newNative()
	default:
		if preferNative {
			return newNative()
		}
		if available() {
			return newMPV()
		}
		return newNative()
	}
}
