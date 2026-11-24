package deezer

import (
	"github.com/sfate/amp-tui/internal/core"
	"github.com/sfate/amp-tui/internal/sanitize"
)

// The mappers sanitize remote metadata, because the TUI renders it as is.

func mapTracks(in []Track) []core.Track {
	out := make([]core.Track, 0, len(in))
	for _, track := range in {
		out = append(out, core.Track{
			ID:        track.ID,
			Title:     sanitize.Text(track.Title),
			Artist:    sanitize.Text(track.Artist),
			Album:     sanitize.Text(track.Album),
			Year:      sanitize.Text(track.Year),
			AddedAtMS: track.AddedAtMS,
		})
	}
	return out
}

func mapPlaylists(in []Playlist) []core.Playlist {
	out := make([]core.Playlist, 0, len(in))
	for _, playlist := range in {
		out = append(out, core.Playlist{
			ID:    playlist.ID,
			Title: sanitize.Text(playlist.Title),
		})
	}
	return out
}

func mapArtists(in []Artist) []core.Artist {
	out := make([]core.Artist, 0, len(in))
	for _, artist := range in {
		out = append(out, core.Artist{
			ID:   artist.ID,
			Name: sanitize.Text(artist.Name),
		})
	}
	return out
}
