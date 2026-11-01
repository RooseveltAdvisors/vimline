package vimline

import "testing"

func TestV031OracleRegressions(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		cursor   int
		keys     string
		wantText string
		wantPos  int
	}{
		{
			name: "dot repeats cw without eating separator",
			text: "one (a (b) c) x two x three x four x\nsecond line", cursor: 0,
			keys: "cwQQ<esc>.", wantText: "QQQ (a (b) c) x two x three x four x\nsecond line", wantPos: 2,
		},
		{
			name: "operator underscore is linewise",
			text: "one (two)\nthree four\n\nfive", cursor: 6,
			keys: "d_", wantText: "three four\n\nfive", wantPos: 6,
		},
		{
			name: "change underscore is linewise",
			text: "one (two)\nthree four\n\nfive", cursor: 6,
			keys: "c_Q<esc>", wantText: "Q\nthree four\n\nfive", wantPos: 0,
		},
		{
			name: "change inner paragraph preserves separator",
			text: "one\ntwo\n\nthree", cursor: 2,
			keys: "cipQ<esc>", wantText: "Q\n\nthree", wantPos: 0,
		},
		{
			name: "change around paragraph preserves one newline",
			text: "one\ntwo\n\nthree", cursor: 2,
			keys: "capQ<esc>", wantText: "Q\nthree", wantPos: 0,
		},
		{
			name: "paragraph motion preserves separator",
			text: "one (two)\nthree four\n\nfive", cursor: 6,
			keys: "d}", wantText: "one (t\n\nfive", wantPos: 5,
		},
		{
			name: "counted pair lookup from before opening",
			text: "one (a (b) c) x two x three x four x\nsecond line", cursor: 0,
			keys: "2di(", wantText: "one (a () c) x two x three x four x\nsecond line", wantPos: 8,
		},
		{
			name: "yank text object moves to object start",
			text: "one two three", cursor: 5,
			keys: "yiw", wantText: "one two three", wantPos: 4,
		},
		{
			name: "yank backward find moves to motion start",
			text: "a0 東x b9 京x cx dx", cursor: 15,
			keys: "yFx", wantText: "a0 東x b9 京x cx dx", wantPos: 13,
		},
		{
			name: "character delete clamps before newline",
			text: "one (two)\nthree four\n\nfive", cursor: 6,
			keys: "dg_", wantText: "one (t\nthree four\n\nfive", wantPos: 5,
		},
		{
			name: "linewise delete preserves desired column",
			text: "one (two)\nthree four\n\nfive", cursor: 6,
			keys: "dgg", wantText: "three four\n\nfive", wantPos: 6,
		},
		{
			name: "around paragraph delete preserves desired column",
			text: "one\ntwo\n\nthree", cursor: 2,
			keys: "dap", wantText: "three", wantPos: 2,
		},
		{
			name: "change and insert undo atomically",
			text: "one (a (b) c) x two x three x four x\nsecond line", cursor: 0,
			keys: "cwQQ<esc>u", wantText: "one (a (b) c) x two x three x four x\nsecond line", wantPos: 0,
		},
		{
			name: "dot repeats search delete",
			text: "xx yy xx yy zz", cursor: 2,
			keys: "d/xx<enter>.", wantText: "xx yy zz", wantPos: 0,
		},
		{
			name: "dot repeats search change",
			text: "xx yy xx yy zz", cursor: 2,
			keys: "c/xx<enter>Q<esc>.", wantText: "xxQxx yy zz", wantPos: 2,
		},
		{
			name: "dot repeats mark change",
			text: "one two three", cursor: 0,
			keys: "wmac`aQ<esc>.", wantText: "one QQtwo three", wantPos: 4,
		},
		{
			name: "dot repeat keeps mark at absolute column after delete",
			text: "one two", cursor: 0,
			keys: "wma0c`aQ<esc>.", wantText: "Qo", wantPos: 0,
		},
		{
			name: "mark keeps absolute column after same-line delete",
			text: "one two", cursor: 4,
			keys: "ma0xx`a", wantText: "e two", wantPos: 4,
		},
		{
			name: "mark keeps absolute column after earlier-line delete",
			text: "one\ntwo three", cursor: 6,
			keys: "ma-0xx`a", wantText: "e\ntwo three", wantPos: 4,
		},
		{
			name: "mark-driven change after earlier-line delete replays",
			text: "one\ntwo three", cursor: 6,
			keys: "ma-0xxc`aQ<esc>.", wantText: "Q three", wantPos: 0,
		},
		{
			name: "mark keeps absolute column across line join",
			text: "one\ntwoxyz", cursor: 6,
			keys: "ma-0lld/w<enter>`a", wantText: "onwoxyz", wantPos: 4,
		},
		{
			name: "mark inside deleted span keeps absolute column",
			text: "one two", cursor: 1,
			keys: "ma0d2l`a", wantText: "e two", wantPos: 1,
		},
		{
			name: "over-eol mark survives d dollar then insert",
			text: "one two", cursor: 6,
			keys: "ma0d$iabc<esc>`a", wantText: "abc", wantPos: 2,
		},
		{
			name: "over-eol mark survives dw then insert",
			text: "one two", cursor: 6,
			keys: "ma0dw0iZZZZ<esc>`a", wantText: "ZZZZtwo", wantPos: 6,
		},
		{
			name: "join change replays at over-eol mark",
			text: "one\ntwoxyz", cursor: 9,
			keys: "ma-0lld/o<enter>A23<esc>0c`aQ<esc>", wantText: "Q3", wantPos: 0,
		},
		{
			name: "long interior line delete keeps mark jump",
			text: "ab\nXXXXXXXXXX\ncd", cursor: 12,
			keys: "ma-lde`a", wantText: "a\ncd", wantPos: 0,
		},
		{
			name: "long interior line delete keeps mark column",
			text: "ab\nXXXXXXXXXX\ncd", cursor: 12,
			keys: "ma-lde0i0123456789<esc>`a", wantText: "0123456789a\ncd", wantPos: 10,
		},
		{
			name: "das on whitespace selects following sentence",
			text: "One two. Three four. Five six.", cursor: 8,
			keys: "das", wantText: "One two. Five six.", wantPos: 8,
		},
		{
			name: "das on leading whitespace selects sentence",
			text: " One two. Three four.", cursor: 0,
			keys: "das", wantText: "Three four.", wantPos: 0,
		},
		{
			name: "das on multi space whitespace selects sentence",
			text: "One two.   Three four.", cursor: 10,
			keys: "das", wantText: "One two.", wantPos: 7,
		},
		{
			name: "counted das selects whole sentences",
			text: "One two. Three four. Five six.", cursor: 12,
			keys: "2das", wantText: "One two.", wantPos: 7,
		},
		{
			name: "d2as selects whole sentences",
			text: "One two. Three four. Five six.", cursor: 12,
			keys: "d2as", wantText: "One two.", wantPos: 7,
		},
		{
			name: "counted cas selects whole sentences",
			text: "One two. Three four. Five six.", cursor: 12,
			keys: "c2asQ<esc>", wantText: "One two.Q", wantPos: 8,
		},
		{
			name: "3das clamps to available sentences",
			text: "One two. Three four. Five six.", cursor: 12,
			keys: "3das", wantText: "One two.", wantPos: 7,
		},
		{
			name: "counted dis steps sentence parts",
			text: "One two. Three four. Five six.", cursor: 12,
			keys: "2dis", wantText: "One two. Five six.", wantPos: 9,
		},
		{
			name: "das on last sentence uses leading whitespace",
			text: "One two. Three four. Five six.", cursor: 25,
			keys: "das", wantText: "One two. Three four.", wantPos: 19,
		},
		{
			name: "dis on whitespace selects whitespace run",
			text: "One two. Three four. Five six.", cursor: 8,
			keys: "dis", wantText: "One two.Three four. Five six.", wantPos: 8,
		},
		{
			name: "das on end whitespace is empty object",
			text: "One two.   ", cursor: 9,
			keys: "das", wantText: "One two.   ", wantPos: 8,
		},
		{
			name: "overshot dis keeps sentence end parity",
			text: "One two. ", cursor: 2,
			keys: "3dis", wantText: " ", wantPos: 0,
		},
		{
			name: "das spans multiline sentence",
			text: "One\ntwo here. Three.", cursor: 5,
			keys: "das", wantText: "Three.", wantPos: 0,
		},
		{
			name: "das trailing whitespace stops at blank line",
			text: "One two.\n\nThree.", cursor: 2,
			keys: "das", wantText: "\nThree.", wantPos: 0,
		},
		{
			name: "cas dot repeat reselects sentence",
			text: "One two. Three four. Five six.", cursor: 12,
			keys: "casQ<esc>.", wantText: "One two.Q", wantPos: 8,
		},
		{
			name: "dot repeat of counted das",
			text: "One two. Three four. Five six.", cursor: 12,
			keys: "2das.", wantText: "", wantPos: 0,
		},
		{
			name: "rparen stops at blank line",
			text: "One two.\n\nThree.", cursor: 0,
			keys: ")", wantText: "One two.\n\nThree.", wantPos: 9,
		},
		{
			name: "d rparen deletes to blank line",
			text: "One two.\n\nThree.", cursor: 0,
			keys: "d)", wantText: "\nThree.", wantPos: 0,
		},
		{
			name: "counted rparen stops at second blank",
			text: "One two.\n\nThree. Four.", cursor: 0,
			keys: "2)", wantText: "One two.\n\nThree. Four.", wantPos: 10,
		},
		{
			name: "rparen ordinary",
			text: "One two. Three four.", cursor: 0,
			keys: ")", wantText: "One two. Three four.", wantPos: 9,
		},
		{
			name: "lparen ordinary",
			text: "One two. Three four.", cursor: 12,
			keys: "(", wantText: "One two. Three four.", wantPos: 9,
		},
		{
			name: "counted lparen",
			text: "One two. Three four. Five.", cursor: 12,
			keys: "2(", wantText: "One two. Three four. Five.", wantPos: 0,
		},
		{
			name: "lparen stops at blank line",
			text: "One two.\n\nThree.", cursor: 10,
			keys: "(", wantText: "One two.\n\nThree.", wantPos: 9,
		},
		{
			name: "counted lparen across blank",
			text: "One two.\n\nThree.", cursor: 10,
			keys: "2(", wantText: "One two.\n\nThree.", wantPos: 0,
		},
		{
			name: "d rparen ordinary",
			text: "One two. Three four.", cursor: 0,
			keys: "d)", wantText: "Three four.", wantPos: 0,
		},
		{
			name: "counted d rparen deletes sentences",
			text: "One two. Three four. Five six.", cursor: 0,
			keys: "d2)", wantText: "Five six.", wantPos: 0,
		},
		{
			name: "dot repeat of d rparen",
			text: "One two. Three four. Five six.", cursor: 0,
			keys: "d).", wantText: "Five six.", wantPos: 0,
		},
		{
			name: "rparen at last sentence clamps",
			text: "One two.", cursor: 0,
			keys: ")", wantText: "One two.", wantPos: 7,
		},
		{
			name: "overshot rparen clamps to end",
			text: "One two. Three", cursor: 0,
			keys: "2)", wantText: "One two. Three", wantPos: 13,
		},
		{
			name: "overshot rparen aborts",
			text: "One two. Three", cursor: 0,
			keys: "3)", wantText: "One two. Three", wantPos: 0,
		},
		{
			name: "overshot lparen aborts",
			text: "One two. Three. Four.", cursor: 17,
			keys: "4(", wantText: "One two. Three. Four.", wantPos: 17,
		},
		{
			name: "d rparen at eol deletes to end",
			text: "One two. ", cursor: 0,
			keys: "d)", wantText: "", wantPos: 0,
		},
		{
			name: "cas on trailing whitespace inserts",
			text: "One two.   ", cursor: 9,
			keys: "casQ<esc>", wantText: "One two.Q   ", wantPos: 8,
		},
		{
			name: "cas on empty buffer inserts",
			text: "", cursor: 0,
			keys: "casQ<esc>", wantText: "Q", wantPos: 0,
		},
		{
			name: "cis on empty buffer inserts",
			text: "", cursor: 0,
			keys: "cisQ<esc>", wantText: "Q", wantPos: 0,
		},
		{
			name: "cas dot repeat on trailing whitespace",
			text: "One two.   ", cursor: 9,
			keys: "casQ<esc>.", wantText: "Q", wantPos: 0,
		},
		{
			name: "cas space dot repeat inserts again",
			text: "One two.   ", cursor: 9,
			keys: "cas <esc>.", wantText: "One two.     ", wantPos: 8,
		},
		{
			name: "dot repeats bracket change",
			text: "(a)b(c", cursor: 5,
			keys: "c[(Q<esc>.", wantText: "(a)bQc", wantPos: 4,
		},
		{
			name: "bracket change undoes atomically",
			text: "(a)b(c", cursor: 5,
			keys: "c[(Q<esc>u", wantText: "(a)b(c", wantPos: 4,
		},
		{
			name: "mark operator consumes its count",
			text: "one\ntwo\nthree", cursor: 0,
			keys: "mad2`add", wantText: "two\nthree", wantPos: 0,
		},
		{
			name: "counted dd clamps to remaining lines",
			text: "one\ntwo", cursor: 0,
			keys: "3dd", wantText: "", wantPos: 0,
		},
		{
			name: "counted yy clamps to remaining lines",
			text: "one\ntwo", cursor: 0,
			keys: "3yy", wantText: "one\ntwo", wantPos: 0,
		},
		{
			name: "counted d underscore clamps to remaining lines",
			text: "one\ntwo", cursor: 0,
			keys: "3d_", wantText: "", wantPos: 0,
		},
		{
			name: "2dd aborts on single line",
			text: "one", cursor: 0,
			keys: "2dd", wantText: "one", wantPos: 0,
		},
		{
			name: "2S aborts on single line",
			text: "one", cursor: 0,
			keys: "2SX<esc>", wantText: "one", wantPos: 0,
		},
		{
			name: "d plus aborts on last line",
			text: "one\ntwo", cursor: 4,
			keys: "d+", wantText: "one\ntwo", wantPos: 4,
		},
		{
			name: "d minus aborts on first line",
			text: "one\ntwo", cursor: 0,
			keys: "d-", wantText: "one\ntwo", wantPos: 0,
		},
		{
			name: "dot repeat of counted dd aborts at edge",
			text: "one\ntwo\nthree", cursor: 0,
			keys: "2dd.", wantText: "three", wantPos: 0,
		},
		{
			name: "append esc moves left",
			text: "abcd", cursor: 1,
			keys: "a<esc>x", wantText: "acd", wantPos: 1,
		},
		{
			name: "insert esc moves left",
			text: "abcd", cursor: 2,
			keys: "i<esc>", wantText: "abcd", wantPos: 1,
		},
		{
			name: "replace esc moves left",
			text: "abcd", cursor: 2,
			keys: "R<esc>", wantText: "abcd", wantPos: 1,
		},
		{
			name: "substitute esc moves left",
			text: "abcd", cursor: 2,
			keys: "s<esc>", wantText: "abd", wantPos: 1,
		},
		{
			name: "counted insert esc moves left",
			text: "abcd", cursor: 2,
			keys: "3i<esc>", wantText: "abcd", wantPos: 1,
		},
		{
			name: "dot replay esc stays in column 0",
			text: "ab\ncd\nef", cursor: 3,
			keys: "s<esc>+.", wantText: "ab\nd\nf", wantPos: 5,
		},
		{
			name: "open below esc stays in column 0",
			text: "one\ntwo", cursor: 0,
			keys: "o<esc>", wantText: "one\n\ntwo", wantPos: 4,
		},
		{
			name: "d2l at line end",
			text: "abc", cursor: 1,
			keys: "d2l", wantText: "a", wantPos: 0,
		},
		{
			name: "c2l at line end",
			text: "abc", cursor: 1,
			keys: "c2lQ<esc>", wantText: "aQ", wantPos: 1,
		},
		{
			name: "c3l at line end",
			text: "abcd", cursor: 2,
			keys: "c3lQ<esc>", wantText: "abQ", wantPos: 2,
		},
		{
			name: "dot repeat d2l",
			text: "abc", cursor: 1,
			keys: "d2l.", wantText: "", wantPos: 0,
		},
		{
			name: "d2w single line stays charwise",
			text: "cd ef", cursor: 0,
			keys: "d2wp", wantText: "cd ef", wantPos: 4,
		},
		{
			name: "d2w last line keeps break",
			text: "ab\ncd ef", cursor: 3,
			keys: "d2w", wantText: "ab\n", wantPos: 3,
		},
		{
			name: "d3w across lines is linewise",
			text: "ab\ncd\nef gh", cursor: 3,
			keys: "d3w", wantText: "ab", wantPos: 0,
		},
		{
			name: "d3w linewise keeps column",
			text: "abcdef\n  cd\nef gh", cursor: 9,
			keys: "d3w", wantText: "abcdef", wantPos: 2,
		},
		{
			name: "D on empty line",
			text: "a\n\nb", cursor: 2,
			keys: "D", wantText: "a\n\nb", wantPos: 2,
		},
		{
			name: "C on empty line",
			text: "a\n\nb", cursor: 2,
			keys: "CQ<esc>", wantText: "a\nQ\nb", wantPos: 2,
		},
		{
			name: "dot repeat D on empty line",
			text: "a\n\nb\n\nc", cursor: 2,
			keys: "D.", wantText: "a\n\nb\n\nc", wantPos: 2,
		},
		{
			name: "d2 dollar onto empty line",
			text: "ab\n\nb", cursor: 0,
			keys: "d2$", wantText: "b", wantPos: 0,
		},
		{
			name: "d2 dollar to buffer end is linewise",
			text: "a\nb\nc", cursor: 2,
			keys: "d2$", wantText: "a", wantPos: 0,
		},
		{
			name: "dis keeps break at column 0",
			text: "Hello there. How are you?\nNext line.", cursor: 15,
			keys: "dis", wantText: "Hello there. \nNext line.", wantPos: 12,
		},
		{
			name: "cis keeps break at column 0",
			text: "Hello there. How are you?\nNext line.", cursor: 15,
			keys: "cisQ<esc>", wantText: "Hello there. Q\nNext line.", wantPos: 13,
		},
		{
			name: "das keeps break at column 0",
			text: "Hello there. How are you?\nNext line.", cursor: 15,
			keys: "das", wantText: "Hello there.\nNext line.", wantPos: 11,
		},
		{
			name: "d rparen keeps break at column 0",
			text: "xx One two.\n\nThree.", cursor: 3,
			keys: "d)", wantText: "xx \n\nThree.", wantPos: 2,
		},
		{
			name: "c rparen from line start is linewise",
			text: "One.\nTwo.", cursor: 0,
			keys: "c)Q<esc>", wantText: "Q\nTwo.", wantPos: 0,
		},
		{
			name: "d rparen linewise register pastes lines",
			text: "One.\nTwo.", cursor: 0,
			keys: "d)p", wantText: "Two.\nOne.", wantPos: 5,
		},
		{
			name: "d rparen from indent is linewise",
			text: "  One.\nTwo three.", cursor: 1,
			keys: "d)", wantText: "Two three.", wantPos: 0,
		},
		{
			name: "c rparen linewise keeps indent",
			text: "  One.\nTwo.", cursor: 1,
			keys: "c)Q<esc>", wantText: "  Q\nTwo.", wantPos: 2,
		},
		{
			name: "y rparen linewise keeps cursor",
			text: "  One.\nTwo three.", cursor: 2,
			keys: "y)", wantText: "  One.\nTwo three.", wantPos: 2,
		},
		{
			name: "dis linewise lands on first non-blank",
			text: "  One.\n   Two.", cursor: 2,
			keys: "dis", wantText: "   Two.", wantPos: 3,
		},
		{
			name: "c lparen from column 0 is linewise",
			text: "One.\nTwo.", cursor: 5,
			keys: "c(Q<esc>", wantText: "Q\nTwo.", wantPos: 0,
		},
		{
			name: "dot repeat c rparen linewise",
			text: "One.\nTwo.\nThree.", cursor: 0,
			keys: "c)Q<esc>+.", wantText: "Q\nQ\nThree.", wantPos: 2,
		},
		{
			name: "dot repeat cis linewise",
			text: "One.\nTwo.\nThree.", cursor: 0,
			keys: "cisQ<esc>+.", wantText: "Q\nThree.", wantPos: 0,
		},
		{
			name: "search delete keeps break at column 0",
			text: "xx One.\nTwo.", cursor: 3,
			keys: "d/Two<enter>", wantText: "xx \nTwo.", wantPos: 2,
		},
		{
			name: "search change keeps break at column 0",
			text: "xx One.\nTwo.", cursor: 3,
			keys: "c/Two<enter>Q<esc>", wantText: "xx Q\nTwo.", wantPos: 3,
		},
		{
			name: "search change from line start is linewise",
			text: "One.\nTwo.", cursor: 0,
			keys: "c/Two<enter>Q<esc>", wantText: "Q\nTwo.", wantPos: 0,
		},
		{
			name: "search delete linewise register",
			text: "One.\nTwo.", cursor: 0,
			keys: "d/Two<enter>p", wantText: "Two.\nOne.", wantPos: 5,
		},
		{
			name: "dot repeat search delete at column 0",
			text: "xx One.\nTwo.\nThree.", cursor: 3,
			keys: "d/Two<enter>+.", wantText: "xx \nTwo.\nThree.", wantPos: 4,
		},
		{
			name: "mark delete keeps break at column 0",
			text: "xx One.\nTwo.", cursor: 0,
			keys: "+ma-3ld`a", wantText: "xx \nTwo.", wantPos: 2,
		},
		{
			name: "mark change keeps break at column 0",
			text: "xx One.\nTwo.", cursor: 0,
			keys: "+ma-3lc`aQ<esc>", wantText: "xx Q\nTwo.", wantPos: 3,
		},
		{
			name: "mark yank keeps break at column 0",
			text: "xx One.\nTwo.", cursor: 0,
			keys: "+ma-3ly`aP", wantText: "xx One.One.\nTwo.", wantPos: 6,
		},
		{
			name: "db from column 0 keeps break",
			text: "xx One.\nTwo.\n\nThree.", cursor: 13,
			keys: "db", wantText: "xx One.\nTwo\n\nThree.", wantPos: 10,
		},
		{
			name: "db onto blank line is linewise",
			text: "a\n\nb", cursor: 3,
			keys: "db", wantText: "a\nb", wantPos: 2,
		},
		{
			name: "unmatched bracket forward is exclusive",
			text: "(a)b", cursor: 1,
			keys: "d])", wantText: "()b", wantPos: 1,
		},
		{
			name: "unmatched bracket change dot",
			text: "(a(b)c)d", cursor: 3,
			keys: "c])Q<esc>.", wantText: "(a(Q)c)d", wantPos: 3,
		},
		{
			name: "unmatched bracket at column 0 is linewise",
			text: "(a\n)b", cursor: 0,
			keys: "d])", wantText: ")b", wantPos: 0,
		},
		{
			name: "unmatched bracket at column 0 keeps break",
			text: "a (b\n) c", cursor: 3,
			keys: "d])", wantText: "a (\n) c", wantPos: 2,
		},
		{
			name: "lbrace from blank lands on blank",
			text: "a\n\nb\n\nc", cursor: 5,
			keys: "{", wantText: "a\n\nb\n\nc", wantPos: 2,
		},
		{
			name: "counted lbrace reaches start",
			text: "a\n\nb\n\nc", cursor: 5,
			keys: "2{", wantText: "a\n\nb\n\nc", wantPos: 0,
		},
		{
			name: "d lbrace from blank",
			text: "a\n\nb\n\nc", cursor: 5,
			keys: "d{", wantText: "a\n\nc", wantPos: 2,
		},
		{
			name: "d rbrace from blank keeps one break",
			text: "a\n\nb\n\nc", cursor: 2,
			keys: "d}", wantText: "a\n\nc", wantPos: 2,
		},
		{
			name: "dot repeat d rbrace from blank",
			text: "a\n\nb\n\nc\n\nd", cursor: 2,
			keys: "d}.", wantText: "a\n\nd", wantPos: 2,
		},
		{
			name: "c rbrace from blank keeps break",
			text: "a\n\nb\n\nc", cursor: 2,
			keys: "c}Q<esc>", wantText: "a\nQ\n\nc", wantPos: 2,
		},
		{
			name: "d2 lbrace linewise",
			text: "a\n\nb\n\nc", cursor: 6,
			keys: "d2{", wantText: "a\nc", wantPos: 2,
		},
		{
			name: "c lbrace from column 0 is linewise",
			text: "a\n\nb\n\nc", cursor: 6,
			keys: "c{Q<esc>", wantText: "a\n\nb\nQ\nc", wantPos: 5,
		},
		{
			name: "c rbrace keeps indent",
			text: "x\n\n  a\nb\n\nc", cursor: 4,
			keys: "c}Q<esc>", wantText: "x\n\n  Q\n\nc", wantPos: 5,
		},
		{
			name: "d rbrace from indent is linewise",
			text: "  a\n\nb\n\n   c", cursor: 1,
			keys: "d}", wantText: "\nb\n\n   c", wantPos: 0,
		},
		{
			name: "d2 rbrace to end is linewise",
			text: "x\n  a\n  b\n\n  c", cursor: 4,
			keys: "d2}", wantText: "x", wantPos: 0,
		},
		{
			name: "dot repeat d rbrace to end",
			text: "x\na\nb", cursor: 2,
			keys: "d}.", wantText: "", wantPos: 0,
		},
		{
			name: "counted shift skips empty line",
			text: "a\n\nb", cursor: 0,
			keys: "3>>", wantText: "\ta\n\n\tb", wantPos: 0,
		},
		{
			name: "shift on empty line is a no-op",
			text: "a b\n\nc d", cursor: 4,
			keys: ">>", wantText: "a b\n\nc d", wantPos: 4,
		},
		{
			name: "dot repeat shift skips empty line",
			text: "a\n\nb", cursor: 0,
			keys: ">j.", wantText: "\t\ta\n\nb", wantPos: 0,
		},
		{
			name: "shift count clamps at end",
			text: "a\n \nb", cursor: 2,
			keys: "3>>", wantText: "a\n\t \n\tb", wantPos: 2,
		},
		{
			name: "counted shift on last line aborts",
			text: "ab\ncd ef", cursor: 6,
			keys: "3>>", wantText: "ab\ncd ef", wantPos: 6,
		},
		{
			name: "shift on empty last line is a no-op",
			text: "One.\n", cursor: 5,
			keys: ">>", wantText: "One.\n", wantPos: 5,
		},
		{
			name: "dis on empty last line",
			text: "One.\n", cursor: 5,
			keys: "dis", wantText: "One.\n", wantPos: 5,
		},
		{
			name: "das on empty last line",
			text: "One.\n", cursor: 5,
			keys: "das", wantText: "One.\n", wantPos: 5,
		},
		{
			name: "cis on empty last line inserts there",
			text: "One.\n", cursor: 5,
			keys: "cisQ<esc>", wantText: "One.\nQ", wantPos: 5,
		},
		{
			name: "dis on empty last line after blank",
			text: "One. \n", cursor: 6,
			keys: "dis", wantText: "One.\n", wantPos: 3,
		},
		{
			name: "dis on punctuation only",
			text: "!", cursor: 0,
			keys: "dis", wantText: "", wantPos: 0,
		},
		{
			name: "cis on punctuation only",
			text: "!", cursor: 0,
			keys: "cisQ<esc>", wantText: "Q", wantPos: 0,
		},
		{
			name: "das on punctuation only",
			text: ".", cursor: 0,
			keys: "das", wantText: "", wantPos: 0,
		},
		{
			name: "dot repeat dd column",
			text: "abcd\nx\nyz", cursor: 3,
			keys: "dd.", wantText: "yz", wantPos: 0,
		},
		{
			name: "dot repeat dj register pastes lines",
			text: "abcd\nx\nyz\nw\nq", cursor: 3,
			keys: "dj.p", wantText: "q\nyz\nw", wantPos: 2,
		},
		{
			name: "dot repeat dk register",
			text: "a\nb\nc\nd", cursor: 6,
			keys: "dk.", wantText: "", wantPos: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newNormal(tc.text)
			e.SetCursor(tc.cursor)
			feedSeq(e, tc.keys)
			if got := e.Text(); got != tc.wantText {
				t.Fatalf("text = %q, want %q", got, tc.wantText)
			}
			if got := e.Cursor(); got != tc.wantPos {
				t.Fatalf("cursor = %d, want %d (text=%q)", got, tc.wantPos, e.Text())
			}
		})
	}
}

func TestLogicalEditsAreAtomic(t *testing.T) {
	tests := []struct {
		name string
		keys string
	}{
		{name: "counted delete", keys: "3xu"},
		{name: "counted case toggle", keys: "3~u"},
		{name: "replace mode", keys: "RXYZ<esc>u"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newNormal("abcdef")
			feedSeq(e, tc.keys)
			if got := e.Text(); got != "abcdef" {
				t.Fatalf("text after one undo = %q, want original", got)
			}
			if !e.CanRedo() {
				t.Fatal("atomic undo did not leave one redo entry")
			}
		})
	}
}

func TestOracleRegisterContents(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		cursor   int
		keys     string
		wantText string
		wantReg  string
	}{
		{name: "yiw", text: "one two three", cursor: 5, keys: "yiw", wantText: "one two three", wantReg: "two"},
		{name: "d underscore linewise", text: "one\ntwo", cursor: 1, keys: "d_", wantText: "two", wantReg: "one\n"},
		{name: "change paragraph", text: "one\ntwo\n\nthree", cursor: 2, keys: "cipQ<esc>", wantText: "Q\n\nthree", wantReg: "one\ntwo\n"},
		{name: "dot repeat search change register", text: "xx yy xx yy zz", cursor: 2, keys: "c/xx<enter>Q<esc>.", wantText: "xxQxx yy zz", wantReg: "Q"},
		{name: "counted dd clamps register", text: "one\ntwo", cursor: 0, keys: "3dd", wantText: "", wantReg: "one\ntwo\n"},
		{name: "bracket change dot register", text: "(a)b(c", cursor: 5, keys: "c[(Q<esc>.", wantText: "(a)bQc", wantReg: "("},
		{name: "das on whitespace register", text: "One two. Three four. Five six.", cursor: 8, keys: "das", wantText: "One two. Five six.", wantReg: " Three four."},
		{name: "das leading whitespace register", text: " One two. Three four.", cursor: 0, keys: "das", wantText: "Three four.", wantReg: " One two. "},
		{name: "counted das register", text: "One two. Three four. Five six.", cursor: 12, keys: "2das", wantText: "One two.", wantReg: " Three four. Five six."},
		{name: "das end whitespace register untouched", text: "One two.   ", cursor: 9, keys: "das", wantText: "One two.   ", wantReg: ""},
		{name: "das blank line register", text: "One two.\n\nThree.", cursor: 2, keys: "das", wantText: "\nThree.", wantReg: "One two.\n"},
		{name: "cas dot repeat register", text: "One two. Three four. Five six.", cursor: 12, keys: "casQ<esc>.", wantText: "One two.Q", wantReg: " QFive six."},
		{name: "d rparen blank line register", text: "One two.\n\nThree.", cursor: 0, keys: "d)", wantText: "\nThree.", wantReg: "One two.\n"},
		{name: "y rparen blank line register", text: "One two.\n\nThree.", cursor: 0, keys: "y)", wantText: "One two.\n\nThree.", wantReg: "One two.\n"},
		{name: "d rparen ordinary register", text: "One two. Three four.", cursor: 0, keys: "d)", wantText: "Three four.", wantReg: "One two. "},
		{name: "dot repeat d rparen register", text: "One two. Three four. Five six.", cursor: 0, keys: "d).", wantText: "Five six.", wantReg: "Three four. "},
		{name: "cas dot repeat empty register", text: "One two.   ", cursor: 9, keys: "casQ<esc>.", wantText: "Q", wantReg: "One two.Q   "},
		{name: "d2l register", text: "abc", cursor: 1, keys: "d2l", wantText: "a", wantReg: "bc"},
		{name: "y2l register", text: "abc", cursor: 1, keys: "y2l", wantText: "abc", wantReg: "bc"},
		{name: "d3w linewise register", text: "ab\ncd\nef gh", cursor: 3, keys: "d3w", wantText: "ab", wantReg: "cd\nef gh\n"},
		{name: "d2w last line register", text: "ab\ncd ef", cursor: 3, keys: "d2w", wantText: "ab\n", wantReg: "cd ef"},
		{name: "y dollar empty line register", text: "a\n\nb", cursor: 2, keys: "y$", wantText: "a\n\nb", wantReg: ""},
		{name: "d2 dollar linewise register", text: "ab\n\nb", cursor: 0, keys: "d2$", wantText: "b", wantReg: "ab\n\n"},
		{name: "dis column 0 register", text: "Hello there. How are you?\nNext line.", cursor: 15, keys: "dis", wantText: "Hello there. \nNext line.", wantReg: "How are you?"},
		{name: "d rparen column 0 register", text: "xx One two.\n\nThree.", cursor: 3, keys: "d)", wantText: "xx \n\nThree.", wantReg: "One two."},
		{name: "y rparen linewise register", text: "  One.\nTwo three.", cursor: 1, keys: "y)", wantText: "  One.\nTwo three.", wantReg: "  One.\n"},
		{name: "search delete column 0 register", text: "xx One.\nTwo.", cursor: 3, keys: "d/Two<enter>", wantText: "xx \nTwo.", wantReg: "One."},
		{name: "mark yank column 0 register", text: "xx One.\nTwo.", cursor: 0, keys: "+ma-3ly`a", wantText: "xx One.\nTwo.", wantReg: "One."},
		{name: "y rbrace from indent register", text: "x\n\n  a\nb\n\nc", cursor: 4, keys: "y}", wantText: "x\n\n  a\nb\n\nc", wantReg: "  a\nb\n"},
		{name: "dot repeat d rbrace register", text: "a\n\nb\n\nc\n\nd", cursor: 2, keys: "d}.", wantText: "a\n\nd", wantReg: "\nc\n"},
		{name: "dot repeat dk register", text: "a\nb\nc\nd", cursor: 6, keys: "dk.", wantText: "", wantReg: "a\nb\n"},
		{name: "dot repeat dj register", text: "abcd\nx\nyz\nw", cursor: 3, keys: "dj.", wantText: "", wantReg: "yz\nw\n"},
		{name: "unmatched bracket exclusive register", text: "(a)b", cursor: 1, keys: "d])", wantText: "()b", wantReg: "a"},
		{name: "d lbrace from blank register", text: "a\n\nb\n\nc", cursor: 5, keys: "d{", wantText: "a\n\nc", wantReg: "\nb\n"},
		{name: "db column 0 register", text: "xx One.\nTwo.\n\nThree.", cursor: 13, keys: "db", wantText: "xx One.\nTwo\n\nThree.", wantReg: "."},
		{name: "dot repeat c rparen linewise register", text: "One.\nTwo.\nThree.", cursor: 0, keys: "c)Q<esc>+.", wantText: "Q\nQ\nThree.", wantReg: "Two.\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newNormal(tc.text)
			e.SetCursor(tc.cursor)
			feedSeq(e, tc.keys)
			if got := e.Text(); got != tc.wantText {
				t.Fatalf("text = %q, want %q", got, tc.wantText)
			}
			if got := e.Register(); got != tc.wantReg {
				t.Fatalf("register = %q, want %q", got, tc.wantReg)
			}
		})
	}
}
