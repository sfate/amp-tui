package playback

type PrebufferStatus int

const (
	PrebufferLoading PrebufferStatus = iota
	PrebufferReady
	PrebufferFailed
)

// Prebufferer is an Engine that fetches upcoming tracks before Start asks for them.
type Prebufferer interface {
	// Prebuffer replaces the window; an empty window cancels pending work and keeps cached tracks.
	Prebuffer(tracks []Track, onStatus func(Track, PrebufferStatus))
	// ClearPrebuffer drops every cached track, because track IDs belong to one provider.
	ClearPrebuffer()
}

type MediaCommandKind int

const (
	MediaPlay MediaCommandKind = iota
	MediaPause
	MediaToggle
	MediaNext
	MediaPrevious
	MediaSetPosition
)

type MediaCommand struct {
	Kind       MediaCommandKind
	PositionMS uint64
}

type MediaState struct {
	Playing     bool
	Stopped     bool
	TrackID     string
	Title       string
	Artist      string
	AlbumArtURL string
	PositionMS  uint64
	DurationMS  uint64
	Volume      uint16
	CanNext     bool
	CanPrevious bool
	CanSeek     bool
}

// MediaController is an Engine that also drives the OS Now Playing widget and media keys.
type MediaController interface {
	MediaCommands() <-chan MediaCommand
	UpdateMediaState(MediaState)
}
