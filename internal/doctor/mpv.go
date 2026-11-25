package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// MPVCheck checks the version needed by the playback engine. It is optional in auto mode.
func MPVCheck(optional bool) Check {
	return Check{
		Name:     "mpv 0.41 or newer",
		Hint:     "Install mpv 0.41 or newer, or set engine to native in the amp-tui config.",
		Optional: optional,
		Run: func(ctx context.Context) error {
			cmd := exec.CommandContext(ctx, "mpv", "--version")
			output, err := cmd.CombinedOutput()
			if err != nil {
				if len(output) > 0 {
					return fmt.Errorf("mpv --version: %w: %s", err, strings.TrimSpace(string(output)))
				}
				return fmt.Errorf("mpv --version: %w", err)
			}
			return checkMPVVersion(string(output))
		},
	}
}

func checkMPVVersion(output string) error {
	fields := strings.Fields(strings.SplitN(strings.TrimSpace(output), "\n", 2)[0])
	if len(fields) < 2 || fields[0] != "mpv" {
		return fmt.Errorf("could not parse mpv version from %q", strings.TrimSpace(output))
	}
	version := strings.TrimLeft(fields[1], "v")
	version = strings.SplitN(version, "-", 2)[0]
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return fmt.Errorf("could not parse mpv version %q", fields[1])
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("could not parse mpv version %q", fields[1])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("could not parse mpv version %q", fields[1])
	}
	if major == 0 && minor < 41 {
		return fmt.Errorf("mpv %s is too old; need 0.41 or newer", version)
	}
	return nil
}
