package tui

func trimLastRune(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	return string(runes[:len(runes)-1])
}

func ternary[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

func intPtr(v int) *int { return &v }

func stringPtr(v string) *string { return &v }

func derefOrZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func derefString(v *string, fallback string) string {
	if v == nil {
		return fallback
	}
	return *v
}
