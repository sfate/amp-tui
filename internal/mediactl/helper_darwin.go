//go:build darwin

package mediactl

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

var (
	helperOnce sync.Once
	helperPath string
	helperErr  error
)

// EnsureHelper compiles the shared Swift helper once and returns its cached path.
func EnsureHelper() (string, error) {
	helperOnce.Do(func() {
		source, err := helperSource()
		if err != nil {
			helperErr = err
			return
		}
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			helperErr = err
			return
		}
		dir := filepath.Join(cacheDir, "amp-tui")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			helperErr = err
			return
		}
		srcPath := filepath.Join(dir, "mac_player_helper.swift")
		binPath := filepath.Join(dir, "mac-player-helper")
		sumPath := filepath.Join(dir, "mac-player-helper.sha256")
		sourceSum := fmt.Sprintf("%x", sha256.Sum256(source))
		if info, err := os.Stat(binPath); err == nil && !info.IsDir() {
			if existingSum, err := os.ReadFile(sumPath); err == nil && string(existingSum) == sourceSum {
				helperPath = binPath
				return
			}
		}
		if err := os.WriteFile(srcPath, source, 0o644); err != nil {
			helperErr = err
			return
		}
		cmd := exec.Command("/usr/bin/xcrun", "swiftc", "-O", "-o", binPath, srcPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			helperErr = fmt.Errorf("compile mac player helper: %w: %s", err, string(output))
			return
		}
		if err := os.WriteFile(sumPath, []byte(sourceSum), 0o644); err != nil {
			helperErr = err
			return
		}
		helperPath = binPath
	})
	return helperPath, helperErr
}
