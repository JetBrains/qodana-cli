package shlex

import "strings"

// Split parses s as a POSIX shell command line and returns its word tokens.
// Empty and whitespace-only input return (nil, nil). See package doc for
// the full semantics including the three POSIX deviations.
func Split(s string) ([]string, error) {
	const (
		stateWS = iota
		stateWord
		stateSQ
		stateDQ
	)

	var (
		out       []string
		buf       strings.Builder
		state     = stateWS
		hasToken  bool
		quoteOpen int // offset of the opening ' or " when in sQ / dQ
	)

	emit := func() {
		if hasToken {
			out = append(out, buf.String())
			buf.Reset()
			hasToken = false
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch state {

		case stateWS:
			switch {
			case isWS(c):
				// skip
			case c == '\'':
				state = stateSQ
				hasToken = true
				quoteOpen = i
			case c == '"':
				state = stateDQ
				hasToken = true
				quoteOpen = i
			case c == '\\':
				if i+1 >= len(s) {
					return nil, &ParseError{Pos: i, Msg: msgTrailingBackslash}
				}
				if skipLineCont(s, &i) {
					continue
				}
				buf.WriteByte(s[i+1])
				i++
				hasToken = true
				state = stateWord
			default:
				buf.WriteByte(c)
				hasToken = true
				state = stateWord
			}

		case stateWord:
			switch {
			case isWS(c):
				emit()
				state = stateWS
			case c == '\'':
				state = stateSQ
				quoteOpen = i
			case c == '"':
				state = stateDQ
				quoteOpen = i
			case c == '\\':
				if i+1 >= len(s) {
					return nil, &ParseError{Pos: i, Msg: msgTrailingBackslash}
				}
				if skipLineCont(s, &i) {
					continue
				}
				buf.WriteByte(s[i+1])
				i++
			default:
				buf.WriteByte(c)
			}

		case stateSQ:
			if c == '\'' {
				state = stateWord
			} else {
				buf.WriteByte(c)
			}

		case stateDQ:
			switch c {
			case '"':
				state = stateWord
			case '\\':
				if i+1 >= len(s) {
					return nil, &ParseError{Pos: i, Msg: msgTrailingBackslash}
				}
				if skipLineCont(s, &i) {
					continue
				}
				n := s[i+1]
				switch n {
				case '$', '`', '"', '\\':
					buf.WriteByte(n)
					i++
				default:
					// Backslash is literal; n is re-processed in the next
					// iteration by not advancing i here.
					buf.WriteByte('\\')
				}
			default:
				buf.WriteByte(c)
			}
		}
	}

	switch state {
	case stateWS, stateWord:
		emit()
	case stateSQ:
		return nil, &ParseError{Pos: quoteOpen, Msg: msgUnterminatedSingleQuote}
	case stateDQ:
		return nil, &ParseError{Pos: quoteOpen, Msg: msgUnterminatedDoubleQuote}
	}
	return out, nil
}

func isWS(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// skipLineCont advances *i past \<LF> or \<CR><LF> (with *i currently at
// the backslash) and returns true. Bare \<CR> without a trailing <LF> is
// NOT a line continuation and returns false. Precondition: *i+1 < len(s).
func skipLineCont(s string, i *int) bool {
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
