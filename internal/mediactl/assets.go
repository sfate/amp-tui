//go:build darwin

package mediactl

import "embed"

// embeddedAssets contains the development source and release helpers.
//
//go:embed mac_player_helper.swift darwin/darwin-*
var embeddedAssets embed.FS
