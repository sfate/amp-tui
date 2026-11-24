package sanitize

import "testing"

func TestText(t *testing.T) {
	tests := map[string]struct {
		in, want string
	}{
		"plain":        {"Daft Punk", "Daft Punk"},
		"csi clear":    {"Song\x1b[2J\x1b[H", "Song"},
		"osc title":    {"\x1b]0;pwned\x07Song", "Song"},
		"osc8 link":    {"\x1b]8;;https://x.test\x1b\\Song\x1b]8;;\x1b\\", "Song"},
		"c1 csi":       {"A\u009b31mB", "A31mB"},
		"bell and nul": {"A\x07\x00B", "AB"},
		"line breaks":  {"A\nB\tC\r", "A B C "},
		"unicode kept": {"Sigur Rós – Hoppípolla 👨‍👩‍👧", "Sigur Rós – Hoppípolla 👨‍👩‍👧"},
		"escape seq":   {"A\x1bB", "A"},
		"trailing esc": {"A\x1b", "A"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Text(tc.in); got != tc.want {
				t.Fatalf("Text(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
