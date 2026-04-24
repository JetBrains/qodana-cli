package shlex

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test vectors adapted from CPython Lib/test/test_shlex.py (posix_data) are
// marked with a "py:" prefix in the name. CPython is PSF-licensed; test
// vectors (input/expected pairs) are factual data, not copyrighted code.

type splitCase struct {
	name string
	in   string
	want []string
}

// (A) Python posix_data port — excluding comment-mode cases and three
// cases where Python diverges from POSIX (moved to sub-table (B)).
func pythonPosixCases() []splitCase {
	return []splitCase{
		{"py:single_word", "x", []string{"x"}},
		{"py:two_words", "foo bar", []string{"foo", "bar"}},
		{"py:leading_ws", " foo bar", []string{"foo", "bar"}},
		{"py:leading_trailing_ws", " foo bar ", []string{"foo", "bar"}},
		{"py:multi_ws", "foo   bar  bla     fasel", []string{"foo", "bar", "bla", "fasel"}},
		{"py:embedded_runs", "x y  z              xxxx", []string{"x", "y", "z", "xxxx"}},
		{"py:bs_x", `\x bar`, []string{"x", "bar"}},
		{"py:bs_space_x", `\ x bar`, []string{" x", "bar"}},
		{"py:bs_space", `\ bar`, []string{" bar"}},
		{"py:bs_x_mid", `foo \x bar`, []string{"foo", "x", "bar"}},
		{"py:bs_space_x_mid", `foo \ x bar`, []string{"foo", " x", "bar"}},
		{"py:bs_space_mid", `foo \ bar`, []string{"foo", " bar"}},
		{"py:dq_word", `foo "bar" bla`, []string{"foo", "bar", "bla"}},
		{"py:all_dq", `"foo" "bar" "bla"`, []string{"foo", "bar", "bla"}},
		{"py:mixed_dq", `"foo" bar "bla"`, []string{"foo", "bar", "bla"}},
		{"py:dq_start", `"foo" bar bla`, []string{"foo", "bar", "bla"}},
		{"py:sq_word", `foo 'bar' bla`, []string{"foo", "bar", "bla"}},
		{"py:all_sq", `'foo' 'bar' 'bla'`, []string{"foo", "bar", "bla"}},
		{"py:mixed_sq", `'foo' bar 'bla'`, []string{"foo", "bar", "bla"}},
		{"py:sq_start", `'foo' bar bla`, []string{"foo", "bar", "bla"}},
		{"py:adjacent_dq", `blurb foo"bar"bar"fasel" baz`, []string{"blurb", "foobarbarfasel", "baz"}},
		{"py:adjacent_sq", `blurb foo'bar'bar'fasel' baz`, []string{"blurb", "foobarbarfasel", "baz"}},
		{"py:empty_dq_mid", `foo "" bar`, []string{"foo", "", "bar"}},
		{"py:empty_sq_mid", `foo '' bar`, []string{"foo", "", "bar"}},
		{"py:triple_empty_dq", `foo "" "" "" bar`, []string{"foo", "", "", "", "bar"}},
		{"py:triple_empty_sq", `foo '' '' '' bar`, []string{"foo", "", "", "", "bar"}},
		{"py:bs_dq_unquoted", `\"`, []string{`"`}},
		{"py:dq_bs_dq", `"\""`, []string{`"`}},
		{"py:dq_bs_space", `"foo\ bar"`, []string{`foo\ bar`}},     // \<space> in dQ: both literal
		{"py:dq_bs_bs_space", `"foo\\ bar"`, []string{`foo\ bar`}}, // \\ in dQ: consume one
		{"py:dq_bs_bs_space_bs_dq", `"foo\\ bar\""`, []string{`foo\ bar"`}},
		{"py:dq_bs_bs_close_bs_dq", `"foo\\" bar\"`, []string{`foo\`, `bar"`}},
		{"py:dq_multi", `"foo\\ bar\" dfadf"`, []string{`foo\ bar" dfadf`}},
		{"py:dq_multi_triple_bs", `"foo\\\ bar\" dfadf"`, []string{`foo\\ bar" dfadf`}},
		{"py:dq_multi_triple_bs_x", `"foo\\\x bar\" dfadf"`, []string{`foo\\x bar" dfadf`}},
		{"py:dq_bs_x_mid", `"foo\x bar\" dfadf"`, []string{`foo\x bar" dfadf`}},
		{"py:bs_sq_unquoted", `\'`, []string{"'"}},
		{"py:sq_bs_space", `'foo\ bar'`, []string{`foo\ bar`}},
		{"py:sq_bs_bs_space", `'foo\\ bar'`, []string{`foo\\ bar`}},
		{"py:mixed_hard", `"foo\\\x bar\" df'a\ 'df"`, []string{`foo\\x bar" df'a\ 'df`}},
		{"py:bs_dq_foo", `\"foo`, []string{`"foo`}},
		{"py:bs_dq_foo_bs_x", `\"foo\x`, []string{`"foox`}},
		{"py:dq_bs_x", `"foo\x"`, []string{`foo\x`}},
		{"py:dq_bs_trailing_space", `"foo\ "`, []string{`foo\ `}},
		{"py:bs_space_mid_word", `foo\ xx`, []string{"foo xx"}},
		{"py:bs_space_bs_x", `foo\ x\x`, []string{"foo xx"}},
		{"py:bs_space_bs_x_bs_dq", `foo\ x\x\"`, []string{`foo xx"`}},
		{"py:dq_bs_space_bs_x", `"foo\ x\x"`, []string{`foo\ x\x`}},
		{"py:dq_bs_space_bs_x_bs_bs", `"foo\ x\x\\"`, []string{`foo\ x\x\`}},
		{"py:dq_mix_adjacent", `"foo\ x\x\\""foobar"`, []string{`foo\ x\x\foobar`}},
		{"py:dq_mix_adjacent_bs_sq", `"foo\ x\x\\"\'"foobar"`, []string{`foo\ x\x\'foobar`}},
		{"py:dq_mix_adjacent_bs_sq_embedded_sq", `"foo\ x\x\\"\'"fo'obar"`, []string{`foo\ x\x\'fo'obar`}},
		{"py:dq_adjacent_with_sq_dont", `"foo\ x\x\\"\'"fo'obar" 'don'\''t'`,
			[]string{`foo\ x\x\'fo'obar`, "don't"}},
		{"py:dq_adjacent_trailing_bs_bs", `"foo\ x\x\\"\'"fo'obar" 'don'\''t' \\`,
			[]string{`foo\ x\x\'fo'obar`, "don't", `\`}},
		{"py:literal_faces", `:-) ;-)`, []string{":-)", ";-)"}},
		{"py:unicode", "áéíóú", []string{"áéíóú"}},
	}
}

// (B) Python-is-wrong per POSIX: strict POSIX consumes the backslash before
// $, `, and <LF> inside "...". Python preserves it. We follow POSIX.
func pythonDivergentCases() []splitCase {
	return []splitCase{
		{"dq_escaped_dollar", `"\$"`, []string{"$"}},
		{"dq_escaped_backtick", "\"\\`\"", []string{"`"}},
		{"dq_line_cont", "\"foo\\\nbar\"", []string{"foobar"}},
	}
}

// (C) Python agrees with POSIX on these — ordinary cases.
func pythonAgreeingDqEscapeCases() []splitCase {
	return []splitCase{
		{"dq_escaped_quote", `"\""`, []string{`"`}},
		{"dq_escaped_backslash", `"\\"`, []string{`\`}},
		{"dq_non_special", `"\P"`, []string{`\P`}},
	}
}

// (D) Windows path matrix from the ticket.
func windowsPathCases() []splitCase {
	return []splitCase{
		{"win_quoted_single_bs", `"C:\Projects\qodana-cli"`, []string{`C:\Projects\qodana-cli`}},
		{"win_quoted_double_bs", `"C:\\Projects\\qodana-cli"`, []string{`C:\Projects\qodana-cli`}},
		{"win_forward_slash", `C:/Projects/qodana-cli`, []string{`C:/Projects/qodana-cli`}},
		{"win_unquoted_bs_consumed", `C:\Projects\qodana-cli`, []string{`C:Projectsqodana-cli`}},
		{"win_include_dir", `-I"C:\Projects\qodana-cli"`, []string{`-IC:\Projects\qodana-cli`}},
		{"win_program_files_plus_arg",
			`"C:\Program Files\LLVM\bin\clang.exe" -c "src\main.c"`,
			[]string{`C:\Program Files\LLVM\bin\clang.exe`, "-c", `src\main.c`}},
		{"win_post_json_single_bs",
			`c:\tools\clang.exe -c src\main.c`,
			// Unquoted — all \X become X; users must quote Windows paths.
			[]string{`c:toolsclang.exe`, "-c", `srcmain.c`}},
		{"win_post_json_quoted", `"c:\tools\clang.exe" -c "src\main.c"`,
			[]string{`c:\tools\clang.exe`, "-c", `src\main.c`}},
	}
}

// (E) Adjacent quoting / empty-token concatenation.
func adjacentCases() []splitCase {
	return []splitCase{
		{"dq_adjacent", `"a""b"`, []string{"ab"}},
		{"sq_adjacent", `'a''b'`, []string{"ab"}},
		{"dq_then_sq", `"a"'b'`, []string{"ab"}},
		{"sq_then_dq", `'a'"b"`, []string{"ab"}},
		{"word_dq_word", `a"b"c`, []string{"abc"}},
		{"dq_word_dq", `"a"b"c"`, []string{"abc"}},
		{"foo_empty_dq", `foo""`, []string{"foo"}},
		{"empty_dq_foo", `""foo`, []string{"foo"}},
		{"bare_empty_dq", `""`, []string{""}},
		{"two_adjacent_empty_dq", `""""`, []string{""}},
		{"foo_dq_empty_dq_bar", `foo "" bar`, []string{"foo", "", "bar"}},
		{"foo_sq_empty_sq_bar", `foo '' '' bar`, []string{"foo", "", "", "bar"}},
	}
}

// (F) Whitespace cases — empty-result cases assert got == nil (not []string{}).
var whitespaceCases = []struct {
	name string
	in   string
	want []string // nil asserts got == nil (not an empty slice)
}{
	{"empty", "", nil},
	{"space", " ", nil},
	{"tab", "\t", nil},
	{"lf", "\n", nil},
	{"cr", "\r", nil},
	{"runs_ws", "   ", nil},
	{"crlf", "\r\n", nil},
	{"a_cr_b", "a\rb", []string{"a", "b"}},
	{"a_crlf_b", "a\r\nb", []string{"a", "b"}},
	{"trailing_space", "a ", []string{"a"}},
	{"double_space_mid", "a  b", []string{"a", "b"}},
	{"leading_and_trailing", " a ", []string{"a"}},
	{"all_ws_types", " \t\n\r", nil},
}

// (G) Literal special chars per our documented deviations.
func literalSpecialCases() []splitCase {
	return []splitCase{
		{"hash_mid_word", "foo#bar", []string{"foo#bar"}},
		{"hash_at_start", "#foo", []string{"#foo"}},
		{"hash_as_separate_word", "a #b c", []string{"a", "#b", "c"}},
		{"cmd_subst", "$(rm -rf /)", []string{"$(rm", "-rf", "/)"}},
		{"param_expansion", "${foo}", []string{"${foo}"}},
		{"backtick_subst", "`backtick`", []string{"`backtick`"}},
		{"shell_operators_no_ws", "a&b;c|d", []string{"a&b;c|d"}},
		{"double_pipe", "a || b", []string{"a", "||", "b"}},
		{"redirects_literal", "a >b <c", []string{"a", ">b", "<c"}},
	}
}

// (I) Line continuation — LF, CRLF, and bare CR (NOT line continuation).
func lineContinuationCases() []splitCase {
	return []splitCase{
		{"lf_unquoted_mid_word", "foo\\\nbar", []string{"foobar"}},
		{"lf_inside_dq", "\"foo\\\nbar\"", []string{"foobar"}},
		{"lf_at_start_only", "\\\n", nil},
		{"lf_split_mid", "a\\\nb c", []string{"ab", "c"}},
		{"crlf_unquoted", "foo\\\r\nbar", []string{"foobar"}},
		{"bare_cr_unquoted_literal", "foo\\\rbar", []string{"foo\rbar"}},
		{"crlf_inside_dq", "\"a\\\r\nb\"", []string{"ab"}},
		{"bare_cr_inside_dq_preserved", "\"a\\\rb\"", []string{"a\\\rb"}},
	}
}

// (S) UTF-8 pass-through.
func utf8Cases() []splitCase {
	return []splitCase{
		{"unicode_two_words", "héllo 世界", []string{"héllo", "世界"}},
		{"unicode_inside_dq", `"café au lait"`, []string{"café au lait"}},
		{"unicode_after_bs_unquoted", `\é`, []string{"é"}},
		{"unicode_after_bs_in_dq", `"\é"`, []string{`\é`}},
	}
}

func runSplitTable(t *testing.T, cases []splitCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Split(tt.in)
			require.NoError(t, err, "input=%q", tt.in)
			assert.Equal(t, tt.want, got, "input=%q", tt.in)
		})
	}
}

func TestSplit_PythonPosixCases(t *testing.T) { runSplitTable(t, pythonPosixCases()) }
func TestSplit_PythonDivergentCases(t *testing.T) {
	runSplitTable(t, pythonDivergentCases())
}
func TestSplit_PythonAgreeingCases(t *testing.T) {
	runSplitTable(t, pythonAgreeingDqEscapeCases())
}
func TestSplit_WindowsPaths(t *testing.T)        { runSplitTable(t, windowsPathCases()) }
func TestSplit_Adjacent(t *testing.T)            { runSplitTable(t, adjacentCases()) }
func TestSplit_LiteralSpecialChars(t *testing.T) { runSplitTable(t, literalSpecialCases()) }
func TestSplit_LineContinuation(t *testing.T)    { runSplitTable(t, lineContinuationCases()) }
func TestSplit_UTF8(t *testing.T)                { runSplitTable(t, utf8Cases()) }

// (F) Whitespace — explicit nil assertion.
func TestSplit_Whitespace(t *testing.T) {
	for _, tt := range whitespaceCases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Split(tt.in)
			require.NoError(t, err, "input=%q", tt.in)
			if tt.want == nil {
				assert.Nil(t, got, "empty-input result must be nil, not empty slice; input=%q", tt.in)
			} else {
				assert.Equal(t, tt.want, got, "input=%q", tt.in)
			}
		})
	}
}

// ParseError.Error formatting.
func TestParseError_Error(t *testing.T) {
	e := &ParseError{Pos: 7, Msg: msgUnterminatedDoubleQuote}
	assert.Equal(t, "shlex: unterminated double quote at offset 7", e.Error())
}

// (H) Errors — assert typed ParseError with exact Pos and Msg.
func TestSplit_Errors(t *testing.T) {
	cases := []struct {
		name string
		in   string
		pos  int
		msg  string
	}{
		{"unterminated_dq_at_start", `"foo`, 0, msgUnterminatedDoubleQuote},
		{"unterminated_sq_at_start", `'foo`, 0, msgUnterminatedSingleQuote},
		{"bare_sq", `'`, 0, msgUnterminatedSingleQuote},
		{"empty_dq_then_unterminated_sq", `""'`, 2, msgUnterminatedSingleQuote},
		{"mid_token_sq", `foo'bar`, 3, msgUnterminatedSingleQuote},
		{"mid_token_dq", `foo"bar`, 3, msgUnterminatedDoubleQuote},
		{"trailing_bs_after_word", `foo\`, 3, msgTrailingBackslash},
		{"trailing_bs_in_dq", `"foo\`, 4, msgTrailingBackslash},
		{"lone_bs", `\`, 0, msgTrailingBackslash},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Split(tt.in)
			require.Error(t, err, "input=%q", tt.in)
			assert.Nil(t, out, "out must be nil on error; got %#v", out)
			var pe *ParseError
			require.True(t, errors.As(err, &pe), "want *ParseError, got %T: %v", err, err)
			assert.Equal(t, tt.pos, pe.Pos, "Pos mismatch; input=%q", tt.in)
			assert.Equal(t, tt.msg, pe.Msg, "Msg mismatch; input=%q", tt.in)
		})
	}
}

// (K) Never-panic fuzz.
func FuzzSplitNeverPanics(f *testing.F) {
	seeds := []string{
		"", `"`, `'`, `\`, `"\`, `'\`, "$(", `"${x}"`, "a\x00b",
		`"foo\`, "\\\n", "\\\r\n", "a\rb", "héllo", "\x7f",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out, err := Split(s)
		if err != nil {
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("non-ParseError: %T %v", err, err)
			}
			if out != nil {
				t.Fatalf("out must be nil on error: %#v", out)
			}
			if pe.Pos < 0 || pe.Pos > len(s) {
				t.Fatalf("ParseError.Pos %d out of range [0,%d]; input=%q", pe.Pos, len(s), s)
			}
			return
		}
		// Success branch — nothing further to assert; no panic is the point.
		_ = out
	})
}
