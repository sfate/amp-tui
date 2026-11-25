//go:build darwin

package mediactl

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

var (
	helperOnce sync.Once
	helperPath string
	helperErr  error
)

// EnsureHelper installs the embedded release helper or compiles the source helper
// for development builds.
func EnsureHelper() (string, error) {
	helperOnce.Do(func() {
		binary, err := embeddedAssets.ReadFile("darwin/darwin-" + runtime.GOARCH)
		if err == nil {
			helperPath, helperErr = installHelper(binary)
			return
		}
		helperPath, helperErr = compileDevelopmentHelper()
	})
	return helperPath, helperErr
}

func installHelper(binary []byte) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "amp-tui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "mac-player-helper-"+runtime.GOARCH)
	sum := fmt.Sprintf("%x", sha256.Sum256(binary))
	if existing, err := os.ReadFile(path); err == nil && fmt.Sprintf("%x", sha256.Sum256(existing)) == sum {
		return path, nil
	}
	if err := os.WriteFile(path, binary, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func compileDevelopmentHelper() (string, error) {
	source, err := embeddedAssets.ReadFile("mac_player_helper.swift")
	if err != nil {
		return "", err
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "amp-tui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	srcPath := filepath.Join(dir, "mac_player_helper.swift")
	binPath := filepath.Join(dir, "mac-player-helper")
	sumPath := filepath.Join(dir, "mac-player-helper.sha256")
	sourceSum := fmt.Sprintf("%x", sha256.Sum256(source))
	if info, err := os.Stat(binPath); err == nil && !info.IsDir() {
		if existingSum, err := os.ReadFile(sumPath); err == nil && string(existingSum) == sourceSum {
			return binPath, nil
		}
	}
	if err := os.WriteFile(srcPath, source, 0o644); err != nil {
		return "", err
	}
	cmd := exec.Command("/usr/bin/xcrun", "swiftc", "-O", "-o", binPath, srcPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("compile mac player helper: %w: %s", err, string(output))
	}
	if err := os.WriteFile(sumPath, []byte(sourceSum), 0o644); err != nil {
		return "", err
	}
	return binPath, nil
}
