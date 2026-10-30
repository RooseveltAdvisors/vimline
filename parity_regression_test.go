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
