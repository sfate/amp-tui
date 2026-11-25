# Manual smoke tests

These checks supplement automated tests. Use a test account or local library where possible. Do not paste credentials, cookies, or ARL values into logs or issue reports.

## Shared TUI checks

- [ ] Start amp-tui and confirm the selected provider loads.
- [ ] Change Provider in Settings and confirm playback stops, queue/search clear, and the selected provider loads.
- [ ] Navigate, search, open a playlist where supported, play a track, pause/resume, skip next/previous, and quit.
- [ ] Check supported seek and volume controls. Confirm unsupported quality controls are hidden or ignored for providers without a quality ladder.
- [ ] Confirm track and artist text renders safely, including unusual punctuation and control characters.
- [ ] Run `amp-tui --doctor` and review dependency results.

## Deezer (macOS and Linux)

- [ ] Sign in using the browser flow and, separately when practical, the ARL/Cookie input flow.
- [ ] Check Home, Flow paging, Explore, Favorites, user playlists, and track/playlist/artist search.
- [ ] Play at 128 kbps, 320 kbps, and FLAC where the account supports them; switch quality and check the displayed track and position.
- [ ] Check seek for MP3 playback. FLAC seek/quality transitions may restart playback.
- [ ] Check queue next/previous, repeat modes, volume, crossfade, prebuffering, artwork, and visualizer where supported by the selected engine.

## YouTube Music audio (macOS and Linux)

- [ ] With no saved cookies, search songs and albums and play an audio result.
- [ ] Confirm playback is audio-only and the player uses the resolved audio URL.
- [ ] With a test browser profile or cookie file, sign in and check Recommended, Liked, Watch later, and playlists.
- [ ] Check seek and playback after a resolved URL expires or fails; confirm the provider resolves it again.

## Apple Music local library (macOS)

- [ ] Choose Apple Music and allow macOS Automation access when prompted.
- [ ] Load the local library and playlists; search for and play a track.
- [ ] Check pause/resume, stop, seek, volume, progress updates, and artwork.
- [ ] Check macOS Now Playing and media keys while Music.app is playing through amp-tui.

## Platform media controls

- [ ] macOS: check Control Center Now Playing, artwork, and play/pause/next/previous media keys for native playback and mpv.
- [ ] Linux: in a D-Bus desktop session, check MPRIS metadata/status and `playerctl` play/pause/next/previous/seek controls.
- [ ] On each platform, compare native and mpv playback for volume, seek, crossfade, prebuffer, and visualizer behavior where available.
