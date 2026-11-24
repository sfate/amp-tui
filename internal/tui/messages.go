package tui

import (
	"github.com/sfate/amp-tui/internal/app"
	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/player"
	"github.com/sfate/amp-tui/internal/provider"
)

// source is the provider that answered; a message from a replaced provider is dropped.
type bootstrapFailedMsg struct {
	source provider.ID
	err    error
}

type bootstrapLoadedMsg struct {
	source    provider.ID
	playlists []app.Playlist
}

type collectionLoadedMsg struct {
	source   provider.ID
	id       string
	title    string
	tracks   []app.Track
	isFlow   bool
	append   bool
	autoplay bool
}

type loadFailedMsg struct {
	message string
}

type searchLoadedMsg struct {
	requestID int
	query     string
	tracks    []app.Track
	playlists []app.Playlist
	artists   []app.Artist
}

type searchFailedMsg struct {
	requestID int
	message   string
}

type playbackStartedMsg struct {
	playID  int
	session PlaybackSession
}

type playbackStartFailedMsg struct {
	playID  int
	trackID string
	quality core.Quality
	err     error
}

type playbackTrackChangedMsg struct {
	playID    int
	info      core.TrackInfo
	quality   core.Quality
	initialMS uint64
}

type bufferingProgressMsg struct {
	playID  int
	percent uint8
	stage   player.BufferingStage
}

type playbackProgressMsg struct {
	playID    int
	currentMS uint64
	totalMS   uint64
}

type playbackVisualizerMsg struct {
	playID int
	bands  []uint8
}

type playbackErrorMsg struct {
	playID int
	err    error
}

type playbackFinishedMsg struct {
	playID int
	err    error
}

type playbackTickMsg struct{}

type prebufferStatusMsg struct {
	trackID string
	quality core.Quality
	status  PrebufferStatus
}

type prebufferTickMsg struct{}

type scheduledPlaybackMsg struct {
	requestID int
}

type loginSucceededMsg struct {
	secret   string
	provider provider.Provider
}

type loginFailedMsg struct {
	message string
}

type artworkLoadedMsg struct {
	url string
	art string
}

type loadingTickMsg struct{}

type mediaControlCommandMsg struct {
	command MediaControlCommand
}
