package doctor

import (
	"context"
	"errors"

	"github.com/sfate/amp-tui/internal/secrets"
)

// SecretStoreCheck reads a missing key from store; it never writes, so the user's keychain stays as it is.
func SecretStoreCheck(store secrets.Store, fallbackPath string) Check {
	return Check{
		Name:     "OS keychain for login secrets",
		Optional: true,
		Hint:     "Without a keychain, amp-tui stores logins in " + fallbackPath + " (mode 0600)",
		Run: func(context.Context) error {
			_, err := store.Get("amp-tui-doctor", "probe")
			if err == nil || errors.Is(err, secrets.ErrNotFound) {
				return nil
			}
			return err
		},
	}
}
