package vimline

import (
	"testing"
)

func feedVisualSeq(e *Editor, seq string) {
	for _, key := range splitOracleKeys(seq) {
		feedOracleKey(e, oracleKeyName(key))
	}
}

func newVisualAt(text string, pos int) *Editor {
	e := New(text)
	e.SetMode(ModeNormal)
	e.SetCursor(pos)
	return e
}

func TestVisualCharEnterSelectsUnderCursor(t *testing.T) {
	e := newVisualAt("abc", 1)
	e.Feed("v")
	if e.Mode() != ModeVisual {
		t.Fatalf("mode = %v", e.Mode())
	}
	v := e.View("/", true)
	if !v.SelActive || v.SelStart != 1 || v.SelEnd != 1 || v.VisualKind != VisualChar {
		t.Fatalf("view sel = %+v", v)
	}
}

func TestVisualEscapeClearsWithoutMutation(t *testing.T) {
	e := newVisualAt("abc", 0)
	beforeUndo := len(e.undo)
	feedVisualSeq(e, "vll")
	text := e.Text()
	feedVisualSeq(e, "<esc>")
	if e.Mode() != ModeNormal {
		t.Fatalf("mode = %v", e.Mode())
	}
	if e.Text() != text {
		t.Fatalf("text mutated: %q", e.Text())
	}
	if len(e.undo) != beforeUndo {
		t.Fatalf("undo grew: %d -> %d", beforeUndo, len(e.undo))
	}
	if e.View("/", true).SelActive {
		t.Fatal("selection still active")
	}
}

func TestVisualGVReselects(t *testing.T) {
	e := newVisualAt("abcdef", 0)
	feedVisualSeq(e, "vll<esc>")
	if e.Mode() != ModeNormal {
		t.Fatal("expected normal after esc")
	}
	if a := e.Feed("g"); a != ActFirst {
		t.Fatalf("g action = %v", a)
	}
	e.Feed("v")
	if e.Mode() != ModeVisual {
		t.Fatalf("gv mode = %v", e.Mode())
	}
	v := e.View("/", true)
	if !v.SelActive || v.SelStart != 0 || v.SelEnd != 2 {
		t.Fatalf("gv sel = start %d end %d active %v", v.SelStart, v.SelEnd, v.SelActive)
	}
}

func TestVisualEndpointToggle(t *testing.T) {
	e := newVisualAt("abcdef", 0)
	feedVisualSeq(e, "vll")
	if e.Cursor() != 2 {
		t.Fatalf("cursor before o = %d", e.Cursor())
	}
	e.Feed("o")
	if e.Cursor() != 0 {
		t.Fatalf("cursor after o = %d", e.Cursor())
	}
	e.Feed("O")
	if e.Cursor() != 2 {
		t.Fatalf("cursor after O = %d", e.Cursor())
	}
}

func TestVisualInnerQuoteDelete(t *testing.T) {
	e := newVisualAt(`one "two" three`, 5)
	feedVisualSeq(e, `vi"d`)
	if e.Text() != `one "" three` {
		t.Fatalf("text = %q", e.Text())
	}
	if e.Register() != "two" {
		t.Fatalf("reg = %q", e.Register())
	}
	if e.Mode() != ModeNormal {
		t.Fatalf("mode = %v", e.Mode())
	}
}

func TestVisualAroundWordDelete(t *testing.T) {
	e := newVisualAt("  hello x", 2)
	feedVisualSeq(e, "vawd")
	if e.Text() != "  x" {
		t.Fatalf("text = %q", e.Text())
	}
}

func TestVisualLineDelete(t *testing.T) {
	e := newVisualAt("one\ntwo\nthree", 0)
	feedVisualSeq(e, "Vjd")
	if e.Text() != "three" {
		t.Fatalf("text = %q", e.Text())
	}
	if e.Register() != "one\ntwo\n" {
		t.Fatalf("reg = %q", e.Register())
	}
}

func TestVisualOperatorsCaseAndYank(t *testing.T) {
	e := newVisualAt("one two", 0)
	feedVisualSeq(e, "veU")
	if e.Text() != "ONE two" {
		t.Fatalf("U text = %q", e.Text())
	}
	e = newVisualAt("Ab Cd", 0)
	feedVisualSeq(e, "veu")
	if e.Text() != "ab Cd" {
		t.Fatalf("u text = %q", e.Text())
	}
	e = newVisualAt("one two", 0)
	feedVisualSeq(e, "ve~")
	if e.Text() != "ONE two" {
		t.Fatalf("~ text = %q", e.Text())
	}
	e = newVisualAt("hello world", 0)
	feedVisualSeq(e, "vllly")
	if e.Register() != "hell" {
		t.Fatalf("yank reg = %q", e.Register())
	}
	if e.Text() != "hello world" {
		t.Fatalf("yank mutated text: %q", e.Text())
	}
}

func TestVisualChangeEntersInsert(t *testing.T) {
	e := newVisualAt("one two", 4)
	feedVisualSeq(e, "viwcX")
	if e.Mode() != ModeInsert {
		t.Fatalf("mode = %v", e.Mode())
	}
	if e.Text() != "one X" {
		t.Fatalf("text = %q", e.Text())
	}
}

func TestVisualBlockYank(t *testing.T) {
	e := newVisualAt("hello world", 0)
	feedVisualSeq(e, "<C-v>lly")
	if e.Register() != "hel" {
		t.Fatalf("reg = %q", e.Register())
	}
}

func TestVisualJoin(t *testing.T) {
	e := newVisualAt("a\nb\nc", 0)
	feedVisualSeq(e, "VjJ")
	if e.Text() != "a b\nc" {
		t.Fatalf("text = %q", e.Text())
	}
}

func TestVisualSentenceAndParagraph(t *testing.T) {
	e := newVisualAt("One two. Three.", 0)
	feedVisualSeq(e, "vasd")
	if e.Text() != "Three." {
		t.Fatalf("vasd text = %q", e.Text())
	}
	e = newVisualAt("p1\n\np2", 0)
	feedVisualSeq(e, "vipd")
	if e.Text() != "\np2" {
		t.Fatalf("vipd text = %q", e.Text())
	}
}

func TestVisualBracketObject(t *testing.T) {
	e := newVisualAt("(ab)", 1)
	feedVisualSeq(e, "vi)d")
	if e.Text() != "()" {
		t.Fatalf("text = %q", e.Text())
	}
}

func TestBareGStillActFirst(t *testing.T) {
	e := newVisualAt("abc", 0)
	if a := e.Feed("g"); a != ActFirst {
		t.Fatalf("g = %v", a)
	}
	if e.Mode() != ModeNormal {
		t.Fatalf("mode = %v", e.Mode())
	}
}

func TestVisualDollarDeletesNewline(t *testing.T) {
	e := newVisualAt("abc\ndef", 0)
	feedVisualSeq(e, "v$d")
	if e.Text() != "def" || e.Register() != "abc\n" {
		t.Fatalf("text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestVisualEmptyInnerQuote(t *testing.T) {
	e := newVisualAt(`a""b`, 1)
	feedVisualSeq(e, `vi"d`)
	if e.Text() != "ab" || e.Register() != `""` {
		t.Fatalf("text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestVisualBlockChangeReplicates(t *testing.T) {
	e := newVisualAt("abcd\nefgh", 0)
	feedVisualSeq(e, "<C-v>jlcX<esc>")
	if e.Text() != "Xcd\nXgh" {
		t.Fatalf("text=%q", e.Text())
	}
}

func TestVisualLineDeleteKeepsColumn(t *testing.T) {
	e := newVisualAt("abcdef\nxyz", 3)
	feedVisualSeq(e, "Vd")
	if e.Text() != "xyz" {
		t.Fatalf("text=%q", e.Text())
	}
	if e.Cursor() != 2 {
		t.Fatalf("cursor=%d want 2", e.Cursor())
	}
}

func TestVisualBlockDollarExtendsEOL(t *testing.T) {
	e := newVisualAt("abc\nde\nfgh", 1)
	feedVisualSeq(e, "<C-v>j$d")
	if e.Text() != "a\nd\nfgh" {
		t.Fatalf("text=%q", e.Text())
	}
	if e.Register() != "bc\ne" {
		t.Fatalf("reg=%q", e.Register())
	}
}

func TestVisualBlockDollarThenH(t *testing.T) {
	e := newVisualAt("abc\nde\nfgh", 1)
	feedVisualSeq(e, "<C-v>j$hd")
	if e.Text() != "ac\nd\nfgh" || e.Register() != "b\ne" {
		t.Fatalf("text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestVisualBlockDollarFromHighCol(t *testing.T) {
	e := newVisualAt("abcdef\ngh\nijkl", 2)
	feedVisualSeq(e, "<C-v>j$d")
	if e.Text() != "ab\ngh\nijkl" || e.Register() != "cdef\n" {
		t.Fatalf("text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestVisualCharCrossLineDollar(t *testing.T) {
	e := newVisualAt("abc\ndef", 5) // on 'e'
	feedVisualSeq(e, "vk$d")
	if e.Text() != "abcf" || e.Register() != "\nde" {
		t.Fatalf("text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestVisualChangeUndoAtomic(t *testing.T) {
	e := newVisualAt("one two", 4)
	feedVisualSeq(e, "viwcX<esc>")
	if e.Text() != "one X" {
		t.Fatalf("text = %q", e.Text())
	}
	e.Feed("u")
	if e.Text() != "one two" {
		t.Fatalf("after undo text = %q", e.Text())
	}
	if e.CanUndo() {
		t.Fatal("visual change must be one undo step")
	}
}

func TestVisualBlockChangeUndoAtomic(t *testing.T) {
	e := newVisualAt("abcd\nefgh", 0)
	feedVisualSeq(e, "<C-v>jlcX<esc>")
	if e.Text() != "Xcd\nXgh" {
		t.Fatalf("text = %q", e.Text())
	}
	e.Feed("u")
	if e.Text() != "abcd\nefgh" {
		t.Fatalf("after undo text = %q", e.Text())
	}
	if e.CanUndo() {
		t.Fatal("blockwise change must be one undo step")
	}
}

func TestVisualBlockChangeRaggedLines(t *testing.T) {
	e := newVisualAt("abcd\nefgh\nz\nijkl", 2)
	feedVisualSeq(e, "<C-v>ljjjcXY<esc>")
	if e.Text() != "abXY\nefXY\nz\nijXY" {
		t.Fatalf("text = %q", e.Text())
	}
	if e.Register() != "cd\ngh\n  \nkl" {
		t.Fatalf("reg = %q", e.Register())
	}
}

func TestVisualDollarBlankLineSpan(t *testing.T) {
	e := newVisualAt("line1\n\nline2", 0)
	feedVisualSeq(e, "v$d")
	if e.Text() != "\nline2" || e.Register() != "line1\n" {
		t.Fatalf("text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestVisualGotoCount(t *testing.T) {
	e := newVisualAt("aaa\nbbb\nccc\nddd", 9)
	feedVisualSeq(e, "v2ggd")
	if e.Text() != "aaa\nbc\nddd" || e.Register() != "bb\ncc" || e.Cursor() != 5 {
		t.Fatalf("text=%q reg=%q cursor=%d", e.Text(), e.Register(), e.Cursor())
	}
}

func TestVisualWordEndBackCount(t *testing.T) {
	e := newVisualAt("a b c d e", 8)
	feedVisualSeq(e, "v3ge")
	if e.Cursor() != 2 {
		t.Fatalf("cursor = %d want 2", e.Cursor())
	}
}

func TestVisualIndentRepeat(t *testing.T) {
	e := newVisualAt("a\nb\nc\nd\ne", 0)
	feedVisualSeq(e, "Vj>..")
	if e.Text() != "\t\t\ta\n\t\t\tb\nc\nd\ne" {
		t.Fatalf("text = %q", e.Text())
	}
}

func TestVisualIndentRepeatSurvivesShrink(t *testing.T) {
	e := newVisualAt("a\nb\nc\nd", 4)
	feedVisualSeq(e, "Vj>VdVd.")
	if e.Text() != "a" || e.Register() != "b\n" {
		t.Fatalf("text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestVisualBlockChangeDotRepeatAnchorsAtCursor(t *testing.T) {
	e := newVisualAt("abcd\nefgh", 0)
	feedVisualSeq(e, "<C-v>jlcXY<esc>.")
	if e.Text() != "XXYd\nXXYh" {
		t.Fatalf("text = %q", e.Text())
	}
	if e.Register() != "Yc\nYg" {
		t.Fatalf("reg = %q", e.Register())
	}
	if e.Cursor() != 2 {
		t.Fatalf("cursor = %d want 2", e.Cursor())
	}
}

func TestVisualBlockDeleteDotRepeatAnchorsAtCursor(t *testing.T) {
	e := newVisualAt("abcd\nefgh\nijkl\nmnop", 0)
	feedVisualSeq(e, "<C-v>ljldjjll.")
	if e.Text() != "d\nh\nij\nmn" {
		t.Fatalf("text = %q", e.Text())
	}
	if e.Register() != "kl\nop" {
		t.Fatalf("reg = %q", e.Register())
	}
	if e.Cursor() != 5 {
		t.Fatalf("cursor = %d want 5", e.Cursor())
	}
}

func TestVisualBlockCaseDotRepeatAnchorsAtCursor(t *testing.T) {
	e := newVisualAt("abcd\nefgh\nijkl\nmnop", 0)
	feedVisualSeq(e, "<C-v>lj~jjll.")
	if e.Text() != "ABcd\nEFgh\nijKL\nmnOP" {
		t.Fatalf("text = %q", e.Text())
	}
	if e.Cursor() != 12 {
		t.Fatalf("cursor = %d want 12", e.Cursor())
	}
}

func TestVisualLineTextObjectAdoptsCharwise(t *testing.T) {
	e := newVisualAt("one two three", 4)
	feedVisualSeq(e, "Viwd")
	if e.Text() != "one  three" || e.Register() != "two" {
		t.Fatalf("text=%q reg=%q", e.Text(), e.Register())
	}
	e = newVisualAt("aa (bb) cc", 4)
	feedVisualSeq(e, "Vi)d")
	if e.Text() != "aa () cc" || e.Register() != "bb" {
		t.Fatalf("text=%q reg=%q", e.Text(), e.Register())
	}
	e = newVisualAt("One two.\nThree four.", 0)
	feedVisualSeq(e, "Visd")
	if e.Text() != "\nThree four." || e.Register() != "One two." {
		t.Fatalf("text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestVisualBlockChangeDropsReplicationOnLineEdits(t *testing.T) {
	e := newVisualAt("zzzz\naaaa\nbbbb\ncccc", 5)
	feedVisualSeq(e, "<C-v>jjlcX<bs><bs>YZ<esc>")
	if e.Text() != "zzzzYZaa\nbb\ncc" {
		t.Fatalf("text = %q", e.Text())
	}
	if e.Register() != "aa\nbb\ncc" {
		t.Fatalf("reg = %q", e.Register())
	}
	if e.Cursor() != 5 {
		t.Fatalf("cursor = %d want 5", e.Cursor())
	}
}

func TestVisualBlockChangeReplicatesTypedOnlyChurn(t *testing.T) {
	e := newVisualAt("abcd\nefgh\nijkl", 0)
	feedVisualSeq(e, "<C-v>jjlcXY")
	e.Feed("ctrl-u")
	feedVisualSeq(e, "Z<esc>")
	if e.Text() != "Zcd\nZgh\nZkl" {
		t.Fatalf("text = %q", e.Text())
	}
	if e.Register() != "ab\nef\nij" {
		t.Fatalf("reg = %q", e.Register())
	}
}

func TestVisualCountDroppedOnToggleAndCancel(t *testing.T) {
	e := newVisualAt("abcdef", 0)
	feedVisualSeq(e, "v2old")
	if e.Text() != "cdef" || e.Register() != "ab" {
		t.Fatalf("o toggle text=%q reg=%q", e.Text(), e.Register())
	}
	e = newVisualAt("abcdef", 0)
	feedVisualSeq(e, "v2vlx")
	if e.Text() != "acdef" || e.Register() != "b" {
		t.Fatalf("v toggle text=%q reg=%q", e.Text(), e.Register())
	}
	e = newVisualAt("abcdef", 0)
	feedVisualSeq(e, "v2f<esc>lx")
	if e.Text() != "cdef" || e.Register() != "ab" {
		t.Fatalf("find cancel text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestVisualBlockGotoKeepsVirtualColumn(t *testing.T) {
	e := newVisualAt("abcd\nwxyz12\nef", 2)
	feedVisualSeq(e, "<C-v>Gd")
	if e.Text() != "abd\nwxz12\nef" || e.Register() != "c\ny\n" {
		t.Fatalf("G text=%q reg=%q", e.Text(), e.Register())
	}
	e = newVisualAt("abcd\nef\nwxyz12", 2)
	feedVisualSeq(e, "<C-v>2ggd")
	if e.Text() != "abd\nef\nwxyz12" || e.Register() != "c\n" {
		t.Fatalf("gg text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestViewBlockwisePerRowSpans(t *testing.T) {
	e := newVisualAt("abcd\nefgh", 0)
	feedVisualSeq(e, "<C-v>l")
	v := e.View("/", true)
	if !v.SelActive || v.SelStart != 0 || v.SelEnd != 6 {
		t.Fatalf("flat sel active=%v start=%d end=%d", v.SelActive, v.SelStart, v.SelEnd)
	}
	want := [][2]int{{0, 1}, {5, 6}}
	if len(v.SelRows) != len(want) {
		t.Fatalf("rows = %v want %v", v.SelRows, want)
	}
	for i := range want {
		if v.SelRows[i] != want[i] {
			t.Fatalf("rows = %v want %v", v.SelRows, want)
		}
	}
}

func TestVisualCountedEntryExtent(t *testing.T) {
	e := newVisualAt("abcdef", 0)
	feedVisualSeq(e, "2vld")
	if e.Text() != "def" || e.Register() != "abc" {
		t.Fatalf("charwise text=%q reg=%q", e.Text(), e.Register())
	}
	e = newVisualAt("abcdef", 0)
	feedVisualSeq(e, "2vy")
	if e.Register() != "ab" {
		t.Fatalf("charwise yank reg=%q", e.Register())
	}
	e = newVisualAt("a\nb\nc\nd", 0)
	feedVisualSeq(e, "2Vjd")
	if e.Text() != "d" || e.Register() != "a\nb\nc\n" {
		t.Fatalf("linewise text=%q reg=%q", e.Text(), e.Register())
	}
	e = newVisualAt("abcd\nefgh", 0)
	feedVisualSeq(e, "2<C-v>jd")
	if e.Text() != "cd\ngh" || e.Register() != "ab\nef" {
		t.Fatalf("blockwise text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestVisualCountedStepsStopAtLineEnd(t *testing.T) {
	e := newVisualAt("abc\ndef", 1)
	feedVisualSeq(e, "v2ld")
	if e.Text() != "adef" || e.Register() != "bc\n" {
		t.Fatalf("counted l text=%q reg=%q", e.Text(), e.Register())
	}
	e = newVisualAt("abc\ndef", 0)
	feedVisualSeq(e, "5vd")
	if e.Text() != "def" || e.Register() != "abc\n" {
		t.Fatalf("entry steps text=%q reg=%q", e.Text(), e.Register())
	}
	e = newVisualAt("abc\ndef\nghi", 0)
	feedVisualSeq(e, "v2$y")
	if e.Register() != "abc\ndef\n" {
		t.Fatalf("counted dollar reg=%q", e.Register())
	}
	e = newVisualAt("abcd\nefgh", 0)
	feedVisualSeq(e, "<C-v>j$2hd")
	if e.Text() != "d\nh" || e.Register() != "abc\nefg" {
		t.Fatalf("block dollar-h text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestVisualCountedEntryRepeatsLastOperation(t *testing.T) {
	e := newVisualAt("abcdefghijkl", 0)
	feedVisualSeq(e, "3vdl2vd")
	if e.Text() != "dkl" || e.Register() != "efghij" {
		t.Fatalf("charwise repeat text=%q reg=%q", e.Text(), e.Register())
	}
	e = newVisualAt("abcd\nefgh\nijkl\nmnop\nqrst\nuvwx", 0)
	feedVisualSeq(e, "<C-v>jlyjjl2<C-v>d")
	if e.Text() != "abcd\nefgh\ni\nm\nq\nu" || e.Register() != "jkl\nnop\nrst\nvwx" {
		t.Fatalf("block repeat text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestVisualBlockOperatorAnchorsAtTopRow(t *testing.T) {
	e := newVisualAt("ab\nlongish", 8)
	feedVisualSeq(e, "<C-v>kcXY<esc>")
	if e.Text() != "abXY\nloXYh" {
		t.Fatalf("text = %q", e.Text())
	}
	if e.Register() != "\nngis" {
		t.Fatalf("reg = %q", e.Register())
	}
	e = newVisualAt("ab\nlongish", 8)
	feedVisualSeq(e, "<C-v>ky")
	if e.Cursor() != 1 {
		t.Fatalf("cursor = %d want 1", e.Cursor())
	}
}

func TestVisualRaggedBlockLeftEdge(t *testing.T) {
	e := newVisualAt("ab\nlongish", 8)
	feedVisualSeq(e, "<C-v>kd")
	if e.Text() != "ab\nloh" || e.Register() != "\nngis" {
		t.Fatalf("d text = %q reg = %q", e.Text(), e.Register())
	}
	e = newVisualAt("ab\nlongish", 8)
	feedVisualSeq(e, "<C-v>ky")
	if e.Register() != "\nngis" {
		t.Fatalf("y reg = %q", e.Register())
	}
	e = newVisualAt("ab\nlongish", 8)
	feedVisualSeq(e, "<C-v>k~")
	if e.Text() != "ab\nloNGISh" {
		t.Fatalf("tilde text = %q", e.Text())
	}
	e = newVisualAt("abc\nlongish", 9)
	feedVisualSeq(e, "<C-v>ky")
	if e.Register() != "\ngis" {
		t.Fatalf("edge reg = %q", e.Register())
	}
}

func TestVisualCountedEntryMultilineCharwise(t *testing.T) {
	e := newVisualAt("ab\ncd\nef\ngh\nij\nkl\nmn\nop", 0)
	feedVisualSeq(e, "vjd2vd")
	if e.Text() != "j\nkl\nmn\nop" || e.Register() != "d\nef\ngh\ni" {
		t.Fatalf("text = %q reg = %q", e.Text(), e.Register())
	}
	e = newVisualAt("abc\ndef\nghi\njkl\nmno\npqr\nstu", 0)
	feedVisualSeq(e, "vjld2vd")
	if e.Text() != "o\npqr\nstu" || e.Register() != "f\nghi\njkl\nmn" {
		t.Fatalf("text = %q reg = %q", e.Text(), e.Register())
	}
}

func TestVisualTextObjectExtendsSelection(t *testing.T) {
	e := newVisualAt("one two\nthree four", 0)
	feedVisualSeq(e, "Vjiwd")
	if e.Text() != " four" || e.Register() != "one two\nthree" {
		t.Fatalf("text = %q reg = %q", e.Text(), e.Register())
	}
	e = newVisualAt("one two three", 4)
	feedVisualSeq(e, "vhiwd")
	if e.Text() != "wo three" || e.Register() != "one t" {
		t.Fatalf("text = %q reg = %q", e.Text(), e.Register())
	}
}

func TestVisualTextObjectUnknownNoops(t *testing.T) {
	e := newVisualAt("abcd", 0)
	feedVisualSeq(e, "v2l")
	feedVisualSeq(e, "i)")
	if e.Mode() != ModeVisual {
		t.Fatalf("mode after unknown object = %v", e.Mode())
	}
	v := e.View("/", true)
	if !v.SelActive || v.SelStart != 0 || v.SelEnd != 2 {
		t.Fatalf("sel after unknown object = %+v", v)
	}
	feedVisualSeq(e, "ib")
	if e.Mode() != ModeVisual {
		t.Fatalf("mode after unmatched object = %v", e.Mode())
	}
	v = e.View("/", true)
	if !v.SelActive || v.SelStart != 0 || v.SelEnd != 2 {
		t.Fatalf("sel after unmatched object = %+v", v)
	}
	feedVisualSeq(e, "<esc>")
	if e.Text() != "abcd" {
		t.Fatalf("text = %q", e.Text())
	}
}

func TestVisualBlockTextObjectKeepsBlockwise(t *testing.T) {
	e := newVisualAt("abcd\nefgh", 0)
	feedVisualSeq(e, "<C-v>liwd")
	if e.Text() != "\nefgh" || e.Register() != "abcd" {
		t.Fatalf("text = %q reg = %q", e.Text(), e.Register())
	}
	e = newVisualAt("abcd\nefgh", 0)
	feedVisualSeq(e, "<C-v>liwyj0P")
	if e.Text() != "abcd\nabcdefgh" {
		t.Fatalf("blockwise put after object yank text = %q", e.Text())
	}
}

func TestBlockwisePutColumnAligned(t *testing.T) {
	e := newVisualAt("abcd\nefgh\nwxyz\nqrst", 0)
	feedVisualSeq(e, "<C-v>jlyjj0P")
	if e.Text() != "abcd\nefgh\nabwxyz\nefqrst" {
		t.Fatalf("P text = %q", e.Text())
	}
	if e.Cursor() != 10 {
		t.Fatalf("P cursor = %d want 10", e.Cursor())
	}
	e = newVisualAt("abcd\nefgh\nwxyz\nqrst", 0)
	feedVisualSeq(e, "<C-v>jlyjj0p")
	if e.Text() != "abcd\nefgh\nwabxyz\nqefrst" {
		t.Fatalf("p text = %q", e.Text())
	}
	if e.Cursor() != 11 {
		t.Fatalf("p cursor = %d want 11", e.Cursor())
	}
	e = newVisualAt("abcd\nefgh", 0)
	feedVisualSeq(e, "<C-v>jlyG$p")
	if e.Text() != "abcd\nefghab\n    ef" {
		t.Fatalf("p at buffer end text = %q", e.Text())
	}
	e = newVisualAt("abcd\nefgh\n\nqrst", 0)
	feedVisualSeq(e, "<C-v>jlyjj0p")
	if e.Text() != "abcd\nefgh\nab\nefqrst" {
		t.Fatalf("p on empty cursor line text = %q", e.Text())
	}
	if e.Cursor() != 10 {
		t.Fatalf("p on empty cursor line cursor = %d want 10", e.Cursor())
	}
	e = newVisualAt("abcd\nefgh\n\nqrst", 0)
	feedVisualSeq(e, "<C-v>jlyjj02p")
	if e.Text() != "abcd\nefgh\nabab\nefefqrst" {
		t.Fatalf("counted p on empty cursor line text = %q", e.Text())
	}
}

func TestBlockwisePutEmptyRowNotPaddedAtBufferEnd(t *testing.T) {
	e := newVisualAt("abc\nde\nz", 2)
	feedVisualSeq(e, "<C-v>jyG0P")
	if e.Text() != "abc\nde\ncz\n" {
		t.Fatalf("P text = %q", e.Text())
	}
	e = newVisualAt("abc\nde\nz", 2)
	feedVisualSeq(e, "<C-v>jyG0p")
	if e.Text() != "abc\nde\nzc\n " {
		t.Fatalf("p text = %q", e.Text())
	}
	e = newVisualAt("abc\nde\nz", 2)
	feedVisualSeq(e, "<C-v>jyG02P")
	if e.Text() != "abc\nde\nccz\n " {
		t.Fatalf("counted P text = %q", e.Text())
	}
}

func TestBlockwisePutPadsRegisterRowsOnExistingLines(t *testing.T) {
	e := newVisualAt("abc\nfgxy\nz", 2)
	feedVisualSeq(e, "<C-v>jlyj0P")
	if e.Text() != "abc\nc fgxy\nxyz" {
		t.Fatalf("existing row text = %q", e.Text())
	}
	e = newVisualAt("abc\nfgxy\nz", 2)
	feedVisualSeq(e, "<C-v>jlyG0P")
	if e.Text() != "abc\nfgxy\nc z\nxy" {
		t.Fatalf("created row text = %q", e.Text())
	}
	e = newVisualAt("abc\nfgxy\nz", 2)
	feedVisualSeq(e, "<C-v>jlyG02P")
	if e.Text() != "abc\nfgxy\nc c z\nxyxy" {
		t.Fatalf("counted created row text = %q", e.Text())
	}
}

func TestVisualEqualsPreservesCursorColumn(t *testing.T) {
	e := newVisualAt(" abcdef", 1)
	feedVisualSeq(e, "V=")
	if e.Text() != "abcdef" || e.Cursor() != 1 {
		t.Fatalf("text=%q cursor=%d", e.Text(), e.Cursor())
	}
	e = newVisualAt("  abcdef", 2)
	feedVisualSeq(e, "v2l=")
	if e.Text() != "abcdef" || e.Cursor() != 4 {
		t.Fatalf("text=%q cursor=%d", e.Text(), e.Cursor())
	}
	e = newVisualAt("  abcdef", 7)
	feedVisualSeq(e, "V=")
	if e.Text() != "abcdef" || e.Cursor() != 5 {
		t.Fatalf("text=%q cursor=%d", e.Text(), e.Cursor())
	}
	e = newVisualAt("  a\n  b", 1)
	feedVisualSeq(e, "Vj=")
	if e.Text() != "a\nb" || e.Cursor() != 0 {
		t.Fatalf("text=%q cursor=%d", e.Text(), e.Cursor())
	}
}

func TestVisualIndentCountMultipliesLevels(t *testing.T) {
	e := newVisualAt("abcdef", 0)
	feedVisualSeq(e, "v2l3>")
	if e.Text() != "\t\t\tabcdef" {
		t.Fatalf("text = %q", e.Text())
	}
	e = newVisualAt("a\nb", 0)
	feedVisualSeq(e, "Vj2>")
	if e.Text() != "\t\ta\n\t\tb" {
		t.Fatalf("text = %q", e.Text())
	}
	e = newVisualAt("\t\t\tabcdef", 0)
	feedVisualSeq(e, "v2l3<")
	if e.Text() != "abcdef" {
		t.Fatalf("text = %q", e.Text())
	}
	e = newVisualAt("abcdef\nxyz", 0)
	feedVisualSeq(e, "v2l3>j.")
	if e.Text() != "\t\t\tabcdef\n\t\t\txyz" {
		t.Fatalf("dot repeat text = %q", e.Text())
	}
}

func TestVisualOperatorCountIgnoredForNonShift(t *testing.T) {
	e := newVisualAt("abcdef", 0)
	feedVisualSeq(e, "v2l3x")
	if e.Text() != "def" || e.Register() != "abc" {
		t.Fatalf("x text=%q reg=%q", e.Text(), e.Register())
	}
	e = newVisualAt("abcdef", 0)
	feedVisualSeq(e, "v2l3d")
	if e.Text() != "def" || e.Register() != "abc" {
		t.Fatalf("d text=%q reg=%q", e.Text(), e.Register())
	}
}

func TestVisualGapKeysDiscardOperands(t *testing.T) {
	e := newVisualAt("abcd", 0)
	feedVisualSeq(e, "vllrx")
	if e.Text() != "abcd" {
		t.Fatalf("r operand ran as a command: %q", e.Text())
	}
	if e.Mode() != ModeVisual {
		t.Fatalf("mode = %v", e.Mode())
	}
	v := e.View("/", true)
	if !v.SelActive || v.SelStart != 0 || v.SelEnd != 2 {
		t.Fatalf("sel = %+v", v)
	}
	e = newVisualAt("one two one", 0)
	feedVisualSeq(e, "v/two<cr>")
	if e.Text() != "one two one" || e.Cursor() != 0 {
		t.Fatalf("search pattern ran as commands: text=%q cursor=%d", e.Text(), e.Cursor())
	}
	if e.Mode() != ModeVisual {
		t.Fatalf("mode = %v", e.Mode())
	}
	feedVisualSeq(e, "x")
	if e.Text() != "ne two one" {
		t.Fatalf("later key misrouted after pattern: %q", e.Text())
	}
	e = newVisualAt("abcd\nefgh", 0)
	feedVisualSeq(e, "<C-v>lyj0vlp")
	if e.Text() != "abcd\nefgh" {
		t.Fatalf("p changed the buffer: %q", e.Text())
	}
	if e.Mode() != ModeVisual {
		t.Fatalf("mode = %v", e.Mode())
	}
}

func TestVisualGapPendingCtrlCCancels(t *testing.T) {
	e := newVisualAt("abcd", 0)
	feedVisualSeq(e, "v/")
	if a := e.Feed("ctrl-c"); a != ActCancel {
		t.Fatalf("search pending ctrl-c action = %v want ActCancel", a)
	}
	feedVisualSeq(e, "x")
	if e.Text() != "bcd" {
		t.Fatalf("search pending ctrl-c poisoned later keys: %q", e.Text())
	}
	e = newVisualAt("abcd", 0)
	feedVisualSeq(e, "vllr")
	if a := e.Feed("ctrl-c"); a != ActCancel {
		t.Fatalf("replace pending ctrl-c action = %v want ActCancel", a)
	}
	feedVisualSeq(e, "x")
	if e.Text() != "d" {
		t.Fatalf("replace pending ctrl-c poisoned later keys: %q", e.Text())
	}
	e = newVisualAt("abcd", 0)
	feedVisualSeq(e, "vf")
	if a := e.Feed("ctrl-c"); a != ActCancel {
		t.Fatalf("find pending ctrl-c action = %v want ActCancel", a)
	}
	feedVisualSeq(e, "x")
	if e.Text() != "bcd" {
		t.Fatalf("find pending ctrl-c poisoned later keys: %q", e.Text())
	}
	e = newVisualAt("abcd", 0)
	feedVisualSeq(e, "vi")
	if a := e.Feed("ctrl-c"); a != ActCancel {
		t.Fatalf("object pending ctrl-c action = %v want ActCancel", a)
	}
	feedVisualSeq(e, "x")
	if e.Text() != "bcd" {
		t.Fatalf("object pending ctrl-c poisoned later keys: %q", e.Text())
	}
	e = newVisualAt("abcd", 0)
	feedVisualSeq(e, "vg")
	if a := e.Feed("ctrl-c"); a != ActCancel {
		t.Fatalf("g pending ctrl-c action = %v want ActCancel", a)
	}
	feedVisualSeq(e, "x")
	if e.Text() != "bcd" {
		t.Fatalf("g pending ctrl-c poisoned later keys: %q", e.Text())
	}
}

func TestBlockwiseDeleteRelocatesMarks(t *testing.T) {
	e := newVisualAt("abcd\nefgh", 6)
	feedVisualSeq(e, "mak0<C-v>jd`a")
	if e.Text() != "bcd\nfgh" {
		t.Fatalf("text = %q", e.Text())
	}
	if e.Cursor() != 5 {
		t.Fatalf("cursor = %d want 5", e.Cursor())
	}
}

func TestVisualEnterExitsBeforeAccept(t *testing.T) {
	e := newVisualAt("abcdef", 0)
	feedVisualSeq(e, "vll")
	if e.Mode() != ModeVisual {
		t.Fatalf("mode = %v", e.Mode())
	}
	feedVisualSeq(e, "<cr>")
	if e.Mode() != ModeNormal {
		t.Fatalf("mode after enter = %v", e.Mode())
	}
	feedVisualSeq(e, "d")
	if e.Text() != "abcdef" {
		t.Fatalf("enter then d must not delete a prior visual span: %q", e.Text())
	}
}

func TestVisualVerticalMotionsRestoreDesiredColumn(t *testing.T) {
	e := newVisualAt("abcdefgh\nxy\nQRSTUVWX", 4)
	feedVisualSeq(e, "vjjy")
	if e.Register() != "efgh\nxy\nQRSTU" {
		t.Fatalf("vjjy reg = %q", e.Register())
	}
	e = newVisualAt("abcdefgh\nxy\nQRSTUVWX", 16)
	feedVisualSeq(e, "vkky")
	if e.Register() != "efgh\nxy\nQRSTU" {
		t.Fatalf("vkky reg = %q", e.Register())
	}
	e = newVisualAt("abcdefgh\nxy\nQRSTUVWX", 4)
	feedVisualSeq(e, "vjky")
	if e.Register() != "e" {
		t.Fatalf("vjky reg = %q", e.Register())
	}
	e = newVisualAt("abcdefgh\nxy\nQRSTUVWX", 16)
	feedVisualSeq(e, "vkjy")
	if e.Register() != "U" {
		t.Fatalf("vkjy reg = %q", e.Register())
	}
	e = newVisualAt("abcdefgh\nxy\nQRSTUVWX", 4)
	feedVisualSeq(e, "vjGy")
	if e.Register() != "efgh\nxy\nQRSTU" {
		t.Fatalf("vjGy reg = %q", e.Register())
	}
	e = newVisualAt("abcdefgh\nxy\nQRSTUVWX", 4)
	feedVisualSeq(e, "vjggy")
	if e.Register() != "e" {
		t.Fatalf("vjggy reg = %q", e.Register())
	}
	e = newVisualAt("abcdefgh\nxy\nQRSTUVWX", 4)
	feedVisualSeq(e, "jvjx")
	if e.Text() != "abcdefgh\nxVWX" || e.Register() != "y\nQRSTU" {
		t.Fatalf("jvjx text = %q reg = %q", e.Text(), e.Register())
	}
}

func TestNormalNavRestoresDesiredColumn(t *testing.T) {
	e := newVisualAt("abcdefgh\nxy\nQRSTUVWX", 4)
	feedVisualSeq(e, "jjx")
	if e.Text() != "abcdefgh\nxy\nQRSTVWX" || e.Register() != "U" {
		t.Fatalf("jjx text = %q reg = %q", e.Text(), e.Register())
	}
	e = newVisualAt("abcdefgh\nxy\nQRSTUVWX", 4)
	feedVisualSeq(e, "jjkjx")
	if e.Text() != "abcdefgh\nxy\nQRSTVWX" || e.Register() != "U" {
		t.Fatalf("jjkjx text = %q reg = %q", e.Text(), e.Register())
	}
	e = newVisualAt("abcdefgh\nxy\nQRSTUVWX", 4)
	feedVisualSeq(e, "vj<esc>jx")
	if e.Text() != "abcdefgh\nxy\nQSTUVWX" || e.Register() != "R" {
		t.Fatalf("vj<esc>jx text = %q reg = %q", e.Text(), e.Register())
	}
}

func TestGVReselectOnlyImmediatePair(t *testing.T) {
	e := newVisualAt("aa\nbb\ncc", 0)
	feedVisualSeq(e, "vj<esc>ggvld")
	if e.Text() != "\nbb\ncc" || e.Register() != "aa" {
		t.Fatalf("ggvld text = %q reg = %q", e.Text(), e.Register())
	}
	e = newVisualAt("aa\nbb\ncc", 3)
	feedVisualSeq(e, "vj<esc>g2v")
	if e.Mode() != ModeVisual {
		t.Fatalf("mode = %v", e.Mode())
	}
	v := e.View("/", true)
	if !v.SelActive || v.SelStart != 0 {
		t.Fatalf("g2v selection = %+v want fresh at 0", v)
	}
}
