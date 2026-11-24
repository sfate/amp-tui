// Package core holds the catalog types shared by providers and the TUI.
package core

type Track struct {
	ID        string
	Title     string
	Artist    string
	Album     string
	Year      string
	AddedAtMS *int64
}

// QueueLabel is the text of the track in the queue panel.
func (t Track) QueueLabel() string {
	return t.Title + " - " + t.Artist
}

type Playlist struct {
	ID    string
	Title string
}

type Artist struct {
	ID   string
	Name string
}

// Quality is one step of a provider's quality ladder; ID is stored in config.
type Quality struct {
	ID    string
	Label string
	// Seekable is false when playback at this quality must start at 0.
	Seekable bool
}

type AudioFormat string

const (
	FormatMP3  AudioFormat = "mp3"
	FormatFLAC AudioFormat = "flac"
	FormatOpus AudioFormat = "opus"
	FormatAAC  AudioFormat = "aac"
)

// Extension is the file name suffix that local players use to pick a decoder.
func (f AudioFormat) Extension() string {
	if f == "" {
		return ""
	}
	return "." + string(f)
}

// TrackInfo describes the track being played; DurationMS is 0 when unknown.
type TrackInfo struct {
	ID          string
	Title       string
	Artist      string
	Album       string
	AlbumArtURL *string
	DurationMS  uint64
}
