// Package native holds the built-in engines: the Swift helper on macOS and beep elsewhere.
package native

import "github.com/sfate/amp-tui/internal/playback"

const engineName = "native"

// capabilities leave Seek off, because both engines restart the track to move the position.
var capabilities = playback.EngineCaps{Seek: false, Visualizer: true}
