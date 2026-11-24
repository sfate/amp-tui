package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"

	"github.com/sfate/amp-tui/internal/fsutil"
)

// File stores secrets as plain JSON in an owner-only file, for systems without a keychain.
type File struct {
	path string
	mu   sync.Mutex
}

func NewFile(path string) *File {
	return &File{path: path}
}

func (f *File) Get(provider, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	values, err := f.read()
	if err != nil {
		return "", err
	}
	value, ok := values[entryName(provider, key)]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (f *File) Set(provider, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	values, err := f.read()
	if err != nil {
		return err
	}
	values[entryName(provider, key)] = value
	return f.write(values)
}

func (f *File) Delete(provider, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	values, err := f.read()
	if err != nil {
		return err
	}
	name := entryName(provider, key)
	if _, ok := values[name]; !ok {
		return ErrNotFound
	}
	delete(values, name)
	return f.write(values)
}

// read returns an error for a broken file, so Set never replaces secrets it could not parse.
func (f *File) read() (map[string]string, error) {
	data, err := os.ReadFile(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", f.path, err)
	}
	values := map[string]string{}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse %s: %w", f.path, err)
	}
	return values, nil
}

func (f *File) write(values map[string]string) error {
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(f.path, data, 0o600)
}
