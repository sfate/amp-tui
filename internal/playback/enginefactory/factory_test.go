package enginefactory

import (
	"context"
	"testing"

	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
)

type namedEngine string

func (e namedEngine) Name() string                    { return string(e) }
func (namedEngine) Capabilities() playback.EngineCaps { return playback.EngineCaps{} }
func (namedEngine) Start(context.Context, provider.Source, playback.StartOptions, playback.EventHandler) (playback.Session, error) {
	return nil, nil
}

func TestSelectEngineHonorsModeAndAutoFallback(t *testing.T) {
	newMPV := func() playback.Engine { return namedEngine("mpv") }
	newNative := func() playback.Engine { return namedEngine("native") }
	for _, tc := range []struct {
		name         string
		setting      config.PlaybackEngine
		requiresMPV  bool
		preferNative bool
		available    bool
		want         string
	}{{"auto prefers mpv", config.EngineAuto, false, false, true, "mpv"}, {"auto falls back", config.EngineAuto, false, false, false, "native"}, {"auto prefers native on macOS", config.EngineAuto, false, true, true, "native"}, {"auto selects mpv for YouTube", config.EngineAuto, true, true, false, "mpv"}, {"explicit mpv", config.EngineMPV, false, true, false, "mpv"}, {"explicit native", config.EngineNative, false, false, true, "native"}} {
		t.Run(tc.name, func(t *testing.T) {
			got := selectEngine(tc.setting, tc.requiresMPV, tc.preferNative, func() bool { return tc.available }, newMPV, newNative)
			if got.Name() != tc.want {
				t.Fatalf("selected %q, want %q", got.Name(), tc.want)
			}
		})
	}
}
