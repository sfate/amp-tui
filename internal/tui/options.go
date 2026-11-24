package tui

import (
	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/secrets"
)

type Option func(*options)

type options struct {
	registry *provider.Registry
	secrets  secrets.Store
}

// WithRegistry sets the providers the TUI can sign in to; main builds it.
func WithRegistry(reg *provider.Registry) Option {
	return func(o *options) { o.registry = reg }
}

// WithSecrets sets the store for login secrets; without it the TUI keeps them in memory only.
func WithSecrets(s secrets.Store) Option {
	return func(o *options) { o.secrets = s }
}

func applyOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if o.secrets == nil {
		o.secrets = secrets.NewMemory()
	}
	return o
}
