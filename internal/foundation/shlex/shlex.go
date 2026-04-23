// Package shlex implements strict-POSIX word splitting and quoting for
// shell-style command lines, suitable for argv construction.
//
// Both Split and Join follow IEEE Std 1003.1-2024 §2.2 "Quoting"
// (https://pubs.opengroup.org/onlinepubs/9799919799/utilities/V3_chap02.html#tag_19_02_02):
//
//   - §2.2.1 Escape Character — an unquoted backslash preserves the literal
//     value of the next character; a backslash followed by newline is line
//     continuation (both consumed).
//   - §2.2.2 Single-Quotes — within '...', every byte is literal. A single
//     quote cannot appear inside single quotes.
//   - §2.2.3 Double-Quotes — within "...", the backslash retains escape
//     meaning only when followed by one of { $, `, ", \, <newline> }; any
//     other \X leaves both bytes literal in the token.
//
// We implement word splitting only — no parameter/arithmetic/command
// substitution, no operator tokenisation.
//
// # Deviations from strict POSIX
//
//  1. Carriage return (0x0D) is treated as whitespace, and \<CR><LF> is a
//     line continuation (for input pasted from Windows text). POSIX reserves
//     only <SP>, <HT>, and <LF> as blanks. A bare \<CR> (no <LF> after) is
//     NOT a line continuation — it falls through to the ordinary \X rule.
//  2. '#' is never a comment introducer. We split argv, not shell scripts,
//     so '#' must be literal in paths, regex patterns, and similar.
//  3. '$', '`', '$(...)', '${...}' are literal bytes. We are a lexer, not
//     a shell: there is no expansion to perform.
//
// # Deviations from Python's shlex module
//
// Python's shlex.split(posix=True) preserves the backslash before any byte
// inside "..." other than '"' or '\\'. That contradicts POSIX, which
// consumes the backslash before '$', '`', and <LF> as well. We match POSIX
// on all three:
//
//	"\$"       -> "$"          (Python: \$)
//	"\`"       -> "`"          (Python: \`)
//	"\<LF>"    -> ""           (Python: \<LF>; POSIX: line continuation)
//
// # NUL, UTF-8, goroutines
//
// NUL (0x00) is a valid word byte both in input to Split and in elements
// of args to Join. Quote wraps NUL-containing strings in single quotes;
// the pair round-trips. Non-ASCII UTF-8 input passes through byte-wise —
// the lexer only looks at bytes < 0x80, so multi-byte UTF-8 continuation
// bytes (>= 0x80) are never lexer-significant. Both Split and Join are
// stateless and goroutine-safe.
package shlex

import "fmt"

// Messages returned in ParseError.Msg. Tested by value, not substring.
const (
	msgUnterminatedSingleQuote = "unterminated single quote"
	msgUnterminatedDoubleQuote = "unterminated double quote"
	msgTrailingBackslash       = "trailing backslash"
)

// ParseError describes a malformed input to Split. Pos is a byte offset
// (0-based) into the input string.
type ParseError struct {
	Pos int
	Msg string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("shlex: %s at offset %d", e.Msg, e.Pos)
}
