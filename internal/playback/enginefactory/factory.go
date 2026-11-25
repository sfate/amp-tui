package enginefactory

import (
	"runtime"

	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/playback/mpv"
	"github.com/sfate/amp-tui/internal/playback/native"
	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/provider/youtube"
)

func Select(setting config.PlaybackEngine, providerID provider.ID) playback.Engine {
	return selectEngine(setting, providerID == youtube.ID, runtime.GOOS == "darwin", mpv.Available, func() playback.Engine { return mpv.New() }, func() playback.Engine { return native.New() })
}

func selectEngine(setting config.PlaybackEngine, requiresMPV, preferNative bool, available func() bool, newMPV, newNative func() playback.Engine) playback.Engine {
	switch config.NormalizeEngine(setting) {
	case config.EngineMPV:
		return newMPV()
	case config.EngineNative:
		return newNative()
	default:
		if requiresMPV {
			return newMPV()
		}
		if preferNative {
			return newNative()
		}
		if available() {
			return newMPV()
		}
		return newNative()
	}
}
