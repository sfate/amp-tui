// Package secrets keeps provider credentials out of the config file.
package secrets

import (
	"errors"
	"sync"
)

// CredentialKey is the key of a provider's login secret.
const CredentialKey = "credential"

// ErrNotFound means the store works but holds no value for the key.
var ErrNotFound = errors.New("secret not found")

// Store keeps secrets per provider ID and key.
type Store interface {
	Get(provider, key string) (string, error)
	Set(provider, key, value string) error
	Delete(provider, key string) error
}

// Default uses the OS keychain and falls back to the file at fallbackPath when no keychain backend works.
func Default(fallbackPath string) Store {
	return newFallback(NewKeyring(), NewFile(fallbackPath))
}

type fallback struct {
	primary, secondary Store
}

func newFallback(primary, secondary Store) Store {
	return fallback{primary: primary, secondary: secondary}
}

// unavailable treats every primary error except ErrNotFound as a missing backend.
func unavailable(err error) bool {
	return err != nil && !errors.Is(err, ErrNotFound)
}

func (f fallback) Get(provider, key string) (string, error) {
	value, err := f.primary.Get(provider, key)
	if unavailable(err) {
		return f.secondary.Get(provider, key)
	}
	return value, err
}

func (f fallback) Set(provider, key, value string) error {
	if err := f.primary.Set(provider, key, value); unavailable(err) {
		return f.secondary.Set(provider, key, value)
	}
	return nil
}

func (f fallback) Delete(provider, key string) error {
	err := f.primary.Delete(provider, key)
	if unavailable(err) {
		return f.secondary.Delete(provider, key)
	}
	return err
}

// Memory keeps secrets in process memory; tests use it.
type Memory struct {
	mu     sync.Mutex
	values map[string]string
}

func NewMemory() *Memory {
	return &Memory{values: map[string]string{}}
}

func entryName(provider, key string) string {
	return provider + "/" + key
}

func (m *Memory) Get(provider, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.values[entryName(provider, key)]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (m *Memory) Set(provider, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[entryName(provider, key)] = value
	return nil
}

func (m *Memory) Delete(provider, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := entryName(provider, key)
	if _, ok := m.values[name]; !ok {
		return ErrNotFound
	}
	delete(m.values, name)
	return nil
}
