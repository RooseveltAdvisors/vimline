package vimline

import (
	"strings"
	"testing"
)

func feedVisualSeq(e *Editor, seq string) {
	seq = strings.NewReplacer("<C-v>", "\x16", "<c-v>", "\x16").Replace(seq)
	for _, key := range splitOracleKeys(seq) {
		switch key {
		case "\x1b":
			key = "esc"
		case "\x16":
			key = "ctrl-v"
		case "\x12":
			key = "ctrl-r"
		case "\x08":
			key = "bs"
		case "\r":
			key = "enter"
		}
		e.Feed(key)
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
