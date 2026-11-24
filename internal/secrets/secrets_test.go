package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func checkRoundTrip(t *testing.T, s Store) {
	t.Helper()
	if _, err := s.Get("deezer", CredentialKey); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing key: expected ErrNotFound, got %v", err)
	}
	if err := s.Set("deezer", CredentialKey, "token-1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := s.Set("other", CredentialKey, "token-2"); err != nil {
		t.Fatalf("set other: %v", err)
	}
	if got, err := s.Get("deezer", CredentialKey); err != nil || got != "token-1" {
		t.Fatalf("get = %q, err = %v", got, err)
	}
	if err := s.Delete("deezer", CredentialKey); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get("deezer", CredentialKey); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted key: expected ErrNotFound, got %v", err)
	}
	if err := s.Delete("deezer", CredentialKey); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: expected ErrNotFound, got %v", err)
	}
	if got, err := s.Get("other", CredentialKey); err != nil || got != "token-2" {
		t.Fatalf("other provider = %q, err = %v", got, err)
	}
}

func TestMemoryStoreRoundTrip(t *testing.T) {
	checkRoundTrip(t, NewMemory())
}

func TestKeyringStoreRoundTrip(t *testing.T) {
	keyring.MockInit()
	checkRoundTrip(t, NewKeyring())
	if err := keyring.Set(keyringService, "deezer/credential", "direct"); err != nil {
		t.Fatalf("seed keyring: %v", err)
	}
	if got, err := NewKeyring().Get("deezer", CredentialKey); err != nil || got != "direct" {
		t.Fatalf("expected service %q and user provider/key, got %q, %v", keyringService, got, err)
	}
}

func TestFileStoreRoundTripAndModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "amp-tui")
	path := filepath.Join(dir, "secrets.json")
	checkRoundTrip(t, NewFile(path))

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat secrets file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("secrets file mode = %04o, want 0600", got)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat secrets dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("secrets dir mode = %04o, want 0700", got)
	}
	if got, err := NewFile(path).Get("other", CredentialKey); err != nil || got != "token-2" {
		t.Fatalf("new File on same path = %q, err = %v", got, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected only secrets.json after atomic writes, got %d entries", len(entries))
	}
}

func TestFileStoreBrokenFileReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := NewFile(path).Get("deezer", CredentialKey); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("expected parse error, got %v", err)
	}
	if err := NewFile(path).Set("deezer", CredentialKey, "x"); err == nil {
		t.Fatal("expected Set to refuse a broken file")
	}
}

// brokenStore stands in for a keyring without a backend.
type brokenStore struct{ calls int }

var errNoBackend = errors.New("no secret service")

func (b *brokenStore) Get(string, string) (string, error) { b.calls++; return "", errNoBackend }
func (b *brokenStore) Set(string, string, string) error   { b.calls++; return errNoBackend }
func (b *brokenStore) Delete(string, string) error        { b.calls++; return errNoBackend }

func TestFallbackUsedOnlyWhenPrimaryUnavailable(t *testing.T) {
	secondary := NewMemory()
	s := newFallback(&brokenStore{}, secondary)
	if err := s.Set("deezer", CredentialKey, "file-token"); err != nil {
		t.Fatalf("set with broken primary: %v", err)
	}
	if got, err := secondary.Get("deezer", CredentialKey); err != nil || got != "file-token" {
		t.Fatalf("expected value in fallback store, got %q, %v", got, err)
	}
	if got, err := s.Get("deezer", CredentialKey); err != nil || got != "file-token" {
		t.Fatalf("get through fallback = %q, %v", got, err)
	}

	primary := NewMemory()
	secondary = NewMemory()
	_ = secondary.Set("deezer", CredentialKey, "stale")
	s = newFallback(primary, secondary)
	if _, err := s.Get("deezer", CredentialKey); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ErrNotFound from primary must not fall back, got %v", err)
	}
	if err := s.Set("deezer", CredentialKey, "keyring-token"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got, _ := secondary.Get("deezer", CredentialKey); got != "stale" {
		t.Fatalf("working primary must not write the fallback, got %q", got)
	}
}
