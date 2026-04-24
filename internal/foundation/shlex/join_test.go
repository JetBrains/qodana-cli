package shlex

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// (L) Quote — exhaustive single-byte matrix. For every byte 0x00..0xff,
// assert the output matches the specified formula. Doubles as a property
// test for isAllSafe.
func TestQuote_ExhaustiveSingleByte(t *testing.T) {
	wantSafeSet := map[byte]bool{}
	for b := byte('a'); b <= 'z'; b++ {
		wantSafeSet[b] = true
	}
	for b := byte('A'); b <= 'Z'; b++ {
		wantSafeSet[b] = true
	}
	for b := byte('0'); b <= '9'; b++ {
		wantSafeSet[b] = true
	}
	for _, c := range []byte{'_', '@', '%', '+', '=', ':', ',', '.', '/', '-'} {
		wantSafeSet[c] = true
	}
	for i := 0; i < 256; i++ {
		b := byte(i)
		in := string([]byte{b})
		got := Quote(in)
		var want string
		switch {
		case wantSafeSet[b]:
			want = in
		case b == '\'':
			want = "''\"'\"''" // 7 bytes: outer ' + replacement '"'"' + outer '
		default:
			want = "'" + in + "'"
		}
		if got != want {
			t.Errorf("Quote(byte 0x%02x)=%q, want %q", b, got, want)
		}
	}
}

// (M) Quote golden table.
func TestQuote_Golden(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "''"},
		{"simple", "simple", "simple"},
		{"has_space", "has space", "'has space'"},
		{"single_quote_embedded", "it's", `'it'"'"'s'`},
		{"double_quote_embedded", `a"b`, `'a"b'`},
		{"backslash_embedded", `a\b`, `'a\b'`},
		{"windows_path_with_space", `C:\Program Files`, `'C:\Program Files'`},
		{"dash_leading", "-flag", "-flag"},
		{"at_colon_slash", "@domain/user", "@domain/user"},
		{"tab_embedded", "has\tab", "'has\tab'"},
		{"nul_embedded", "has\x00nul", "'has\x00nul'"},
		{"newline_embedded", "a\nb", "'a\nb'"},
		{"shell_meta", "$foo", "'$foo'"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Quote(tt.in), "Quote(%q)", tt.in)
		})
	}
}

// (N) Join golden table.
func TestJoin_Golden(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"nil", nil, ""},
		{"empty_slice", []string{}, ""},
		{"single_safe", []string{"a"}, "a"},
		{"two_safe", []string{"a", "b"}, "a b"},
		{"first_has_trailing_space", []string{"a ", "b"}, "'a ' b"},
		{"second_has_leading_space", []string{"a", " b"}, "a ' b'"},
		{"separator_embedded", []string{"a", " ", "b"}, "a ' ' b"},
		{"embedded_dq", []string{`"a`, `b"`}, `'"a' 'b"'`},
		{"has_empty", []string{"a", "", "b"}, "a '' b"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Join(tt.in), "Join(%#v)", tt.in)
		})
	}
}

// equalArgs treats nil and empty slice as equivalent; see "Empty-input
// return value" in the package doc.
func equalArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// roundTripCorpus — hand-curated slices for the round-trip invariant.
func roundTripCorpus() [][]string {
	var corpus [][]string

	// Empty and near-empty
	corpus = append(corpus,
		nil,
		[]string{},
		[]string{""},
		[]string{"", ""},
		[]string{"a", "", "b"},
	)

	// Every single byte 0..255 as a one-element slice.
	for b := 0; b < 256; b++ {
		corpus = append(corpus, []string{string([]byte{byte(b)})})
	}

	// NUL-containing multi-byte
	corpus = append(corpus,
		[]string{"a\x00b"},
		[]string{"\x00"},
		[]string{"pre\x00\x00post"},
	)

	// Quote-heavy
	corpus = append(corpus,
		[]string{"'"},
		[]string{`"`},
		[]string{"'\""},
		[]string{"a\"b'c"},
	)

	// Backslash-heavy
	corpus = append(corpus,
		[]string{`\`},
		[]string{`\\`},
		[]string{`a\b\c`},
	)

	// Whitespace-heavy
	corpus = append(corpus,
		[]string{" "},
		[]string{"\t"},
		[]string{"\n"},
		[]string{"\r"},
		[]string{" \t\n\r "},
	)

	// Windows paths
	corpus = append(corpus,
		[]string{`C:\Projects\file`},
		[]string{`C:\Program Files\LLVM`},
	)

	// Shell specials
	corpus = append(corpus,
		[]string{"$foo"},
		[]string{"`cmd`"},
		[]string{"$(echo hi)"},
		[]string{"a;b&c|d"},
		[]string{"#comment"},
	)

	// Dash-leading
	corpus = append(corpus,
		[]string{"-I/usr"},
		[]string{"--flag=val"},
		[]string{"--"},
	)

	// UTF-8
	corpus = append(corpus,
		[]string{"héllo 世界"},
		[]string{"café", "au", "lait"},
	)

	// Long mixed slice
	long := make([]string, 50)
	for i := range long {
		long[i] = fmt.Sprintf("arg-%d with 'spaces' and \"quotes\"", i)
	}
	corpus = append(corpus, long)

	return corpus
}

// (O) Round-trip invariant: Split(Join(args)) ~= args.
func TestJoinSplitRoundTrip(t *testing.T) {
	for i, args := range roundTripCorpus() {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			joined := Join(args)
			got, err := Split(joined)
			require.NoError(t, err, "Split(Join(%#v))=%q errored", args, joined)
			if !equalArgs(got, args) {
				t.Fatalf("round-trip mismatch\n args   = %#v\n joined = %q\n got    = %#v", args, joined, got)
			}
		})
	}
}

// (P) Quote output is always a single token.
func TestQuoteOutputIsSingleToken(t *testing.T) {
	var singles []string
	// Every byte
	for b := 0; b < 256; b++ {
		singles = append(singles, string([]byte{byte(b)}))
	}
	// A handful of tricky multi-byte strings
	singles = append(singles,
		"",
		"'",
		`"`,
		`\`,
		"a b",
		"a\nb",
		"café au lait",
		"'\"`$()[]{}",
		strings.Repeat("'", 5),
	)
	for _, s := range singles {
		out, err := Split(Quote(s))
		if err != nil {
			t.Fatalf("Split(Quote(%q)) failed: %v", s, err)
		}
		if len(out) != 1 || out[0] != s {
			t.Fatalf("Quote output not a single token: input=%q Quote=%q Split=%#v", s, Quote(s), out)
		}
	}
}

// (R) Non-idempotence of Quote on unsafe input. Guards against accidental
// "optimization" that would break the round-trip.
func TestQuoteNotIdempotent(t *testing.T) {
	q1 := Quote("'")
	q2 := Quote(q1)
	assert.NotEqual(t, q1, q2, "Quote must not be idempotent on unsafe input")
	s1, err := Split(q1)
	require.NoError(t, err)
	assert.Equal(t, []string{"'"}, s1)
	s2, err := Split(q2)
	require.NoError(t, err)
	assert.Equal(t, []string{q1}, s2)
}

// (Q) Round-trip fuzz via unit-separator encoding — covers variable-arity.
func FuzzSplitJoinRoundTrip(f *testing.F) {
	seeds := []string{
		"",
		"\x1f",
		"\x1f\x1f",
		"a",
		"a\x1fb",
		"a b\x1fc\td",
		"'\x1f\"",
		"\\\x1fx",
		"C:\\path\x1f-I/usr",
		"he\x1fllo",
		"\x00\x1f\x00",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, packed string) {
		var args []string
		if packed != "" {
			args = strings.Split(packed, "\x1f")
		}
		joined := Join(args)
		got, err := Split(joined)
		if err != nil {
			t.Fatalf("round-trip Split errored: %v\n args=%#v\n joined=%q", err, args, joined)
		}
		if !equalArgs(got, args) {
			t.Fatalf("round-trip mismatch\n args   = %#v\n joined = %q\n got    = %#v", args, joined, got)
		}
	})
}
