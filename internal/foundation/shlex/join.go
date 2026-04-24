package shlex

import "strings"

// safeBytes[b] is true iff byte b does not require shell quoting when
// emitted by Quote. Matches CPython's shlex safe set: [A-Za-z0-9] plus
// _ @ % + = : , . / -.
var safeBytes = func() [256]bool {
	var t [256]bool
	for b := byte('a'); b <= 'z'; b++ {
		t[b] = true
	}
	for b := byte('A'); b <= 'Z'; b++ {
		t[b] = true
	}
	for b := byte('0'); b <= '9'; b++ {
		t[b] = true
	}
	for _, c := range []byte{'_', '@', '%', '+', '=', ':', ',', '.', '/', '-'} {
		t[c] = true
	}
	return t
}()

func isAllSafe(s string) bool {
	for i := 0; i < len(s); i++ {
		if !safeBytes[s[i]] {
			return false
		}
	}
	return true
}

// Quote returns s shell-escaped so Split parses it as a single token.
// Quote is intentionally NOT idempotent on unsafe input: Quote(Quote("'"))
// differs from Quote("'"). This is required for round-trip correctness.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	if isAllSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// Join returns a POSIX shell command line for args, such that
// Split(Join(args)) is equivalent to args (the two forms nil and
// []string{} round-trip to the same nil). Join quotes each element via
// Quote and separates them with a single ASCII space.
func Join(args []string) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = Quote(a)
	}
	return strings.Join(parts, " ")
}
