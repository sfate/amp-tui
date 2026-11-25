//go:build !darwin

package applemusic

import "github.com/sfate/amp-tui/internal/provider"

func Register(*provider.Registry) {}
