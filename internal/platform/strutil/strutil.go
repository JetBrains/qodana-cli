/*
 * Copyright 2021-2024 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package strutil

import (
	"runtime"
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
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
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

// QuoteForWindows wraps s in quotes if s contains a typical Windows batch special char and isn't yet quoted.
func QuoteForWindows(s string) string {
	if IsStringQuoted(s) {
		return s
	}
	if runtime.GOOS == "windows" && ContainsWinSpecialChar(s) {
		return `"` + s + `"`
	}
	return s
}

// GetQuotedPath returns a quoted path for the current OS.
func GetQuotedPath(path string) string {
	if runtime.GOOS == "windows" {
		return QuoteForWindows(path)
	}
	return QuoteIfSpace(path)
}

// IsStringQuoted checks if a string is already quoted with double quotes.
func IsStringQuoted(s string) bool {
	return strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")
}

// ContainsWinSpecialChar returns true if s contains any common Windows batch special char that requires quoting.
func ContainsWinSpecialChar(s string) bool {
	specialChars := []string{" ", "(", ")", "^", "&", "|", "<", ">"}
	for _, c := range specialChars {
		if strings.Contains(s, c) {
			return true
		}
	}
	return false
}

// Reverse reverses the given string slice.
func Reverse(s []string) []string {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

// GetLines splits a string into lines. Equivalent to strings.Split except the trailing newline does not produce a line
// (as per POSIX spec). Use this to parse output of commands that print lines to stdout.
func GetLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}
