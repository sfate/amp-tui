package tui

import "github.com/sfate/amp-tui/internal/provider"

type Option func(*options)

type options struct {
	registry *provider.Registry
}

// WithRegistry sets the providers the TUI can sign in to; main builds it.
func WithRegistry(reg *provider.Registry) Option {
	return func(o *options) { o.registry = reg }
}

func applyOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
