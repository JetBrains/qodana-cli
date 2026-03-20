package str

import (
	"slices"
	"strings"
)

// SafeSplit splits a string by a separator and safely returns the element at the given index.
// If the index is out of range, an empty string is returned.
func SafeSplit(s string, sep string, index int) string {
	parts := strings.Split(s, sep)
	if index >= 0 && index < len(parts) {
		return parts[index]
	}
	return ""
}

// Contains checks if a string is in a given slice.
func Contains(s []string, str string) bool {
	return slices.Contains(s, str)
}

// Remove removes the first occurrence of r from s and returns the shortened slice.
// The returned slice shares the underlying array with s, so s should not be used after calling Remove.
func Remove(s []string, r string) []string {
	for i, v := range s {
		if v == r {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}

// QuoteIfSpace wraps in '"' if the string contains a space.
func QuoteIfSpace(s string) string {
	if IsStringQuoted(s) {
		return s
	}
	if strings.Contains(s, " ") {
		escaped := strings.ReplaceAll(s, `"`, `\"`)
		return "\"" + escaped + "\""
	}
	return s
}

// IsStringQuoted checks if a string is already quoted with double quotes.
func IsStringQuoted(s string) bool {
	return strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")
}

// Reverse reverses the given string slice in-place and returns it.
func Reverse(s []string) []string {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

// GetLines splits a string into lines.
// A "line" is defined as a sequence of characters followed by either:
//   - A newline character `\n`
//   - The end of the string
// This means that a string ending in \n will not produce an empty line at the last element of result.
func GetLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}
