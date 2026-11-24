//go:build !darwin

package doctor

// PlatformChecks returns the checks for platforms without runtime dependencies.
func PlatformChecks() []Check {
	return nil
}
