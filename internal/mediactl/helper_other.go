//go:build !darwin

package mediactl

import "errors"

func EnsureHelper() (string, error) {
	return "", errors.New("macOS media controls are only available on macOS")
}
