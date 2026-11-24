package deezer

import "testing"

const hostileTitle = "Song\x1b[2J\x1b]0;pwned\x07\nB-side"

func TestLoaderMappersSanitizeRemoteMetadata(t *testing.T) {
	tracks := mapTracks([]Track{{ID: "1", Title: hostileTitle, Artist: "A\x1b[31m", Album: "Al\x00bum", Year: "19\x0799"}})
	got := tracks[0]
	if got.Title != "Song B-side" || got.Artist != "A" || got.Album != "Album" || got.Year != "1999" {
		t.Fatalf("track not sanitized: %+v", got)
	}
	if p := mapPlaylists([]Playlist{{ID: "1", Title: hostileTitle}}); p[0].Title != "Song B-side" {
		t.Fatalf("playlist not sanitized: %q", p[0].Title)
	}
	if a := mapArtists([]Artist{{ID: "1", Name: hostileTitle}}); a[0].Name != "Song B-side" {
		t.Fatalf("artist not sanitized: %q", a[0].Name)
	}
}
