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
	"testing"
)

func TestSafeSplit(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		sep      string
		index    int
		expected string
	}{
		{"valid index 0", "a:b:c", ":", 0, "a"},
		{"valid index 1", "a:b:c", ":", 1, "b"},
		{"valid index 2", "a:b:c", ":", 2, "c"},
		{"index out of range positive", "a:b:c", ":", 5, ""},
		{"index out of range negative", "a:b:c", ":", -1, ""},
		{"empty string", "", ":", 0, ""},
		{"no separator found", "abc", ":", 0, "abc"},
		{"no separator found index 1", "abc", ":", 1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SafeSplit(tt.input, tt.sep, tt.index)
			if result != tt.expected {
				t.Errorf("SafeSplit(%q, %q, %d) = %q, want %q", tt.input, tt.sep, tt.index, result, tt.expected)
			}
		})
	}
}

func TestLower(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"HELLO", "hello"},
		{"Hello", "hello"},
		{"hello", "hello"},
		{"", ""},
		{"123ABC", "123abc"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Lower(tt.input)
			if result != tt.expected {
				t.Errorf("Lower(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		str      string
		expected bool
	}{
		{"found at start", []string{"a", "b", "c"}, "a", true},
		{"found at middle", []string{"a", "b", "c"}, "b", true},
		{"found at end", []string{"a", "b", "c"}, "c", true},
		{"not found", []string{"a", "b", "c"}, "d", false},
		{"empty slice", []string{}, "a", false},
		{"empty string in slice", []string{"", "b"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Contains(tt.slice, tt.str)
			if result != tt.expected {
				t.Errorf("Contains(%v, %q) = %v, want %v", tt.slice, tt.str, result, tt.expected)
			}
		})
	}
}

func TestAppend(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		elem     string
		expected []string
	}{
		{"append new element", []string{"a", "b"}, "c", []string{"a", "b", "c"}},
		{"append existing element", []string{"a", "b"}, "a", []string{"a", "b"}},
		{"append to empty slice", []string{}, "a", []string{"a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Append(tt.slice, tt.elem)
			if len(result) != len(tt.expected) {
				t.Errorf("Append(%v, %q) length = %d, want %d", tt.slice, tt.elem, len(result), len(tt.expected))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("Append(%v, %q)[%d] = %q, want %q", tt.slice, tt.elem, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestRemove(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		remove   string
		expected []string
	}{
		{"remove from start", []string{"a", "b", "c"}, "a", []string{"b", "c"}},
		{"remove from middle", []string{"a", "b", "c"}, "b", []string{"a", "c"}},
		{"remove from end", []string{"a", "b", "c"}, "c", []string{"a", "b"}},
		{"remove non-existent", []string{"a", "b", "c"}, "d", []string{"a", "b", "c"}},
		{"remove from empty", []string{}, "a", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Remove(tt.slice, tt.remove)
			if len(result) != len(tt.expected) {
				t.Errorf("Remove(%v, %q) length = %d, want %d", tt.slice, tt.remove, len(result), len(tt.expected))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("Remove(%v, %q)[%d] = %q, want %q", tt.slice, tt.remove, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestQuoteIfSpace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"hello world", "\"hello world\""},
		{"\"already quoted\"", "\"already quoted\""},
		{"", ""},
		{"path/to/file", "path/to/file"},
		{"path/to/my file", "\"path/to/my file\""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := QuoteIfSpace(tt.input)
			if result != tt.expected {
				t.Errorf("QuoteIfSpace(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsStringQuoted(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"\"quoted\"", true},
		{"not quoted", false},
		{"\"only start", false},
		{"only end\"", false},
		{"", false},
		{"\"\"", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := IsStringQuoted(tt.input)
			if result != tt.expected {
				t.Errorf("IsStringQuoted(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestContainsWinSpecialChar(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"hello world", true},  // space
		{"hello(world)", true}, // parentheses
		{"hello^world", true},  // caret
		{"hello&world", true},  // ampersand
		{"hello|world", true},  // pipe
		{"hello<world", true},  // less than
		{"hello>world", true},  // greater than
		{"helloworld", false},  // no special chars
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ContainsWinSpecialChar(tt.input)
			if result != tt.expected {
				t.Errorf("ContainsWinSpecialChar(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestQuoteForWindows(t *testing.T) {
	assert := func(input, expected string) {
		if result := QuoteForWindows(input); result != expected {
			t.Errorf("QuoteForWindows(%q) = %q, want %q", input, result, expected)
		}
	}
	assert("hello", "hello")
	assert("\"already quoted\"", "\"already quoted\"")
	if runtime.GOOS == "windows" {
		assert("hello world", "\"hello world\"")
	} else {
		assert("hello world", "hello world")
	}
}

func TestGetQuotedPath(t *testing.T) {
	result := GetQuotedPath("path with space")
	expected := "\"path with space\""
	if result != expected {
		t.Errorf("GetQuotedPath(%q) = %q, want %q", "path with space", result, expected)
	}
	if result := GetQuotedPath("nospace"); result != "nospace" {
		t.Errorf("GetQuotedPath(%q) = %q, want %q", "nospace", result, "nospace")
	}
}

func TestReverse(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{"reverse odd", []string{"a", "b", "c"}, []string{"c", "b", "a"}},
		{"reverse even", []string{"a", "b", "c", "d"}, []string{"d", "c", "b", "a"}},
		{"reverse single", []string{"a"}, []string{"a"}},
		{"reverse empty", []string{}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy since Reverse modifies in place
			input := make([]string, len(tt.input))
			copy(input, tt.input)
			result := Reverse(input)
			if len(result) != len(tt.expected) {
				t.Errorf("Reverse(%v) length = %d, want %d", tt.input, len(result), len(tt.expected))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("Reverse(%v)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestGetLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty string", "", []string{""}},
		{"single line no newline", "hello", []string{"hello"}},
		{"single line with newline", "hello\n", []string{"hello"}},
		{"multiple lines no trailing newline", "a\nb\nc", []string{"a", "b", "c"}},
		{"multiple lines with trailing newline", "a\nb\nc\n", []string{"a", "b", "c"}},
		{"only newline", "\n", []string{""}},
		{"empty lines in middle", "a\n\nb", []string{"a", "", "b"}},
		{"empty lines in middle with trailing", "a\n\nb\n", []string{"a", "", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetLines(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("GetLines(%q) length = %d, want %d; got %q", tt.input, len(result), len(tt.expected), result)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("GetLines(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}
