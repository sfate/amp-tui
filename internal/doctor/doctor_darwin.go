package doctor

import "context"

// PlatformChecks returns the checks for macOS.
func PlatformChecks() []Check {
	return []Check{{
		Name: "Swift compiler for the playback helper (xcrun swiftc)",
		Hint: "Install the Xcode command line tools: xcode-select --install",
		Run: func(ctx context.Context) error {
			return commandSucceeds(ctx, "/usr/bin/xcrun", "--find", "swiftc")
		},
	}}
}
