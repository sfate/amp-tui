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
