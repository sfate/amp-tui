package doctor

import (
	"context"

	"github.com/sfate/amp-tui/internal/mediactl"
)

// PlatformChecks returns the checks for macOS.
func PlatformChecks() []Check {
	return []Check{{
		Name: "Prebuilt macOS media helper",
		Hint: "Reinstall amp-tui from a release built for this architecture.",
		Run: func(ctx context.Context) error {
			_, err := mediactl.EnsureHelper()
			return err
		},
	}}
}
