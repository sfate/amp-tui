package deezer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteDebugDumpIsOwnerOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	client := &Client{debugDumpsEnabled: true}

	client.writeDebugDump("dump.json", `{"token":"x"}`)

	dir := filepath.Join(home, ".amp-tui-debug")
	for path, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, "dump.json"): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s mode %04o, want %04o", path, got, want)
		}
	}
}
