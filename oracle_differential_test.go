package vimline

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type oracleCase struct {
	Name   string   `json:"name"`
	Text   string   `json:"text"`
	Pos    int      `json:"pos"`
	Row    int      `json:"row"`
	Col    int      `json:"col"`
	Keys   []string `json:"keys"`
	Seq    string   `json:"seq"`
	Result oracleResult
}

type oracleResult struct {
	Text string `json:"text"`
	Pos  int    `json:"pos"`
	Reg  string `json:"reg"`
}

func TestNeovimDifferentialV031Acceptance(t *testing.T) {
	cases := []oracleCase{
		{Name: "cw dot", Text: "one (a (b) c)", Pos: 0, Keys: splitOracleKeys("cwQQ<esc>.")},
		{Name: "operator underscore", Text: "one (two)\nthree four\n\nfive", Pos: 6, Keys: splitOracleKeys("d_")},
		{Name: "inner paragraph", Text: "one\ntwo\n\nthree", Pos: 2, Keys: splitOracleKeys("cipQ<esc>")},
		{Name: "counted pair ahead", Text: "one (a (b) c)", Pos: 0, Keys: splitOracleKeys("2di(")},
		{Name: "yank placement", Text: "one two three", Pos: 5, Keys: splitOracleKeys("yiw")},
		{Name: "delete placement", Text: "one (two)\nthree four\n\nfive", Pos: 6, Keys: splitOracleKeys("dgg")},
		{Name: "atomic undo", Text: "one (a (b) c)", Pos: 0, Keys: splitOracleKeys("cwQQ<esc>u")},
		{Name: "atomic redo", Text: "one (a (b) c)", Pos: 0, Keys: splitOracleKeys("cwQQ<esc>u<ctrl-r>")},
		{Name: "replace mode", Text: "abcdef", Pos: 0, Keys: splitOracleKeys("RXYZ<esc>")},
		{Name: "open below", Text: "one\ntwo", Pos: 0, Keys: splitOracleKeys("oQQ<esc>")},
		{Name: "dot repeats search delete", Text: "xx yy xx yy zz", Pos: 2, Keys: splitOracleKeys("d/xx<cr>.")},
		{Name: "dot repeats search change", Text: "xx yy xx yy zz", Pos: 2, Keys: splitOracleKeys("c/xx<cr>Q<esc>.")},
		{Name: "dot repeats backward search delete", Text: "aa xx bb xx cc", Pos: 12, Keys: splitOracleKeys("d?xx<cr>.")},
		{Name: "dot repeats backward search change", Text: "aa xx bb xx cc", Pos: 12, Keys: splitOracleKeys("c?xx<cr>Q<esc>.")},
		{Name: "dot repeats mark change", Text: "one two three", Pos: 0, Keys: splitOracleKeys("wmac`aQ<esc>.")},
		{Name: "dot repeat keeps mark absolute column", Text: "one two", Pos: 0, Keys: splitOracleKeys("wma0c`aQ<esc>.")},
		{Name: "mark keeps column after same-line delete", Text: "one two", Pos: 4, Keys: splitOracleKeys("ma0xx`a")},
		{Name: "mark keeps column after earlier-line delete", Text: "one\ntwo three", Pos: 6, Keys: splitOracleKeys("ma-0xx`a")},
		{Name: "mark change and dot after earlier-line delete", Text: "one\ntwo three", Pos: 6, Keys: splitOracleKeys("ma-0xxc`aQ<esc>.")},
		{Name: "mark keeps column across line join", Text: "one\ntwoxyz", Pos: 6, Keys: splitOracleKeys("ma-0lld/w<cr>`a")},
		{Name: "mark inside deleted span keeps column", Text: "one two", Pos: 1, Keys: splitOracleKeys("ma0d2l`a")},
		{Name: "over-eol mark survives d dollar insert", Text: "one two", Pos: 6, Keys: splitOracleKeys("ma0d$iabc<esc>`a")},
		{Name: "over-eol mark survives dw insert", Text: "one two", Pos: 6, Keys: splitOracleKeys("ma0dw0iZZZZ<esc>`a")},
		{Name: "join change dot at over-eol mark", Text: "one\ntwoxyz", Pos: 9, Keys: splitOracleKeys("ma-0lld/o<cr>A23<esc>0c`aQ<esc>")},
		{Name: "long interior delete keeps mark jump", Text: "ab\nXXXXXXXXXX\ncd", Pos: 12, Keys: splitOracleKeys("ma-lde`a")},
		{Name: "long interior delete keeps mark column", Text: "ab\nXXXXXXXXXX\ncd", Pos: 12, Keys: splitOracleKeys("ma-lde0i0123456789<esc>`a")},
		{Name: "das on whitespace selects following sentence", Text: "One two. Three four. Five six.", Pos: 8, Keys: splitOracleKeys("das")},
		{Name: "das on leading whitespace selects sentence", Text: " One two. Three four.", Pos: 0, Keys: splitOracleKeys("das")},
		{Name: "das on multi space whitespace", Text: "One two.   Three four.", Pos: 10, Keys: splitOracleKeys("das")},
		{Name: "counted das selects whole sentences", Text: "One two. Three four. Five six.", Pos: 12, Keys: splitOracleKeys("2das")},
		{Name: "d2as selects whole sentences", Text: "One two. Three four. Five six.", Pos: 12, Keys: splitOracleKeys("d2as")},
		{Name: "counted cas selects whole sentences", Text: "One two. Three four. Five six.", Pos: 12, Keys: splitOracleKeys("c2asQ<esc>")},
		{Name: "3das clamps to available sentences", Text: "One two. Three four. Five six.", Pos: 12, Keys: splitOracleKeys("3das")},
		{Name: "counted dis steps sentence parts", Text: "One two. Three four. Five six.", Pos: 12, Keys: splitOracleKeys("2dis")},
		{Name: "das on last sentence uses leading whitespace", Text: "One two. Three four. Five six.", Pos: 25, Keys: splitOracleKeys("das")},
		{Name: "dis on whitespace selects whitespace run", Text: "One two. Three four. Five six.", Pos: 8, Keys: splitOracleKeys("dis")},
		{Name: "das on end whitespace is empty object", Text: "One two.   ", Pos: 9, Keys: splitOracleKeys("das")},
		{Name: "overshot dis keeps sentence end parity", Text: "One two. ", Pos: 2, Keys: splitOracleKeys("3dis")},
		{Name: "das spans multiline sentence", Text: "One\ntwo here. Three.", Pos: 5, Keys: splitOracleKeys("das")},
		{Name: "das whitespace stops at blank line", Text: "One two.\n\nThree.", Pos: 2, Keys: splitOracleKeys("das")},
		{Name: "cas dot repeat reselects sentence", Text: "One two. Three four. Five six.", Pos: 12, Keys: splitOracleKeys("casQ<esc>.")},
		{Name: "dot repeat of counted das", Text: "One two. Three four. Five six.", Pos: 12, Keys: splitOracleKeys("2das.")},
		{Name: "rparen stops at blank line", Text: "One two.\n\nThree.", Pos: 0, Keys: splitOracleKeys(")")},
		{Name: "d rparen deletes to blank line", Text: "One two.\n\nThree.", Pos: 0, Keys: splitOracleKeys("d)")},
		{Name: "counted rparen stops at second blank", Text: "One two.\n\nThree. Four.", Pos: 0, Keys: splitOracleKeys("2)")},
		{Name: "rparen ordinary", Text: "One two. Three four.", Pos: 0, Keys: splitOracleKeys(")")},
		{Name: "lparen ordinary", Text: "One two. Three four.", Pos: 12, Keys: splitOracleKeys("(")},
		{Name: "counted lparen", Text: "One two. Three four. Five.", Pos: 12, Keys: splitOracleKeys("2(")},
		{Name: "lparen stops at blank line", Text: "One two.\n\nThree.", Pos: 10, Keys: splitOracleKeys("(")},
		{Name: "counted lparen across blank", Text: "One two.\n\nThree.", Pos: 10, Keys: splitOracleKeys("2(")},
		{Name: "d rparen ordinary", Text: "One two. Three four.", Pos: 0, Keys: splitOracleKeys("d)")},
		{Name: "counted d rparen deletes sentences", Text: "One two. Three four. Five six.", Pos: 0, Keys: splitOracleKeys("d2)")},
		{Name: "dot repeat of d rparen", Text: "One two. Three four. Five six.", Pos: 0, Keys: splitOracleKeys("d).")},
		{Name: "rparen at last sentence clamps", Text: "One two.", Pos: 0, Keys: splitOracleKeys(")")},
		{Name: "overshot rparen clamps to end", Text: "One two. Three", Pos: 0, Keys: splitOracleKeys("2)")},
		{Name: "overshot rparen aborts", Text: "One two. Three", Pos: 0, Keys: splitOracleKeys("3)")},
		{Name: "overshot lparen aborts", Text: "One two. Three. Four.", Pos: 17, Keys: splitOracleKeys("4(")},
		{Name: "d rparen at eol deletes to end", Text: "One two. ", Pos: 0, Keys: splitOracleKeys("d)")},
		{Name: "cas on trailing whitespace inserts", Text: "One two.   ", Pos: 9, Keys: splitOracleKeys("casQ<esc>")},
		{Name: "cas on empty buffer inserts", Text: "", Pos: 0, Keys: splitOracleKeys("casQ<esc>")},
		{Name: "cis on empty buffer inserts", Text: "", Pos: 0, Keys: splitOracleKeys("cisQ<esc>")},
		{Name: "cas dot repeat on trailing whitespace", Text: "One two.   ", Pos: 9, Keys: splitOracleKeys("casQ<esc>.")},
		{Name: "cas space dot repeat inserts again", Text: "One two.   ", Pos: 9, Keys: splitOracleKeys("cas <esc>.")},
		{Name: "dot repeats bracket change", Text: "(a)b(c", Pos: 5, Keys: splitOracleKeys("c[(Q<esc>.")},
		{Name: "bracket change undoes atomically", Text: "(a)b(c", Pos: 5, Keys: splitOracleKeys("c[(Q<esc>u")},
		{Name: "counted dd clamps to remaining lines", Text: "one\ntwo", Pos: 0, Keys: splitOracleKeys("3dd")},
		{Name: "counted cc clamps to remaining lines", Text: "one\ntwo", Pos: 0, Keys: splitOracleKeys("3ccX<esc>")},
		{Name: "counted yy clamps to remaining lines", Text: "one\ntwo", Pos: 0, Keys: splitOracleKeys("3yy")},
		{Name: "counted d underscore clamps", Text: "one\ntwo", Pos: 0, Keys: splitOracleKeys("3d_")},
		{Name: "2dd aborts on single line", Text: "one", Pos: 0, Keys: splitOracleKeys("2dd")},
		{Name: "2S aborts on single line", Text: "one", Pos: 0, Keys: splitOracleKeys("2SX<esc>")},
		{Name: "d plus aborts on last line", Text: "one\ntwo", Pos: 4, Keys: splitOracleKeys("d+")},
		{Name: "d minus aborts on first line", Text: "one\ntwo", Pos: 0, Keys: splitOracleKeys("d-")},
		{Name: "dot repeat of counted dd aborts at edge", Text: "one\ntwo\nthree", Pos: 0, Keys: splitOracleKeys("2dd.")},
		{Name: "counted Y clamps to remaining lines", Text: "one\ntwo", Pos: 0, Keys: splitOracleKeys("3Y")},
		{Name: "2Y aborts on single line", Text: "one", Pos: 0, Keys: splitOracleKeys("2Y")},
		{Name: "counted plus clamps at last line", Text: "one\ntwo\nthree", Pos: 1, Keys: splitOracleKeys("3+")},
		{Name: "counted minus clamps at first line", Text: "one\ntwo\nthree", Pos: 9, Keys: splitOracleKeys("3-")},
		// r1: leaving insert moves left unless in column 0
		{Name: "append esc moves left", Text: "abcd", Pos: 1, Keys: splitOracleKeys("a<esc>x")},
		{Name: "insert esc moves left", Text: "abcd", Pos: 2, Keys: splitOracleKeys("i<esc>")},
		{Name: "repeated append esc stays", Text: "abcd", Pos: 1, Keys: splitOracleKeys("a<esc>a<esc>")},
		{Name: "replace esc moves left", Text: "abcd", Pos: 2, Keys: splitOracleKeys("R<esc>")},
		{Name: "substitute esc moves left", Text: "abcd", Pos: 2, Keys: splitOracleKeys("s<esc>")},
		{Name: "counted insert esc moves left", Text: "abcd", Pos: 2, Keys: splitOracleKeys("3i<esc>")},
		{Name: "backspace past start esc", Text: ")ab", Pos: 2, Keys: splitOracleKeys("i<bs>Q<esc>")},
		{Name: "dot replay esc stays in column 0", Text: "ab\ncd\nef", Pos: 3, Keys: splitOracleKeys("s<esc>+.")},
		{Name: "dot replay cw esc stays in column 0", Text: "ab\ncd\nef", Pos: 3, Keys: splitOracleKeys("cw<esc>+.")},
		{Name: "append esc after wide rune", Text: "a😀b", Pos: 1, Keys: splitOracleKeys("la<esc>")},
		// r2: counted l cut short by the line end is inclusive
		{Name: "d2l at line end", Text: "abc", Pos: 1, Keys: splitOracleKeys("d2l")},
		{Name: "c2l at line end", Text: "abc", Pos: 1, Keys: splitOracleKeys("c2lQ<esc>")},
		{Name: "c3l at line end", Text: "abcd", Pos: 2, Keys: splitOracleKeys("c3lQ<esc>")},
		{Name: "y2l at line end", Text: "abc", Pos: 1, Keys: splitOracleKeys("y2l")},
		{Name: "d2 space at line end", Text: "abc", Pos: 1, Keys: splitOracleKeys("d2 ")},
		{Name: "dot repeat d2l", Text: "abc", Pos: 1, Keys: splitOracleKeys("d2l.")},
		// r3: counted w to the buffer end is linewise only across lines
		{Name: "d2w single line stays charwise", Text: "cd ef", Pos: 0, Keys: splitOracleKeys("d2wp")},
		{Name: "d2w last line keeps break", Text: "ab\ncd ef", Pos: 3, Keys: splitOracleKeys("d2w")},
		{Name: "d3w across lines is linewise", Text: "ab\ncd\nef gh", Pos: 3, Keys: splitOracleKeys("d3w")},
		{Name: "d3w from indent is linewise", Text: "ab\n  cd\nef gh", Pos: 5, Keys: splitOracleKeys("d3w")},
		{Name: "d3w linewise keeps column", Text: "abcdef\n  cd\nef gh", Pos: 9, Keys: splitOracleKeys("d3w")},
		// r4: dollar on an empty line is an empty span
		{Name: "D on empty line", Text: "a\n\nb", Pos: 2, Keys: splitOracleKeys("D")},
		{Name: "C on empty line", Text: "a\n\nb", Pos: 2, Keys: splitOracleKeys("CQ<esc>")},
		{Name: "y dollar on empty line", Text: "a\n\nb", Pos: 2, Keys: splitOracleKeys("y$")},
		{Name: "dot repeat D on empty line", Text: "a\n\nb\n\nc", Pos: 2, Keys: splitOracleKeys("D.")},
		{Name: "d2 dollar onto empty line", Text: "ab\n\nb", Pos: 0, Keys: splitOracleKeys("d2$")},
		{Name: "d2 dollar to buffer end is linewise", Text: "a\nb\nc", Pos: 2, Keys: splitOracleKeys("d2$")},
		{Name: "d2 dollar linewise keeps column", Text: "abcdef\n  b\nc", Pos: 10, Keys: splitOracleKeys("d2$")},
		{Name: "c2 dollar stays charwise", Text: "a\nb\nc", Pos: 2, Keys: splitOracleKeys("c2$Q<esc>")},
		// r5: exclusive spans ending in column 0
		{Name: "dis keeps break at column 0", Text: "Hello there. How are you?\nNext line.", Pos: 15, Keys: splitOracleKeys("dis")},
		{Name: "cis keeps break at column 0", Text: "Hello there. How are you?\nNext line.", Pos: 15, Keys: splitOracleKeys("cisQ<esc>")},
		{Name: "das keeps break at column 0", Text: "Hello there. How are you?\nNext line.", Pos: 15, Keys: splitOracleKeys("das")},
		{Name: "yis keeps break at column 0", Text: "Hello there. How are you?\nNext line.", Pos: 15, Keys: splitOracleKeys("yis")},
		{Name: "d rparen keeps break at column 0", Text: "xx One two.\n\nThree.", Pos: 3, Keys: splitOracleKeys("d)")},
		{Name: "c rparen from line start is linewise", Text: "One.\nTwo.", Pos: 0, Keys: splitOracleKeys("c)Q<esc>")},
		{Name: "d rparen linewise register pastes lines", Text: "One.\nTwo.", Pos: 0, Keys: splitOracleKeys("d)p")},
		{Name: "d rparen from indent is linewise", Text: "  One.\nTwo three.", Pos: 1, Keys: splitOracleKeys("d)")},
		{Name: "c rparen linewise keeps indent", Text: "  One.\nTwo.", Pos: 1, Keys: splitOracleKeys("c)Q<esc>")},
		{Name: "y rparen linewise keeps cursor", Text: "  One.\nTwo three.", Pos: 2, Keys: splitOracleKeys("y)")},
		{Name: "dis linewise lands on first non-blank", Text: "  One.\n   Two.", Pos: 2, Keys: splitOracleKeys("dis")},
		{Name: "c lparen from column 0 is linewise", Text: "One.\nTwo.", Pos: 5, Keys: splitOracleKeys("c(Q<esc>")},
		{Name: "dot repeat c rparen linewise", Text: "One.\nTwo.\nThree.", Pos: 0, Keys: splitOracleKeys("c)Q<esc>+.")},
		{Name: "dot repeat cis linewise", Text: "One.\nTwo.\nThree.", Pos: 0, Keys: splitOracleKeys("cisQ<esc>+.")},
		{Name: "c rparen undoes atomically", Text: "One.\nTwo.", Pos: 0, Keys: splitOracleKeys("c)Q<esc>u")},
		{Name: "search delete keeps break at column 0", Text: "xx One.\nTwo.", Pos: 3, Keys: splitOracleKeys("d/Two<cr>")},
		{Name: "search change keeps break at column 0", Text: "xx One.\nTwo.", Pos: 3, Keys: splitOracleKeys("c/Two<cr>Q<esc>")},
		{Name: "search yank keeps break at column 0", Text: "xx One.\nTwo.", Pos: 3, Keys: splitOracleKeys("y/Two<cr>")},
		{Name: "search change from line start is linewise", Text: "One.\nTwo.", Pos: 0, Keys: splitOracleKeys("c/Two<cr>Q<esc>")},
		{Name: "search delete linewise register", Text: "One.\nTwo.", Pos: 0, Keys: splitOracleKeys("d/Two<cr>p")},
		{Name: "search yank linewise keeps cursor", Text: "  One.\nTwo.", Pos: 1, Keys: splitOracleKeys("y/Two<cr>")},
		{Name: "dot repeat search delete at column 0", Text: "xx One.\nTwo.\nThree.", Pos: 3, Keys: splitOracleKeys("d/Two<cr>+.")},
		{Name: "mark delete keeps break at column 0", Text: "xx One.\nTwo.", Pos: 0, Keys: splitOracleKeys("+ma-3ld`a")},
		{Name: "mark change keeps break at column 0", Text: "xx One.\nTwo.", Pos: 0, Keys: splitOracleKeys("+ma-3lc`aQ<esc>")},
		{Name: "mark yank keeps break at column 0", Text: "xx One.\nTwo.", Pos: 0, Keys: splitOracleKeys("+ma-3ly`aP")},
		{Name: "mark delete from line start is linewise", Text: "xx One.\nTwo.", Pos: 0, Keys: splitOracleKeys("+ma-d`a")},
		{Name: "db from column 0 keeps break", Text: "xx One.\nTwo.\n\nThree.", Pos: 13, Keys: splitOracleKeys("db")},
		{Name: "dB from column 0 keeps break", Text: "xx One.\nTwo.", Pos: 8, Keys: splitOracleKeys("dB")},
		{Name: "db onto blank line is linewise", Text: "a\n\nb", Pos: 3, Keys: splitOracleKeys("db")},
		{Name: "dot repeat db at column 0", Text: "One two\nTwo.", Pos: 8, Keys: splitOracleKeys("db.")},
		{Name: "unmatched bracket forward is exclusive", Text: "(a)b", Pos: 1, Keys: splitOracleKeys("d])")},
		{Name: "unmatched bracket change dot", Text: "(a(b)c)d", Pos: 3, Keys: splitOracleKeys("c])Q<esc>.")},
		{Name: "unmatched bracket at column 0 is linewise", Text: "(a\n)b", Pos: 0, Keys: splitOracleKeys("d])")},
		{Name: "unmatched bracket at column 0 keeps break", Text: "a (b\n) c", Pos: 3, Keys: splitOracleKeys("d])")},
		// r6: paragraph motions from a blank line
		{Name: "lbrace from blank lands on blank", Text: "a\n\nb\n\nc", Pos: 5, Keys: splitOracleKeys("{")},
		{Name: "double lbrace reaches start", Text: "a\n\nb\n\nc", Pos: 5, Keys: splitOracleKeys("{{")},
		{Name: "counted lbrace reaches start", Text: "a\n\nb\n\nc", Pos: 5, Keys: splitOracleKeys("2{")},
		{Name: "d lbrace from blank", Text: "a\n\nb\n\nc", Pos: 5, Keys: splitOracleKeys("d{")},
		{Name: "d rbrace from blank keeps one break", Text: "a\n\nb\n\nc", Pos: 2, Keys: splitOracleKeys("d}")},
		{Name: "dot repeat d rbrace from blank", Text: "a\n\nb\n\nc\n\nd", Pos: 2, Keys: splitOracleKeys("d}.")},
		{Name: "c rbrace from blank keeps break", Text: "a\n\nb\n\nc", Pos: 2, Keys: splitOracleKeys("c}Q<esc>")},
		{Name: "y rbrace linewise register", Text: "a\n\nb\n\nc", Pos: 2, Keys: splitOracleKeys("y}p")},
		{Name: "d2 lbrace linewise", Text: "a\n\nb\n\nc", Pos: 6, Keys: splitOracleKeys("d2{")},
		{Name: "c lbrace from column 0 is linewise", Text: "a\n\nb\n\nc", Pos: 6, Keys: splitOracleKeys("c{Q<esc>")},
		{Name: "c rbrace keeps indent", Text: "x\n\n  a\nb\n\nc", Pos: 4, Keys: splitOracleKeys("c}Q<esc>")},
		{Name: "d rbrace from indent is linewise", Text: "  a\n\nb\n\n   c", Pos: 1, Keys: splitOracleKeys("d}")},
		{Name: "d2 rbrace to end is linewise", Text: "x\n  a\n  b\n\n  c", Pos: 4, Keys: splitOracleKeys("d2}")},
		{Name: "dot repeat d rbrace to end", Text: "x\na\nb", Pos: 2, Keys: splitOracleKeys("d}.")},
		{Name: "y rbrace from indent is linewise", Text: "x\n\n  a\nb\n\nc", Pos: 4, Keys: splitOracleKeys("y}p")},
		// r7: shift skips empty lines and clamps its count
		{Name: "counted shift skips empty line", Text: "a\n\nb", Pos: 0, Keys: splitOracleKeys("3>>")},
		{Name: "shift on empty line is a no-op", Text: "a b\n\nc d", Pos: 4, Keys: splitOracleKeys(">>")},
		{Name: "shift to end skips empty line", Text: "a\n\nb", Pos: 0, Keys: splitOracleKeys(">G")},
		{Name: "dot repeat shift skips empty line", Text: "a\n\nb", Pos: 0, Keys: splitOracleKeys(">j.")},
		{Name: "shift count clamps at end", Text: "a\n \nb", Pos: 2, Keys: splitOracleKeys("3>>")},
		{Name: "shift count from first line clamps", Text: "a\n \nb", Pos: 0, Keys: splitOracleKeys("5>>")},
		{Name: "counted shift on last line aborts", Text: "ab\ncd ef", Pos: 6, Keys: splitOracleKeys("3>>")},
		{Name: "counted shift on single line aborts", Text: "word", Pos: 1, Keys: splitOracleKeys("3>>")},
		{Name: "shift on empty last line is a no-op", Text: "One.\n", Pos: 5, Keys: splitOracleKeys(">>")},
		// r8: sentence object at the buffer edges
		{Name: "dis on empty last line", Text: "One.\n", Pos: 5, Keys: splitOracleKeys("dis")},
		{Name: "das on empty last line", Text: "One.\n", Pos: 5, Keys: splitOracleKeys("das")},
		{Name: "cis on empty last line inserts there", Text: "One.\n", Pos: 5, Keys: splitOracleKeys("cisQ<esc>")},
		{Name: "dis on empty last line after blank", Text: "One. \n", Pos: 6, Keys: splitOracleKeys("dis")},
		{Name: "dis on later empty last line", Text: "One.\nTwo.\n", Pos: 10, Keys: splitOracleKeys("dis")},
		{Name: "dis on punctuation only", Text: "!", Pos: 0, Keys: splitOracleKeys("dis")},
		{Name: "cis on punctuation only", Text: "!", Pos: 0, Keys: splitOracleKeys("cisQ<esc>")},
		{Name: "das on punctuation only", Text: ".", Pos: 0, Keys: splitOracleKeys("das")},
		// r9: linewise dot repeat reads the column at repeat time
		{Name: "dot repeat dd column", Text: "abcd\nx\nyz", Pos: 3, Keys: splitOracleKeys("dd.")},
		{Name: "dot repeat dj register", Text: "abcd\nx\nyz\nw", Pos: 3, Keys: splitOracleKeys("dj.")},
		{Name: "dot repeat dj register pastes lines", Text: "abcd\nx\nyz\nw\nq", Pos: 3, Keys: splitOracleKeys("dj.p")},
		{Name: "dot repeat dk register", Text: "a\nb\nc\nd", Pos: 6, Keys: splitOracleKeys("dk.")},
	}
	runOracleDifferential(t, cases, false)
}

func runOracleDifferential(t *testing.T, cases []oracleCase, visual bool) {
	t.Helper()
	if _, err := exec.LookPath("nvim"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("nvim is required in CI so the live differential gates the suite")
		}
		t.Skip("nvim is not installed; deterministic oracle regressions still run")
	}
	for i := range cases {
		before := []rune(cases[i].Text)[:cases[i].Pos]
		prefix := string(before)
		cases[i].Row = strings.Count(prefix, "\n") + 1
		linePrefix := prefix
		if at := strings.LastIndexByte(prefix, '\n'); at >= 0 {
			linePrefix = prefix[at+1:]
		}
		cases[i].Col = len([]byte(linePrefix))
		if visual {
			cases[i].Seq = visualOracleSeq(cases[i].Keys)
		} else {
			cases[i].Seq = strings.Join(cases[i].Keys, "")
		}
	}

	for i, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			want := runOracleCase(t, cases[i], visual)
			e := New(tc.Text)
			e.SetMode(ModeNormal)
			e.SetCursor(tc.Pos)
			for _, key := range tc.Keys {
				e.Feed(oracleKeyName(key))
			}
			if e.Text() != want.Text || e.Cursor() != want.Pos || e.Register() != want.Reg {
				t.Fatalf("vimline=(text %q, cursor %d, reg %q), nvim=(text %q, cursor %d, reg %q)",
					e.Text(), e.Cursor(), e.Register(), want.Text, want.Pos, want.Reg)
			}
		})
	}
}

// runOracleCase drives one case through a fresh nvim process. nvim's
// last-visual-op shape ([count]v reselect) is process-global state that no ex
// or Lua command resets, so cases must not share a process.
func runOracleCase(t *testing.T, tc oracleCase, visual bool) oracleResult {
	t.Helper()
	dir := t.TempDir()
	inPath := filepath.Join(dir, "case.json")
	outPath := filepath.Join(dir, "result.json")
	raw, err := json.Marshal([]oracleCase{tc})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	apply := "  vim.cmd('normal! ' .. c.seq)"
	if visual {
		apply = "  vim.fn.feedkeys(vim.api.nvim_replace_termcodes(c.seq, true, false, true), 'x')"
	}
	lua := `
local input = os.getenv('VIMLINE_ORACLE_INPUT')
local output = os.getenv('VIMLINE_ORACLE_OUTPUT')
local c = vim.json.decode(table.concat(vim.fn.readfile(input), "\n"))[1]
vim.api.nvim_buf_set_lines(0, 0, -1, false, vim.split(c.text, "\n", { plain = true }))
vim.api.nvim_win_set_cursor(0, { c.row, c.col })
vim.fn.setreg('"', '')
vim.cmd('set undolevels=-1')
vim.cmd('set undolevels=1000')
` + apply + `
local cursor = vim.api.nvim_win_get_cursor(0)
local lines = vim.api.nvim_buf_get_lines(0, 0, -1, false)
local pos = 0
for line = 1, cursor[1] - 1 do
  pos = pos + vim.str_utfindex(lines[line]) + 1
end
pos = pos + vim.str_utfindex(lines[cursor[1]]:sub(1, cursor[2]))
local result = { { text = table.concat(lines, "\n"), pos = pos, reg = vim.fn.getreg('"') } }
vim.fn.writefile({ vim.json.encode(result) }, output)
vim.cmd('qa!')
`
	luaPath := filepath.Join(dir, "oracle.lua")
	if err := os.WriteFile(luaPath, []byte(lua), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("nvim", "--headless", "-u", "NONE", "-i", "NONE", "-l", luaPath)
	cmd.Env = append(os.Environ(), "HOME="+dir, "XDG_CONFIG_HOME="+filepath.Join(dir, "config"), "XDG_STATE_HOME="+filepath.Join(dir, "state"), "VIMLINE_ORACLE_INPUT="+inPath, "VIMLINE_ORACLE_OUTPUT="+outPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("nvim oracle: %v\n%s", err, output)
	}
	resultRaw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var results []oracleResult
	if err := json.Unmarshal(resultRaw, &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("oracle returned %d results for 1 case", len(results))
	}
	return results[0]
}

func splitOracleKeys(seq string) []string {
	replacer := strings.NewReplacer(
		"<esc>", "\x1b",
		"<Esc>", "\x1b",
		"<ctrl-r>", "\x12",
		"<C-v>", "\x16",
		"<c-v>", "\x16",
		"<bs>", "\x08",
		"<cr>", "\r",
	)
	return strings.Split(replacer.Replace(seq), "")
}

func oracleKeyName(key string) string {
	switch key {
	case "\x1b":
		return "esc"
	case "\x16":
		return "ctrl-v"
	case "\x12":
		return "ctrl-r"
	case "\x08":
		return "bs"
	case "\r":
		return "enter"
	}
	return key
}
