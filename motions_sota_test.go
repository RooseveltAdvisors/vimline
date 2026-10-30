package vimline

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// feedSeq splits UTF-8 runes and known <esc> / <bs> / <ctrl-...> tokens cleanly
// without breaking literal '<' or multibyte runes.
func feedSeq(e *Editor, seq string) Action {
	last := ActStay
	for i := 0; i < len(seq); {
		if seq[i] == '<' {
			if j := strings.IndexByte(seq[i:], '>'); j > 1 {
				tok := seq[i+1 : i+j]
				if tok == "esc" || tok == "bs" || tok == "enter" || tok == "left" || tok == "right" || tok == "up" || tok == "down" || strings.HasPrefix(tok, "ctrl-") {
					last = e.Feed(tok)
					i += j + 1
					continue
				}
			}
		}
		r, w := utf8.DecodeRuneInString(seq[i:])
		last = e.Feed(string(r))
		i += w
	}
	return last
}

func TestFindMotionsWithOperatorsAndRepeats(t *testing.T) {
	cases := []struct {
		name     string
		init     string
		startCur int
		keys     string
		wantText string
		wantCur  int
	}{
		{"dfx basic", "abcdefxgh", 1, "dfx", "agh", 1},
		{"dtx basic", "abcdefxgh", 1, "dtx", "axgh", 1},
		{"dtb adjacent inclusive", "ab", 0, "dtb", "b", 0},
		{"dTa adjacent exclusive noop", "ab", 1, "dTa", "ab", 1},
		{"dFb backward exclusive", "abcdef", 4, "dFb", "aef", 1},
		{"dTb backward exclusive", "abcdef", 4, "dTb", "abef", 2},
		{"2dfx pre-op count", "axbxcxd", 0, "2dfx", "cxd", 0},
		{"d2fx post-op count", "axbxcxd", 0, "d2fx", "cxd", 0},
		{"2d2fx multiplied count", "axbxcxdxex", 0, "2d2fx", "ex", 0},
		{"df digit target", "abc9def", 0, "df9", "def", 0},
		{"dt0 digit target", "abc0def", 0, "dt0", "0def", 0},
		{"Unicode df東", "hello 東京 world", 0, "df東", "京 world", 0},
		{"Unicode ct京", "hello 東京 world", 0, "ct京XY<esc>", "XY京 world", 1},
		{"cfx change and dot repeat", "foo=1, bar=2", 0, "cf=X<esc>2w.", "X1, X2", 4},
		{"dfx dot repeat", "a:b:c:d", 0, "df:.", "c:d", 0},
		{"yfx and p paste", "key=value", 0, "yf=$p", "key=valuekey=", 12},
		{"t_ then ; skips adjacent", "a_b_c_d", 0, "t_;", "a_b_c_d", 2},
		{"t_ then d;", "a_b_c_d", 0, "t_d;", "_c_d", 0},
		{"f_ then , reverses", "a_b_c_d", 0, "2f_,", "a_b_c_d", 1},
		{"T_ then , reverses to t_", "a_b_c_d", 6, "2T_,", "a_b_c_d", 4},
		{"find stays on current line", "abc\ndefx", 0, "dfx", "abc\ndefx", 0},
		{"missing target aborts cleanly", "abcdef", 2, "dfz", "abcdef", 2},
		{"esc cancels pending df", "abcdef", 1, "df<esc>x", "acdef", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newNormal(tc.init)
			e.SetCursor(tc.startCur)
			feedSeq(e, tc.keys)
			if e.Text() != tc.wantText {
				t.Fatalf("text = %q, want %q", e.Text(), tc.wantText)
			}
			if e.Cursor() != tc.wantCur {
				t.Fatalf("cursor = %d, want %d (text=%q)", e.Cursor(), tc.wantCur, e.Text())
			}
		})
	}
}

func TestTextObjectsFullSuite(t *testing.T) {
	cases := []struct {
		name     string
		init     string
		startCur int
		keys     string
		wantText string
	}{
		{"diw middle word", "one two three", 5, "diw", "one  three"},
		{"daw middle word", "one two three", 5, "daw", "one three"},
		{"daw last word trims leading space", "one two", 5, "daw", "one"},
		{"ciw and dot repeat", "alpha beta gamma", 0, "ciwONE<esc>w.", "ONE ONE gamma"},
		{"diW big WORD", "foo-bar.baz qux", 4, "diW", " qux"},
		{"daW big WORD", "foo-bar.baz qux", 4, "daW", "qux"},
		{"ci\" from col 0 seeks forward", `say "hello world" now`, 0, `ci"hi<esc>`, `say "hi" now`},
		{"da\" removes quotes and trailing space", `say "hello" now`, 6, `da"`, `say now`},
		{"di' with escaped quote", `it\'s 'real text' ok`, 8, `di'`, `it\'s '' ok`},
		{"ci` backticks", "run `go test ./...` ok", 0, "ci`make<esc>", "run `make` ok"},
		{"di( nested inner", "outer(inner(a, b) + c)", 13, "di(", "outer(inner() + c)"},
		{"2di( outer via count", "outer(inner(a, b) + c)", 13, "2di(", "outer()"},
		{"dab around parens", "call(foo, bar)!", 6, "dab", "call!"},
		{"di[ brackets", "arr[idx + 1] = 0", 5, "di[", "arr[] = 0"},
		{"da{ braces", "fn { body } end", 6, "da{", "fn  end"},
		{"ci< angle brackets", "Vec<String> x", 5, "ci<i32<esc>", "Vec<i32> x"},
		{"dip inner paragraph", "p1a\np1b\n\np2a\np2b", 1, "dip", "\np2a\np2b"},
		{"dap around paragraph", "p1a\np1b\n\np2a\np2b", 1, "dap", "p2a\np2b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newNormal(tc.init)
			e.SetCursor(tc.startCur)
			feedSeq(e, tc.keys)
			if e.Text() != tc.wantText {
				t.Fatalf("text = %q, want %q", e.Text(), tc.wantText)
			}
		})
	}
}

func TestExtendedMotionsBracketsParagraphsAndLineOps(t *testing.T) {
	e := newNormal("fn(a, (b+c)) end")
	e.SetCursor(2)
	feedSeq(e, "%")
	if e.Cursor() != 11 {
		t.Fatalf("%% cursor = %d, want 11", e.Cursor())
	}
	feedSeq(e, "%")
	if e.Cursor() != 2 {
		t.Fatalf("reverse %% cursor = %d, want 2", e.Cursor())
	}
	feedSeq(e, "d%")
	want(t, e, "fn end", "d%")

	e = newNormal("line1\nline2\nline3\nline4")
	e.SetCursor(7)
	feedSeq(e, "dgg")
	want(t, e, "line3\nline4", "dgg")

	e = newNormal("line1\nline2\nline3")
	e.SetCursor(6)
	feedSeq(e, "dG")
	want(t, e, "line1", "dG")

	e = newNormal("one two three")
	e.SetCursor(8)
	feedSeq(e, "dge")
	want(t, e, "one twhree", "dge col 8")

	e = newNormal("one two three")
	e.SetCursor(10)
	feedSeq(e, "dge")
	want(t, e, "one twee", "dge col 10")

	e = newNormal("hello world   ")
	e.SetCursor(6)
	feedSeq(e, "dg_")
	want(t, e, "hello    ", "dg_")

	e = newNormal("a\nb\n\nc\nd\n\ne")
	e.SetCursor(0)
	feedSeq(e, "}")
	if e.Cursor() != 4 {
		t.Fatalf("} cursor = %d, want 4", e.Cursor())
	}

	e = newNormal("hello\n  world\n  again")
	feedSeq(e, "J.")
	want(t, e, "hello world again", "J.")

	e = newNormal("  indented\nsecond")
	feedSeq(e, "Sfresh<esc>")
	want(t, e, "fresh\nsecond", "S")

	e = newNormal("first\nsecond")
	feedSeq(e, "A!<esc>")
	want(t, e, "first!\nsecond", "A on multiline")

	e = newNormal("abcdefghij")
	feedSeq(e, "3sZZ<esc>l.")
	want(t, e, "ZZZZghij", "3s + .")
}
