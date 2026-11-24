package tui

import (
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/playback"
)

func startPlaybackCmdWithEvents(playID int, trackID string, runtime *playerRuntime, quality core.Quality, seekMS uint64, enableVisualizer bool, events chan tea.Msg) tea.Cmd {
	if runtime == nil {
		return nil
	}
	return func() tea.Msg {
		var visualizerMu sync.Mutex
		var lastVisualizerSent time.Time
		handler := playback.EventHandler{
			OnTrackChanged: func(info core.TrackInfo, q core.Quality, initialMS uint64) {
				events <- playbackTrackChangedMsg{playID: playID, info: info, quality: q, initialMS: initialMS}
			},
			OnBufferingProgress: func(percent uint8, stage playback.BufferingStage) {
				events <- bufferingProgressMsg{playID: playID, percent: percent, stage: stage}
			},
			OnPlaybackProgress: func(currentMS, totalMS uint64) {
				events <- playbackProgressMsg{playID: playID, currentMS: currentMS, totalMS: totalMS}
			},
			OnError: func(err error) {
				events <- playbackErrorMsg{playID: playID, err: err}
			},
		}
		if enableVisualizer {
			handler.OnAudioBands = func(bands []uint8) {
				visualizerMu.Lock()
				if !lastVisualizerSent.IsZero() && time.Since(lastVisualizerSent) < visualizerUIUpdateMinInterval {
					visualizerMu.Unlock()
					return
				}
				lastVisualizerSent = time.Now()
				visualizerMu.Unlock()

				copied := append([]uint8(nil), bands...)
				select {
				case events <- playbackVisualizerMsg{playID: playID, bands: copied}:
				default:
				}
			}
		}
		session, err := runtime.start(trackID, quality, seekMS, handler)
		if err != nil {
			return playbackStartFailedMsg{playID: playID, trackID: trackID, quality: quality, err: err}
		}
		return playbackStartedMsg{playID: playID, session: session}
	}
}

func waitPlaybackCmd(playID int, session playback.Session) tea.Cmd {
	if session == nil {
		return nil
	}
	return func() tea.Msg {
		return playbackFinishedMsg{playID: playID, err: session.Wait()}
	}
}
