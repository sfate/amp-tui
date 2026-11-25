//go:build darwin

package applemusic

import "github.com/sfate/amp-tui/internal/provider"

func Register(reg *provider.Registry) {
	reg.Register(ID, "Apple Music", func(cfg provider.Config) (provider.Provider, error) {
		return New(cfg), nil
	})
}
