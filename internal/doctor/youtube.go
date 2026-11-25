package doctor

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var versionNumbers = regexp.MustCompile(`\d+`)

// YTDLPCheck verifies yt-dlp supports the current YouTube JavaScript challenge flow.
func YTDLPCheck(optional bool) Check {
	return Check{
		Name:     "yt-dlp 2025.11.12 or newer",
		Hint:     "Install or update yt-dlp; YouTube extraction requires the current yt-dlp JavaScript challenge support.",
		Optional: optional,
		Run: func(ctx context.Context) error {
			output, err := exec.CommandContext(ctx, "yt-dlp", "--version").CombinedOutput()
			if err != nil {
				return fmt.Errorf("yt-dlp --version: %w", err)
			}
			return checkYTDLPVersion(string(output))
		},
	}
}

// DenoCheck verifies the recommended runtime used by yt-dlp's YouTube challenge scripts.
func DenoCheck(optional bool) Check {
	return Check{
		Name:     "Deno 2.3.0 or newer",
		Hint:     "Install Deno 2.3.0 or newer and make sure deno is on PATH.",
		Optional: optional,
		Run: func(ctx context.Context) error {
			output, err := exec.CommandContext(ctx, "deno", "--version").CombinedOutput()
			if err != nil {
				return fmt.Errorf("deno --version: %w", err)
			}
			return checkDenoVersion(string(output))
		},
	}
}

func checkYTDLPVersion(output string) error {
	version := strings.TrimSpace(strings.SplitN(output, "\n", 2)[0])
	parts := versionNumbers.FindAllString(version, -1)
	if len(parts) < 3 {
		return fmt.Errorf("could not parse yt-dlp version from %q", version)
	}
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	patch, _ := strconv.Atoi(parts[2])
	if major < 2025 || major == 2025 && (minor < 11 || minor == 11 && patch < 12) {
		return fmt.Errorf("yt-dlp %s is too old; need 2025.11.12 or newer", version)
	}
	return nil
}

func checkDenoVersion(output string) error {
	fields := strings.Fields(strings.TrimSpace(strings.SplitN(output, "\n", 2)[0]))
	if len(fields) < 2 || fields[0] != "deno" {
		return errors.New("could not parse Deno version")
	}
	parts := strings.Split(fields[1], ".")
	if len(parts) < 2 {
		return fmt.Errorf("could not parse Deno version %q", fields[1])
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("could not parse Deno version %q", fields[1])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("could not parse Deno version %q", fields[1])
	}
	if major < 2 || major == 2 && minor < 3 {
		return fmt.Errorf("deno %s is too old; need 2.3.0 or newer", fields[1])
	}
	return nil
}
