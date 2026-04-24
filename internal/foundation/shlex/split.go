package shlex

import "strings"

// Split parses s as a POSIX shell command line and returns its word tokens.
// Empty and whitespace-only input return (nil, nil). See package doc for
// the full semantics including the three POSIX deviations.
func Split(s string) ([]string, error) {
	const (
		stateWhitespace = iota
		stateWord
		stateSingleQuote
		stateDoubleQuote
	)

	var (
		out       []string
		buffer    strings.Builder
		state     = stateWhitespace
		hasToken  bool
		quoteOpen int // offset of the opening ' or " when in stateSingleQuote / stateDoubleQuote
	)

	emit := func() {
		if hasToken {
			out = append(out, buffer.String())
			buffer.Reset()
			hasToken = false
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch state {

		case stateWhitespace:
			switch {
			case isWhitespace(c):
				// skip
			case c == '\'':
				state = stateSingleQuote
				hasToken = true
				quoteOpen = i
			case c == '"':
				state = stateDoubleQuote
				hasToken = true
				quoteOpen = i
			case c == '\\':
				if i+1 >= len(s) {
					return nil, &ParseError{Pos: i, Msg: msgTrailingBackslash}
				}
				if skipLineContinuation(s, &i) {
					continue
				}
				buffer.WriteByte(s[i+1])
				i++
				hasToken = true
				state = stateWord
			default:
				buffer.WriteByte(c)
				hasToken = true
				state = stateWord
			}

		case stateWord:
			switch {
			case isWhitespace(c):
				emit()
				state = stateWhitespace
			case c == '\'':
				state = stateSingleQuote
				quoteOpen = i
			case c == '"':
				state = stateDoubleQuote
				quoteOpen = i
			case c == '\\':
				if i+1 >= len(s) {
					return nil, &ParseError{Pos: i, Msg: msgTrailingBackslash}
				}
				if skipLineContinuation(s, &i) {
					continue
				}
				buffer.WriteByte(s[i+1])
				i++
			default:
				buffer.WriteByte(c)
			}

		case stateSingleQuote:
			if c == '\'' {
				state = stateWord
			} else {
				buffer.WriteByte(c)
			}

		case stateDoubleQuote:
			switch c {
			case '"':
				state = stateWord
			case '\\':
				if i+1 >= len(s) {
					return nil, &ParseError{Pos: i, Msg: msgTrailingBackslash}
				}
				if skipLineContinuation(s, &i) {
					continue
				}
				n := s[i+1]
				switch n {
				case '$', '`', '"', '\\':
					buffer.WriteByte(n)
					i++
				default:
					// Backslash is literal; n is re-processed in the next
					// iteration by not advancing i here.
					buffer.WriteByte('\\')
				}
			default:
				buffer.WriteByte(c)
			}
		}
	}

	switch state {
	case stateWhitespace, stateWord:
		emit()
	case stateSingleQuote:
		return nil, &ParseError{Pos: quoteOpen, Msg: msgUnterminatedSingleQuote}
	case stateDoubleQuote:
		return nil, &ParseError{Pos: quoteOpen, Msg: msgUnterminatedDoubleQuote}
	}
	return out, nil
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// skipLineContinuation advances *i past \<LF> or \<CR><LF> (with *i currently at
// the backslash) and returns true. Bare \<CR> without a trailing <LF> is
// NOT a line continuation and returns false. Precondition: *i+1 < len(s).
func skipLineContinuation(s string, i *int) bool {
	n := s[*i+1]
	if n == '\n' {
		*i++
		return true
	}
	if n == '\r' && *i+2 < len(s) && s[*i+2] == '\n' {
		*i += 2
		return true
	}
	return false
}
