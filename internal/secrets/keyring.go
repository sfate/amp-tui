package secrets

import (
	"errors"

	"github.com/zalando/go-keyring"
)

const keyringService = "amp-tui"

// Keyring stores secrets in the OS keychain: macOS Keychain, Secret Service on Linux, Credential Manager on Windows.
type Keyring struct{}

func NewKeyring() Keyring {
	return Keyring{}
}

func (Keyring) Get(provider, key string) (string, error) {
	value, err := keyring.Get(keyringService, entryName(provider, key))
	return value, mapKeyringErr(err)
}

func (Keyring) Set(provider, key, value string) error {
	return mapKeyringErr(keyring.Set(keyringService, entryName(provider, key), value))
}

func (Keyring) Delete(provider, key string) error {
	return mapKeyringErr(keyring.Delete(keyringService, entryName(provider, key)))
}

func mapKeyringErr(err error) error {
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
