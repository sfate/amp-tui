package tui

import (
	"github.com/sfate/amp-tui/internal/config"
	"github.com/sfate/amp-tui/internal/playback"
	"github.com/sfate/amp-tui/internal/provider"
	"github.com/sfate/amp-tui/internal/secrets"
)

type Option func(*options)

type options struct {
	registry         *provider.Registry
	secrets          secrets.Store
	providerOverride provider.ID
	engine           playback.Engine
	engineFactory    func(config.PlaybackEngine, provider.ID) playback.Engine
}

// WithRegistry sets the providers the TUI can sign in to; main builds it.
func WithRegistry(reg *provider.Registry) Option {
	return func(o *options) { o.registry = reg }
}

// WithSecrets sets the store for login secrets; without it the TUI keeps them in memory only.
func WithSecrets(s secrets.Store) Option {
	return func(o *options) { o.secrets = s }
}

// WithProviderOverride selects the provider for this run only; Settings changes still save.
func WithProviderOverride(id provider.ID) Option {
	return func(o *options) { o.providerOverride = id }
}

// WithEngine sets the engine that plays tracks; without it the TUI browses but cannot play.
func WithEngine(e playback.Engine) Option {
	return func(o *options) { o.engine = e }
}

// WithEngineFactory lets the global engine setting select a new engine without coupling the TUI to implementations.
func WithEngineFactory(factory func(config.PlaybackEngine, provider.ID) playback.Engine) Option {
	return func(o *options) { o.engineFactory = factory }
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
