package vimline

import (
	"strings"
	"testing"
)

// TestNeovimDifferentialVisual drives identical keystroke sequences through
// vimline and live nvim (feedkeys, so " is not eaten as a register).
func TestNeovimDifferentialVisual(t *testing.T) {
	cases := []oracleCase{
		{Name: `vi"d`, Text: `one "two" three`, Pos: 5, Keys: splitOracleKeys(`vi"d`)},
		{Name: `va"d`, Text: `xx"yy"zz`, Pos: 3, Keys: splitOracleKeys(`va"d`)},
		{Name: "viwd", Text: "one two three", Pos: 4, Keys: splitOracleKeys("viwd")},
		{Name: "vawd", Text: "  hello x", Pos: 2, Keys: splitOracleKeys("vawd")},
		{Name: "vwd", Text: "one two three", Pos: 4, Keys: splitOracleKeys("vwd")},
		{Name: "Vjd", Text: "one\ntwo\nthree", Pos: 0, Keys: splitOracleKeys("Vjd")},
		{Name: "ve~", Text: "one two", Pos: 0, Keys: splitOracleKeys("ve~")},
		{Name: "veU", Text: "one two", Pos: 0, Keys: splitOracleKeys("veU")},
		{Name: "veu", Text: "Ab Cd", Pos: 0, Keys: splitOracleKeys("veu")},
		{Name: "vllly", Text: "hello world", Pos: 0, Keys: splitOracleKeys("vllly")},
		{Name: "viwcXesc", Text: "one two", Pos: 4, Keys: splitOracleKeys("viwcX<esc>")},
		{Name: "block yank", Text: "hello world", Pos: 0, Keys: splitOracleKeys("<C-v>lly")},
		{Name: "VjJ", Text: "a\nb\nc", Pos: 0, Keys: splitOracleKeys("VjJ")},
		{Name: "vasd", Text: "One two. Three.", Pos: 0, Keys: splitOracleKeys("vasd")},
		{Name: "vipd", Text: "p1\n\np2", Pos: 0, Keys: splitOracleKeys("vipd")},
		{Name: "vi)d", Text: "(ab)", Pos: 1, Keys: splitOracleKeys("vi)d")},
		{Name: "vd", Text: "one two", Pos: 4, Keys: splitOracleKeys("vd")},
		{Name: "v$d", Text: "one two", Pos: 0, Keys: splitOracleKeys("v$d")},
		{Name: "v2ld", Text: "one two", Pos: 0, Keys: splitOracleKeys("v2ld")},
		{Name: "indent >", Text: "abcdef", Pos: 0, Keys: splitOracleKeys("vll>")},
		{Name: "indent <", Text: "\tabcdef", Pos: 1, Keys: splitOracleKeys("V<")},
		// Spaces, not tabs: live nvim 0.12.4 leaves the cursor at the last
		// column after `=` on a leading-tab line, which contradicts both the
		// first-non-blank rule and vimline's indent operator. Spaces agree.
		{Name: "indent =", Text: "  abcdef", Pos: 0, Keys: splitOracleKeys("V=")},
		{Name: `vi'd`, Text: "a 'bc' d", Pos: 3, Keys: splitOracleKeys("vi'd")},
		{Name: "viWd", Text: "one-two three", Pos: 0, Keys: splitOracleKeys("viWd")},
		{Name: "vaWd", Text: "one-two three", Pos: 0, Keys: splitOracleKeys("vaWd")},
		{Name: "vi{d", Text: "x{ab}y", Pos: 2, Keys: splitOracleKeys("vi{d")},
		{Name: "va[d", Text: "x[ab]y", Pos: 2, Keys: splitOracleKeys("va[d")},
		{Name: "vi<d", Text: "x<ab>y", Pos: 2, Keys: splitOracleKeys("vi<d")},
		{Name: "visd", Text: "One two. Three.", Pos: 2, Keys: splitOracleKeys("visd")},
		{Name: "vapd", Text: "p1\n\np2\n", Pos: 0, Keys: splitOracleKeys("vapd")},
		{Name: "line Vd", Text: "one\ntwo", Pos: 0, Keys: splitOracleKeys("Vd")},
		{Name: "char change", Text: "abcd", Pos: 1, Keys: splitOracleKeys("vlcZ<esc>")},
		{Name: "gv reselect", Text: "abcdef", Pos: 0, Keys: splitOracleKeys("vll<esc>gvd")},
		{Name: "endpoint o then d", Text: "abcdef", Pos: 0, Keys: splitOracleKeys("vllood")},
		{Name: "visual backtick", Text: "a `bc` d", Pos: 3, Keys: splitOracleKeys("vi`d")},
		{Name: "v$d includes newline", Text: "abc\ndef", Pos: 0, Keys: splitOracleKeys("v$d")},
		{Name: "empty vi\"d", Text: `a""b`, Pos: 1, Keys: splitOracleKeys(`vi"d`)},
		{Name: "blockwise c", Text: "abcd\nefgh", Pos: 0, Keys: splitOracleKeys("<C-v>jlcX<esc>")},
		{Name: "linewise Vd curswant", Text: "abcdef\nxyz", Pos: 3, Keys: splitOracleKeys("Vd")},
		{Name: "block $ unequal", Text: "abc\nde\nfgh", Pos: 1, Keys: splitOracleKeys("<C-v>j$d")},
		{Name: "counted entry repeats charwise op", Text: "abcdefghijkl", Pos: 0, Keys: splitOracleKeys("3vdl2vd")},
		{Name: "counted entry repeats linewise op", Text: "a\nb\nc\nd\ne\nf\ng\nh", Pos: 0, Keys: splitOracleKeys("Vjd2vd")},
		{Name: "counted entry repeats block op", Text: "abcd\nefgh\nijkl\nmnop\nqrst\nuvwx", Pos: 0, Keys: splitOracleKeys("<C-v>jlyjjl2<C-v>d")},
		{Name: "counted entry multi-line charwise", Text: "ab\ncd\nef\ngh\nij\nkl\nmn\nop", Pos: 0, Keys: splitOracleKeys("vjd2vd")},
		{Name: "counted entry end column", Text: "abc\ndef\nghi\njkl\nmno\npqr\nstu", Pos: 0, Keys: splitOracleKeys("vjld2vd")},
		{Name: "block put P", Text: "abcd\nefgh\nwxyz\nqrst", Pos: 0, Keys: splitOracleKeys("<C-v>jlyjj0P")},
		{Name: "block put p", Text: "abcd\nefgh\nwxyz\nqrst", Pos: 0, Keys: splitOracleKeys("<C-v>jlyjj0p")},
		{Name: "block put p at buffer end", Text: "abcd\nefgh", Pos: 0, Keys: splitOracleKeys("<C-v>jlyG$p")},
		{Name: "block put p on empty cursor line", Text: "abcd\nefgh\n\nqrst", Pos: 0, Keys: splitOracleKeys("<C-v>jlyjj0p")},
		{Name: "block put pads empty register lines", Text: "ab\nlongish", Pos: 8, Keys: splitOracleKeys("<C-v>kyj0P")},
		{Name: "block put empty row at buffer end", Text: "abc\nde\nz", Pos: 2, Keys: splitOracleKeys("<C-v>jyG0P")},
		{Name: "block put pads short row on existing line", Text: "abc\nfgxy\nz", Pos: 2, Keys: splitOracleKeys("<C-v>jlyj0P")},
		{Name: "block put counted created row", Text: "abc\nfgxy\nz", Pos: 2, Keys: splitOracleKeys("<C-v>jlyG02P")},
		{Name: "ragged block left edge change", Text: "ab\nlongish", Pos: 8, Keys: splitOracleKeys("<C-v>kcXY<esc>")},
		{Name: "ragged block left edge delete", Text: "ab\nlongish", Pos: 8, Keys: splitOracleKeys("<C-v>kd")},
		{Name: "visual object extends linewise", Text: "one two\nthree four", Pos: 0, Keys: splitOracleKeys("Vjiwd")},
		{Name: "visual object extends backward", Text: "one two three", Pos: 4, Keys: splitOracleKeys("vhiwd")},
		{Name: "block text object keeps block", Text: "abcd\nefgh", Pos: 0, Keys: splitOracleKeys("<C-v>liwd")},
		{Name: "counted visual shift", Text: "abcdef", Pos: 0, Keys: splitOracleKeys("v2l3>")},
		{Name: "counted visual shift lines", Text: "a\nb", Pos: 0, Keys: splitOracleKeys("Vj2>")},
		{Name: "counted visual unshift", Text: "\t\t\tabcdef", Pos: 0, Keys: splitOracleKeys("v2l3<")},
		{Name: "visual = preserves column", Text: "  abcdef", Pos: 2, Keys: splitOracleKeys("V=")},
	}
	runOracleDifferential(t, cases, true)
}

func visualOracleSeq(keys []string) string {
	var b strings.Builder
	for _, key := range keys {
		switch key {
		case "\x1b":
			b.WriteString("<Esc>")
		case "\x16":
			b.WriteString("<C-v>")
		case "\x12":
			b.WriteString("<C-r>")
		case "\x08":
			b.WriteString("<BS>")
		case "\r":
			b.WriteString("<CR>")
		default:
			b.WriteString(key)
		}
	}
	return b.String()
}
