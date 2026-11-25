# Providers

## Current providers

| Provider | Platform and sign-in | Library and search | Playback and controls |
| --- | --- | --- | --- |
| Deezer | macOS and Linux; sign in with a browser session or paste an ARL/Cookie value | Home, Flow, Explore, Favorites, user playlists; search tracks, playlists, and artists | amp-tui streams audio. Quality choices are 128 kbps, 320 kbps, and FLAC. Quality switching is available. Seek works for non-FLAC playback; FLAC seeking is limited. |
| YouTube Music | macOS and Linux; anonymous search works. Optional sign-in uses a Netscape cookies file or a browser/profile | Anonymous: song and album search. Signed in: Recommended, Liked, Watch later, playlists; search songs and albums | Audio-only. `yt-dlp` resolves the best audio URL and mpv plays it. One quality choice; no Deezer quality ladder. |
| Apple Music | macOS only; no amp-tui account sign-in. Uses the local Music app library and macOS Automation permission | Local library and playlists; search tracks | Music.app owns audio playback. amp-tui sends playback controls and reads progress. |

Spotify integration is postponed. amp-tui does not play video.

## Sign in and switch providers

Open **Settings** from Browse. The **Provider** row changes the active service with Left/Right and saves the selection. Provider switching stops the current track, clears the queue and search results, then loads the selected provider. Press `O` to open a sign-in flow when the active provider supports sign-in. The **Account** row appears for those providers and signs in or out.

Credentials are stored in the OS keychain when available. The fallback secret file is `secrets.json` beside the configuration file and is created with mode `0600`. Provider settings are stored separately under `providers` in `config.json`.

### Deezer

Sign in by opening the temporary browser flow and completing Deezer login, or paste an ARL value / Cookie header containing `arl=...`. The app validates the session before saving it. The ARL is a Deezer session credential; treat it like a password and do not include it in bug reports.

### YouTube Music

Sign in with either a Netscape-format `cookies.txt` file path or `browser[:profile]` (for example `chrome:Default`). The app validates cookies by loading one liked item. yt-dlp reads the cookie source when it runs; amp-tui stores the selected path/profile, not a copy of the browser cookies. Anonymous search does not require sign-in.

YouTube playback requires `mpv` 0.41 or newer, `yt-dlp` 2025.11.12 or newer, and Deno 2.3.0 or newer on `PATH`. Run `amp-tui --doctor` for checks.

### Apple Music

Install and configure Music.app, then choose Apple Music in Settings. macOS may ask permission for amp-tui to control Music.app. There is no cloud Apple Music catalog or subscription playback in this provider; it reads the local library exposed by Music.app.

## Adding a provider

1. Add a package under `internal/provider/<id>` implementing `provider.Provider`: stable ID, display name, sections, bootstrap, section and playlist loading, and search.
2. If the provider needs user credentials, implement `provider.Authenticator` with login steps and validation. Providers that work signed out should implement `provider.AnonymousProvider` and make signed-out behavior explicit.
3. For audio delivered to amp-tui, implement `provider.StreamSource`. Return a service-neutral `provider.Source`, sanitize all remote text before it reaches the TUI, and avoid shell command strings when calling external tools. For a service-owned player, implement `provider.ManagedAudioProvider` and its playback controls instead.
4. Add optional interfaces only for real supported behavior, such as `provider.InfiniteRadio`. `provider.CapabilitiesOf` derives UI feature availability from interfaces and section kinds.
5. Register the factory in `cmd/amp-tui` (or use a platform-specific registration file, as Apple Music does). Keep provider-specific settings in `config.ProviderSettings` and credentials in the secrets store.
6. Add tests with fake HTTP servers or fake command runners. Tests must not use the real config directory, keychain, browser profile, or music library. Include the provider in `docs/smoke-tests.md` when it is ready for manual checks.

Keep the provider package independent from the TUI. Map remote API structures into `internal/core` at the provider boundary.
