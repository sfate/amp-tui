// Package sanitize makes remote text safe to render in a terminal.
package sanitize

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Text removes escape sequences and control characters from remote metadata.
// Tabs and line breaks become spaces so a title stays on one line.
func Text(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return ' '
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, s)
}
