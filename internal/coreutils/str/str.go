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

// Lower is a shortcut to strings.ToLower.
func Lower(s string) string {
	return strings.ToLower(s)
}

// Contains checks if a string is in a given slice.
func Contains(s []string, str string) bool {
	return slices.Contains(s, str)
}

// Append appends a string to a slice if it's not already there.
func Append(slice []string, elems ...string) []string {
	if !Contains(slice, elems[0]) {
		slice = append(slice, elems[0])
	}
	return slice
}

// Remove removes a string from a slice.
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
		return "\"" + s + "\""
	}
	return s
}

// IsStringQuoted checks if a string is already quoted with double quotes.
func IsStringQuoted(s string) bool {
	return strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")
}

// Reverse reverses the given string slice.
func Reverse(s []string) []string {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}
