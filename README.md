# amp-tui

Terminal music player written in Go with Bubble Tea. It started as [deezer-tui](https://github.com/sfate/deezer-tui) and grows into one player with several backend providers: Deezer (available now), YouTube, Spotify and Apple Music (planned, see the issues).

<img src='images/preview-loading.png' width='550' alt='amp-tui loading' /> 

## Features

- First-run Deezer login with browser-assisted ARL/Cookie import
- Browse Home, Flow, Explore, Favorites, and user playlists
- Search tracks, playlists, and artists
- Queue playback with next/previous controls
- Repeat modes: off, all, one
- Favorites sorting by added date, ascending or descending
- Streaming quality selection: 128 kbps, 320 kbps, and FLAC
- Playback retry on failure with per-song quality fallback
- Seek controls for non-FLAC playback
- Live quality switching while preserving position where possible
- Configurable crossfade and automatic near-end transitions
- Album artwork rendering in supported terminals
- macOS Now Playing / media key integration through the native helper
- macOS native playback helper for pause, resume, volume, and stop
- Linux/other playback through the Go audio backend
- Theme support: Aetheria, Gruvbox, Winamp, Deezer Dark, Deezer Classic, and Deezer Orange

> [!NOTE]
> Discord Rich Presence is intentionally not implemented.

## Controls

```text
Arrow keys / HJKL  Navigate
Tab                Switch focus
Shift+Tab          Switch focus backward
Enter              Select
Space              Play/pause or play selected track
N                  Next track
P                  Previous track
R                  Cycle repeat mode
,                  Seek back 10 seconds
.                  Seek forward 10 seconds
U                  Lower playback quality
I                  Raise playback quality
+ / -              Volume up/down
S                  Toggle Favorites sort direction
/                  Search
O                  Open/change the provider login
Esc                Leave search/settings
Q                  Quit
```

## Build From Source

Requirements:

- Go `1.27.3` or newer
- macOS only: Xcode command line tools for the Swift playback helper

Build the binary from the repository root:

```bash
make build
```

On macOS, `make build` also precompiles the native playback helper into the user cache.

The binary is written to:

```bash
./bin/amp-tui
```

For a direct development run:

```bash
go run ./cmd/amp-tui
```

Check runtime dependencies:

```bash
./bin/amp-tui --doctor
```

Use another registered provider for one run without saving it (`--help` lists the IDs; an unknown ID exits with status 2):

```bash
./bin/amp-tui --provider deezer
```

Useful checks:

```bash
make lint
make audit
make test
```

## Login

On first launch, `amp-tui` opens a login screen instead of requiring manual config edits. Saved sessions are validated on startup. If Deezer rejects a saved session, the login screen opens again. Other startup failures show an error with `Enter` to retry or `O` to sign in. If you already have a saved session and want to replace it, press `O` from the app to open the same login flow.

Press `Enter` to confirm opening Deezer in a temporary browser session. After signing in, return to the app. It will capture the Deezer session from that temporary browser, validate it, save it, and load your library automatically.

If a supported Chromium browser is not available, press `Tab` on the login screen to switch to the manual fallback with either:

- a Deezer `arl` value
- a copied browser `Cookie` header containing `arl=...`

After validation succeeds, the app stores the ARL in the OS keychain and loads your Deezer library. The token is masked in the UI while you type or paste it.

## Settings

Open `Settings` from the Browse list. The rows are:

- `Provider`: `Left`/`Right`/`Enter` switches to the next registered provider. A switch stops playback, clears the queue and search results, saves the choice, and loads the new library. If the provider needs a login and has none, the login screen opens.
- `Account`: shows `Signed in` or `Not signed in`. `Enter` signs out (deletes the saved login and opens the login screen) or signs in. Providers without a login do not show this row.
- Theme, Volume, Crossfade, Duration, Display, Compact View: global settings.
- Settings of the active provider, such as `Quality`.

## Configuration

The app reads:

```bash
$XDG_CONFIG_HOME/amp-tui/config.json   # or ~/.config/amp-tui/config.json, also on macOS
```

The app creates and updates this file (mode 0600) after a successful login, and fills defaults for missing settings. Per-provider settings such as the default quality live under `providers`:

```json
{
  "schema_version": 2,
  "provider": "deezer",
  "providers": { "deezer": { "default_quality": "Kbps320" } }
}
```

Login secrets are not in this file. The app keeps them in the OS keychain (macOS Keychain, Secret Service on Linux, Credential Manager on Windows) under the service `amp-tui`. If no keychain is available, it uses `secrets.json` (mode 0600) next to `config.json`; `amp-tui --doctor` shows which one applies.

On first start, the app migrates an old `~/.deezer-tui-config.json`: it copies the settings and the ARL into the new locations and leaves the old file as it is. Delete the old file after the migration, because it still holds the ARL in plain text.

## Colors

Currently app has six themes: [Aetheria](https://github.com/JJDizz1L/aetheria), [Gruvbox](https://github.com/morhetz/gruvbox), Winamp, Deezer Dark, Deezer Classic, and Deezer Orange.
Theme can be set in the config file or switched in-app. The default theme is Aetheria:
```json
{
  "theme": "Aetheria"
}
```

## Preview

![amp-tui preview](images/preview.png)

## TODO

- [] Add Linux MPRIS/media-key support behind the same media-control abstraction used for macOS
- [] Add lyrics loading and a lyrics view
- [] Improve FLAC seeking, or keep documenting FLAC as restart-only for seek/quality transitions
- [] Add broader manual smoke-test documentation for macOS Control Center/media keys
- [] Improve packaging and release artifacts for macOS and Linux
- [] Harden Deezer payload parsing for API shape changes

## Kudos

amp-tui is based on [sfate/deezer-tui](https://github.com/sfate/deezer-tui). The original idea and inspiration was taken from [Minuga-RC/deezer-tui](https://github.com/Minuga-RC/deezer-tui).

## License

MIT. See [LICENSE](LICENSE).
