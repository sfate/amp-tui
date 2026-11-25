# amp-tui

Terminal music player written in Go with Bubble Tea. It started as [deezer-tui](https://github.com/sfate/deezer-tui) and supports Deezer, YouTube Music audio, and the local Apple Music library on macOS. Spotify integration is postponed.

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

## Providers

| Provider | Availability | Sign-in | Catalog and playback |
| --- | --- | --- | --- |
| Deezer | macOS, Linux | Browser session or ARL/Cookie | Home, Flow, Explore, Favorites, playlists; search tracks, playlists, artists; 128/320 kbps and FLAC |
| YouTube Music | macOS, Linux | Optional browser profile or cookies.txt | Anonymous song/album search; signed-in recommendations, liked tracks, watch later, playlists; audio-only via mpv |
| Apple Music | macOS | Local Music.app permission | Local library and playlists; Music.app owns audio output |

| Provider | Playback | Seek | Quality ladder | Provider-specific sections |
| --- | --- | --- | --- | --- |
| Deezer | amp-tui audio engine | Yes; FLAC is limited | 128 kbps, 320 kbps, FLAC | Home, Flow, Explore, Favorites |
| YouTube Music | mpv, audio-only | Yes | One best-audio choice | Recommended, Liked, Watch later when signed in |
| Apple Music | Managed by Music.app | Yes | Not applicable | Local Library |

Credentials for signed-in providers use the OS keychain when available, with a mode 0600 file fallback. Spotify integration is postponed. Video playback is out of scope. See [provider details](docs/providers.md), [architecture](docs/architecture.md), and the [manual smoke checklist](docs/smoke-tests.md).

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

Development build requirements:

- Go `1.27.3` or newer
- macOS only: Xcode command line tools for the Swift playback helper

Build the binary from the repository root:

```bash
make build
```

On macOS, `make build` builds the native playback helper at first use. Release binaries include a prebuilt helper for their architecture and do not need Xcode command line tools at runtime.

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

## Deezer login

On first launch, `amp-tui` opens a login screen instead of requiring manual config edits. Saved sessions are validated on startup. If Deezer rejects a saved session, the login screen opens again. Other startup failures show an error with `Enter` to retry or `O` to sign in. If you already have a saved session and want to replace it, press `O` from the app to open the same login flow.

Press `Enter` to confirm opening Deezer in a temporary browser session. After signing in, return to the app. It will capture the Deezer session from that temporary browser, validate it, save it, and load your library automatically.

If a supported Chromium browser is not available, press `Tab` on the login screen to switch to the manual fallback with either:

- a Deezer `arl` value
- a copied browser `Cookie` header containing `arl=...`

After validation succeeds, the app stores the ARL in the OS keychain and loads your Deezer library. The token is masked in the UI while you type or paste it.

## Settings

Open `Settings` from the Browse list. The rows are:

- `Provider`: `Left`/`Right` switches to the next registered provider (`Enter` does not, so a stray key press cannot stop playback). A switch stops playback, clears the queue and search results, saves the choice, and loads the new library. If the provider needs a login and has none, the login screen opens.
- `Account`: shows `Signed in` or `Not signed in`. `Enter` signs out (deletes the saved login and opens the login screen) or signs in. Providers without a login do not show this row.
- Theme, Volume, Crossfade, Duration, Display, Compact View: global settings.
- Settings of the active provider, such as `Quality`.

## Provider controls

| Key | Action | Availability |
| --- | --- | --- |
| `,` / `.` | Seek back/forward 10 seconds | Providers/engines with seek; FLAC seek is limited |
| `U` / `I` | Lower/raise quality | Deezer's quality ladder |
| `+` / `-` | Change volume | Playback engine dependent |
| `O` | Sign in to the active provider | Providers that support sign-in |

Other common controls are listed above. Features and search categories vary by provider; unsupported provider-only items are hidden. See [provider controls and sign-in](docs/providers.md#sign-in-and-switch-providers).

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

## Releases

Tagged `v*` releases are built with GoReleaser for macOS (Intel and Apple Silicon) and Linux (x86_64). macOS release archives include the prebuilt Swift playback helper. The Homebrew Cask publisher is configured for `sfate/homebrew-tap`; enabling it requires that tap repository and a `HOMEBREW_TAP_TOKEN` repository secret.

## TODO

- [] Add lyrics loading and a lyrics view
- [] Improve FLAC seeking, or keep documenting FLAC as restart-only for seek/quality transitions
- [] Harden Deezer payload parsing for API shape changes

## Kudos

amp-tui is based on [sfate/deezer-tui](https://github.com/sfate/deezer-tui). The original idea and inspiration was taken from [Minuga-RC/deezer-tui](https://github.com/Minuga-RC/deezer-tui).

## License

MIT. See [LICENSE](LICENSE).
