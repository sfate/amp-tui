package applemusic

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type scriptSpec struct {
	language string
	source   string
}

type execScriptRunner struct{}

func (execScriptRunner) Run(ctx context.Context, operation string, args ...string) ([]byte, error) {
	spec, ok := scripts[operation]
	if !ok {
		return nil, fmt.Errorf("unknown Music.app operation %q", operation)
	}
	if _, err := exec.LookPath("osascript"); err != nil {
		return nil, errorsNoAppleScript
	}
	if operation == "artwork" {
		return runArtworkScript(ctx, spec, args)
	}
	return runScript(ctx, spec, args...)
}

var errorsNoAppleScript = fmt.Errorf("osascript was not found; Apple Music control is available only on macOS")

func runScript(ctx context.Context, spec scriptSpec, args ...string) ([]byte, error) {
	cmdArgs := []string{"-l", spec.language, "-e", spec.source, "--"}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, "osascript", cmdArgs...)
	cmd.WaitDelay = 2 * time.Second
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("music.app script failed: %w", err)
	}
	return output, nil
}

func runArtworkScript(ctx context.Context, spec scriptSpec, args []string) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("artwork operation expects a Music.app track ID")
	}
	file, err := os.CreateTemp("", "amp-music-art-*.data")
	if err != nil {
		return nil, fmt.Errorf("create temporary Music.app artwork: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("prepare temporary Music.app artwork: %w", err)
	}
	defer func() { _ = os.Remove(path) }()
	output, err := runScript(ctx, spec, args[0], filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(output)) == "no-art" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect temporary Music.app artwork: %w", err)
	}
	if info.Size() > maxArtworkSize {
		return nil, fmt.Errorf("music.app artwork exceeds %d bytes", maxArtworkSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read temporary Music.app artwork: %w", err)
	}
	return data, nil
}

const jxaCommon = `
ObjC.import("Foundation");
function musicApp() { return Application("Music"); }
function textValue(object, property) {
  try {
    const value = object[property]();
    return value === null || value === undefined ? "" : String(value);
  } catch (_) { return ""; }
}
function numberValue(object, property) {
  try {
    const value = Number(object[property]());
    return Number.isFinite(value) ? value : 0;
  } catch (_) { return 0; }
}
function trackRecord(track) {
  return {
    id: textValue(track, "persistentID"),
    title: textValue(track, "name"),
    artist: textValue(track, "artist"),
    album: textValue(track, "album"),
    duration: numberValue(track, "duration")
  };
}
function findTrack(id) {
  const library = musicApp().libraryPlaylists()[0];
  return library.tracks().find(track => textValue(track, "persistentID") === id);
}
`

var scripts = map[string]scriptSpec{
	"playlists": {language: "JavaScript", source: jxaCommon + `
function run(_) {
  const playlists = musicApp().userPlaylists().map(playlist => ({
    id: textValue(playlist, "persistentID"),
    title: textValue(playlist, "name")
  }));
  return JSON.stringify(playlists);
}`},
	"library_tracks": {language: "JavaScript", source: jxaCommon + `
function run(_) {
  const tracks = musicApp().libraryPlaylists()[0].tracks().map(trackRecord);
  return JSON.stringify(tracks);
}`},
	"playlist_tracks": {language: "JavaScript", source: jxaCommon + `
function run(argv) {
  const id = String(argv[0]);
  const playlist = musicApp().userPlaylists().find(item => textValue(item, "persistentID") === id);
  if (!playlist) throw new Error("playlist was not found in Music.app");
  return JSON.stringify(playlist.tracks().map(trackRecord));
}`},
	"search_tracks": {language: "JavaScript", source: jxaCommon + `
function run(argv) {
  const query = String(argv[0]);
  const library = musicApp().libraryPlaylists()[0];
  const tracks = musicApp().search(library, {for: query, only: "songs"}).map(trackRecord);
  return JSON.stringify(tracks);
}`},
	"track": {language: "JavaScript", source: jxaCommon + `
function run(argv) {
  const track = findTrack(String(argv[0]));
  if (!track) throw new Error("track was not found in the Music.app library");
  return JSON.stringify(trackRecord(track));
}`},
	"play": {language: "JavaScript", source: jxaCommon + `
function run(argv) {
  const app = musicApp();
  const track = findTrack(String(argv[0]));
  if (!track) throw new Error("track was not found in the Music.app library");
  app.play(track, {once: true});
  app.playerPosition = Number(argv[1]) / 1000;
  if (String(argv[2]) === "true") app.pause();
  return "ok";
}`},
	"state": {language: "JavaScript", source: jxaCommon + `
function run(_) {
  const app = musicApp();
  let track = null;
  try { track = trackRecord(app.currentTrack()); } catch (_) {}
  return JSON.stringify({
    state: textValue(app, "playerState"),
    track: track,
    position: numberValue(app, "playerPosition")
  });
}`},
	"pause":  {language: "JavaScript", source: `function run(_) { musicApp().pause(); return "ok"; }`},
	"resume": {language: "JavaScript", source: `function run(_) { musicApp().resume(); return "ok"; }`},
	"stop":   {language: "JavaScript", source: `function run(_) { musicApp().stop(); return "ok"; }`},
	"seek":   {language: "JavaScript", source: `function run(argv) { musicApp().playerPosition = Number(argv[0]) / 1000; return "ok"; }`},
	"volume": {language: "JavaScript", source: `function run(argv) { musicApp().soundVolume = Math.round(Number(argv[0]) * 100); return "ok"; }`},
	"artwork": {language: "AppleScript", source: `on run argv
set trackID to item 1 of argv
set outputPath to item 2 of argv
tell application id "com.apple.Music"
  set targetTrack to first track of library playlist 1 whose persistent ID is trackID
  if (count of artworks of targetTrack) is 0 then return "no-art"
  set imageData to raw data of artwork 1 of targetTrack
end tell
set outputFile to open for access (POSIX file outputPath) with write permission
try
  set eof outputFile to 0
  write imageData to outputFile
  close access outputFile
on error errorMessage number errorNumber
  try
    close access outputFile
  end try
  error errorMessage number errorNumber
end try
return "ok"
end run`},
}
