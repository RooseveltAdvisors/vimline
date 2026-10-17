package keys

import (
	"reflect"
	"testing"
)

func TestSplitBurstsKeepEveryKey(t *testing.T) {
	// The bug this package exists to kill: a burst in one read used to lose
	// every byte after the first.
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"ggt", []string{"g", "g", "t"}},
		{"tc", []string{"t", "c"}},
		{"abc", []string{"a", "b", "c"}},
		{"\r", []string{"enter"}},
		{"\x7f", []string{"bs"}},
		{"\x15", []string{"ctrl-u"}},
		{"\x0e", []string{"ctrl-n"}},
		{"\x10", []string{"ctrl-p"}},
		{"\x04", []string{"ctrl-d"}},
		{"\x1b", []string{"esc"}},
		{"\x1b[A", []string{"up"}},
		{"\x1b[B", []string{"down"}},
		{"\x1b[C", []string{"right"}},
		{"\x1b[D", []string{"left"}},
		{"\x1b[1;5A", []string{"up"}},
		{"\x1b[13u", []string{"enter"}},
		{"\x1b[57417u", []string{"left"}},
		{"\x1b[97u", []string{"a"}},
		{"\x1b[Z", []string{"esc"}},
		{"\x1b[A\x1b[B", []string{"up", "down"}},
		{"j\x1b[Ak", []string{"j", "up", "k"}},
	} {
		if got := Split([]byte(tc.in)); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("Split(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSplitAltModifierYieldsEscThenKey(t *testing.T) {
	got := Split([]byte("\x1bx"))
	if !reflect.DeepEqual(got, []string{"esc", "x"}) {
		t.Fatalf("alt-x = %v", got)
	}
}

func TestSplitKeepsOneUTF8RuneWhole(t *testing.T) {
	got := Split([]byte("aé"))
	if !reflect.DeepEqual(got, []string{"a", "é"}) {
		t.Fatalf("utf8 = %v", got)
	}
}

func TestDispatcherChordLifecycle(t *testing.T) {
	d := New()
	d.Register("g", map[string]string{"g": "first", "t": "next-account", "T": "prev-account"}, "first - t account")

	if _, r := d.Feed("3"); r != Consumed {
		t.Fatalf("3 result = %v", r)
	}
	if _, r := d.Feed("g"); r != Consumed {
		t.Fatalf("g result = %v", r)
	}
	if p, hint, ok := d.Pending(); !ok || p != "g" || hint != "first - t account" {
		t.Fatalf("pending = %q %q %v", p, hint, ok)
	}
	a, r := d.Feed("t")
	if r != Chord || a.Name != "next-account" || a.Count != 3 {
		t.Fatalf("chord = %+v result = %v", a, r)
	}
	if d.Count() != 0 {
		t.Fatal("count survived the chord")
	}
	if _, _, ok := d.Pending(); ok {
		t.Fatal("pending survived the chord")
	}
}

func TestDispatcherCancelAndSwallow(t *testing.T) {
	d := New()
	d.Register("t", map[string]string{"c": "create"}, "c create")
	d.Feed("2")
	d.Feed("t")
	// A key that matches no continuation cancels and is swallowed.
	if a, r := d.Feed("q"); r != Consumed || a.Name != "" {
		t.Fatalf("cancel = %+v %v", a, r)
	}
	if d.Count() != 0 {
		t.Fatal("count survived the cancel")
	}
	// The swallowed key is gone: the next key is fresh.
	if _, r := d.Feed("e"); r != Pass {
		t.Fatalf("post-cancel key = %v", r)
	}
}

func TestDispatcherEscCancelsPending(t *testing.T) {
	d := New()
	d.Register("Z", map[string]string{"Z": "quit"}, "quit")
	d.Feed("Z")
	if _, r := d.Feed("esc"); r != Consumed {
		t.Fatalf("esc on pending = %v", r)
	}
	if _, _, ok := d.Pending(); ok {
		t.Fatal("pending survived esc")
	}
	// esc with nothing pending passes through to the caller.
	if _, r := d.Feed("esc"); r != Pass {
		t.Fatalf("bare esc = %v", r)
	}
}

func TestDispatcherZeroPassesThroughWithoutCount(t *testing.T) {
	d := New()
	d.Register("g", map[string]string{"g": "first"}, "first")
	if _, r := d.Feed("0"); r != Pass {
		t.Fatalf("bare 0 = %v, want pass", r)
	}
	d.Feed("1")
	if _, r := d.Feed("0"); r != Consumed {
		t.Fatalf("0 in count = %v", r)
	}
	if d.Count() != 10 {
		t.Fatalf("count = %d, want 10", d.Count())
	}
}

func TestDispatcherTakeCountPairsWithPassedKeys(t *testing.T) {
	d := New()
	d.Feed("3")
	if _, r := d.Feed("j"); r != Pass {
		t.Fatalf("j = %v", r)
	}
	if n := d.TakeCount(); n != 3 {
		t.Fatalf("take = %d", n)
	}
	if d.TakeCount() != 0 {
		t.Fatal("take did not clear")
	}
}

func TestDispatcherEscClearsRunningCount(t *testing.T) {
	d := New()
	d.Feed("5")
	if _, r := d.Feed("esc"); r != Consumed {
		t.Fatalf("esc with count = %v", r)
	}
	if d.Count() != 0 {
		t.Fatal("count survived esc")
	}
}

func TestDispatcherSpaceIsALeader(t *testing.T) {
	d := New()
	d.Register(" ", map[string]string{"u": "unread"}, "u unread")
	if _, r := d.Feed(" "); r != Consumed {
		t.Fatalf("space = %v", r)
	}
	a, r := d.Feed("u")
	if r != Chord || a.Name != "unread" {
		t.Fatalf("space+u = %+v %v", a, r)
	}
}
