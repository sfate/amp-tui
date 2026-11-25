//go:build darwin

package mediactl

import "embed"

//go:embed mac_player_helper.swift
var helper embed.FS

// helperSource is shared by the native playback engine and the control-only process.
func helperSource() ([]byte, error) {
	return helper.ReadFile("mac_player_helper.swift")
}
