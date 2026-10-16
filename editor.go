package vimline

// Package vimline is a framework-neutral vim line Editor: modal insert,
// normal, and replace states, motions, operators with counts, one register,
// undo and redo, and the jj exit idiom. It renders nothing; every consumer
// paints the View it gets.

import (
	"unicode"
)

// edit.go is the shared vim-style line Editor. The search prompt, the sticky
// note prompt, and the custom tab name field all drive this one type, so the
// editing feel is identical wherever text is entered.
//
// The key inventory follows GNU readline vi-editing-mode and zsh bindkey -v:
// motions, operators with counts, one unnamed register, undo and redo, and
// replace mode. j/k and the arrows move the result list, because a single
// line has no vertical motion to spend them on.

type Mode int

const (
	ModeInsert Mode = iota
	ModeNormal
	ModeReplace
)

// Action is what the caller must do with a key the Editor did not keep.
type Action int

const (
	ActStay Action = iota
	ActAccept
	ActOut
	ActCancel
	ActUp
	ActDown
	ActFirst
	ActLast
	ActPageUp
	ActPageDown
)

type undoSnap struct {
	text   []rune
	cursor int
}

type Editor struct {
	text   []rune
	cursor int
	mode   Mode

	pending   rune // 'd', 'c', 'y', 'r', 'f', 'F', 't', 'T'
	pendingOp rune // operator held while a find motion waits for its char
	count     int  // numeric prefix
	pendingJ  bool // insert mode saw one j and may be half of jj

	reg []rune

	undo []undoSnap
	redo []undoSnap

	findCh  rune
	findDir int

	// change tracks one insert session so '.' can replay it.
	inChange    bool
	changeM     rune
	changeCount int
	changeFrom  int

	lastEdit func(*Editor)
}

func New(s string) *Editor {
	rs := []rune(s)
	return &Editor{text: rs, cursor: len(rs), mode: ModeInsert}
}

func (e *Editor) String() string { return string(e.text) }

// Text returns the buffer contents.
func (e *Editor) Text() string { return string(e.text) }

// PendingJ reports whether insert mode is holding one j for a possible jj.
func (e *Editor) Cursor() int { return e.cursor }

// Mode reports the current editing mode.
func (e *Editor) Mode() Mode { return e.mode }

func (e *Editor) PendingJ() bool { return e.pendingJ }

// FlushJ inserts a held j when no second j arrived inside the window.
func (e *Editor) FlushJ() {
	if !e.pendingJ {
		return
	}
	e.pendingJ = false
	e.insertRunes([]rune{'j'})
}

func (e *Editor) pushUndo() {
	e.undo = append(e.undo, undoSnap{text: append([]rune(nil), e.text...), cursor: e.cursor})
	e.redo = e.redo[:0]
}

func (e *Editor) takeCount() int {
	c := e.count
	if c <= 0 {
		c = 1
	}
	e.count = 0
	return c
}

func (e *Editor) clamp() {
	if e.cursor > len(e.text) {
		e.cursor = len(e.text)
	}
	if e.cursor < 0 {
		e.cursor = 0
	}
}

// ---------------------------------------------------------------------------
// motions
// ---------------------------------------------------------------------------

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (e *Editor) wordClass(r rune, big bool) int {
	if r == ' ' {
		return 0
	}
	if big {
		return 1
	}
	if isWordRune(r) {
		return 1
	}
	return 2
}

func (e *Editor) wordForward(p int, big bool) int {
	n := len(e.text)
	if p >= n {
		return n
	}
	start := e.wordClass(e.text[p], big)
	i := p
	for i < n && e.wordClass(e.text[i], big) == start {
		i++
	}
	for i < n && e.text[i] == ' ' {
		i++
	}
	return i
}

func (e *Editor) wordBack(p int, big bool) int {
	if p <= 0 {
		return 0
	}
	i := p - 1
	for i > 0 && e.text[i] == ' ' {
		i--
	}
	cls := e.wordClass(e.text[i], big)
	for i > 0 && e.wordClass(e.text[i-1], big) == cls {
		i--
	}
	return i
}

func (e *Editor) wordEnd(p int, big bool) int {
	n := len(e.text)
	if n == 0 {
		return 0
	}
	i := p + 1
	for i < n && e.text[i] == ' ' {
		i++
	}
	if i >= n {
		return n - 1
	}
	cls := e.wordClass(e.text[i], big)
	for i+1 < n && e.wordClass(e.text[i+1], big) == cls {
		i++
	}
	return i
}

// findChar runs f, F, t or T. findCh must already be set.
func (e *Editor) findChar(m rune, count int) (int, bool) {
	n := len(e.text)
	if e.findCh == 0 {
		return e.cursor, false
	}
	step := 1
	if m == 'F' || m == 'T' {
		step = -1
	}
	p := e.cursor
	for i := 0; i < count; i++ {
		q := p + step
		for q >= 0 && q < n && e.text[q] != e.findCh {
			q += step
		}
		if q < 0 || q >= n {
			return e.cursor, false
		}
		p = q
	}
	if m == 't' {
		p--
	}
	if m == 'T' {
		p++
	}
	if p < 0 || p >= n {
		return e.cursor, false
	}
	return p, true
}

// move applies one motion and returns the target cursor. ok is false when a
// character search finds nothing, which in vim aborts the command.
func (e *Editor) move(m rune, count int) (int, bool) {
	if count < 1 {
		count = 1
	}
	p := e.cursor
	n := len(e.text)
	switch m {
	case 'h':
		p -= count
		if p < 0 {
			p = 0
		}
	case 'l':
		p += count
		if p > n {
			p = n
		}
	case '0':
		p = 0
	case '^':
		p = 0
		for p < n && e.text[p] == ' ' {
			p++
		}
	case '$':
		p = n
	case 'w', 'W':
		for i := 0; i < count; i++ {
			p = e.wordForward(p, m == 'W')
		}
	case 'b', 'B':
		for i := 0; i < count; i++ {
			p = e.wordBack(p, m == 'B')
		}
	case 'e', 'E':
		for i := 0; i < count; i++ {
			p = e.wordEnd(p, m == 'E')
		}
	case 'f', 'F', 't', 'T':
		return e.findChar(m, count)
	case ';':
		if e.findCh == 0 {
			return p, false
		}
		if e.findDir < 0 {
			return e.findChar('F', count)
		}
		return e.findChar('f', count)
	case ',':
		if e.findCh == 0 {
			return p, false
		}
		if e.findDir < 0 {
			return e.findChar('f', count)
		}
		return e.findChar('F', count)
	}
	return p, true
}

// rangeFor maps an operator motion onto a half-open [start,end) span.
func (e *Editor) rangeFor(m rune, count int) (int, int, bool) {
	start := e.cursor
	target, ok := e.move(m, count)
	if !ok {
		return 0, 0, false
	}
	switch m {
	case 'b', 'B', '0', '^':
		return target, e.cursor, true
	case '$':
		return e.cursor, len(e.text), true
	}
	if target < start {
		return target, start, true
	}
	// Forward motions are inclusive of the target character, except w and W
	// which stop at the start of the next word.
	if m == 'w' || m == 'W' {
		return start, target, true
	}
	end := target + 1
	if end > len(e.text) {
		end = len(e.text)
	}
	return start, end, true
}

// ---------------------------------------------------------------------------
// edits
// ---------------------------------------------------------------------------

func (e *Editor) insertRunes(rs []rune) {
	e.pushUndo()
	out := make([]rune, 0, len(e.text)+len(rs))
	out = append(out, e.text[:e.cursor]...)
	out = append(out, rs...)
	out = append(out, e.text[e.cursor:]...)
	e.text = out
	e.cursor += len(rs)
	e.clamp()
}

func (e *Editor) deleteRange(a, b int) {
	if a > b {
		a, b = b, a
	}
	if a < 0 {
		a = 0
	}
	if b > len(e.text) {
		b = len(e.text)
	}
	if a == b {
		return
	}
	e.pushUndo()
	e.reg = append([]rune(nil), e.text[a:b]...)
	out := make([]rune, 0, len(e.text)-(b-a))
	out = append(out, e.text[:a]...)
	out = append(out, e.text[b:]...)
	e.text = out
	e.cursor = a
	e.clamp()
}

// beginChange arms '.' capture for one insert session.
func (e *Editor) beginChange(m rune, count int) {
	e.inChange = true
	e.changeM = m
	e.changeCount = count
	e.changeFrom = e.cursor
}

// endChange records the finished insert session as the '.' repeat target.
func (e *Editor) endChange() {
	if !e.inChange {
		return
	}
	e.inChange = false
	kind := e.changeM
	count := e.changeCount
	ins := append([]rune(nil), e.text[e.changeFrom:e.cursor]...)
	e.lastEdit = func(ed *Editor) {
		switch kind {
		case 'i', 'I':
			ed.insertRunes(ins)
		case 'a', 'A':
			if ed.cursor < len(ed.text) {
				ed.cursor++
			}
			ed.insertRunes(ins)
		case 'C', '$':
			ed.deleteRange(ed.cursor, len(ed.text))
			ed.insertRunes(ins)
		case 'L':
			ed.deleteRange(0, len(ed.text))
			ed.cursor = 0
			ed.insertRunes(ins)
		default:
			if s, en, ok := ed.rangeFor(kind, count); ok {
				ed.deleteRange(s, en)
				ed.insertRunes(ins)
			}
		}
	}
}

func (e *Editor) operate(op, m rune, count int) {
	if m == op {
		switch op {
		case 'y':
			e.reg = append([]rune(nil), e.text...)
		case 'd':
			e.deleteRange(0, len(e.text))
			e.lastEdit = func(ed *Editor) { ed.deleteRange(0, len(ed.text)) }
		case 'c':
			e.deleteRange(0, len(e.text))
			e.beginChange('L', 1)
			e.mode = ModeInsert
		}
		return
	}
	// Vim special case: cw and cW change to the end of the word, not to the
	// start of the next one. They behave exactly like ce and cE.
	if op == 'c' {
		if m == 'w' {
			m = 'e'
		} else if m == 'W' {
			m = 'E'
		}
	}
	start, end, ok := e.rangeFor(m, count)
	if !ok {
		return
	}
	switch op {
	case 'y':
		e.reg = append([]rune(nil), e.text[start:end]...)
	case 'd':
		e.deleteRange(start, end)
		mc := m
		cc := count
		e.lastEdit = func(ed *Editor) {
			if s, en, ok := ed.rangeFor(mc, cc); ok {
				ed.deleteRange(s, en)
			}
		}
	case 'c':
		e.deleteRange(start, end)
		e.beginChange(m, count)
		e.mode = ModeInsert
	}
}

func (e *Editor) paste(before bool) {
	if len(e.reg) == 0 {
		return
	}
	e.pushUndo()
	at := e.cursor
	if !before && at < len(e.text) {
		at++
	}
	out := make([]rune, 0, len(e.text)+len(e.reg))
	out = append(out, e.text[:at]...)
	out = append(out, e.reg...)
	out = append(out, e.text[at:]...)
	e.text = out
	e.cursor = at + len(e.reg) - 1
	e.clamp()
	e.lastEdit = func(ed *Editor) { ed.paste(before) }
}

func (e *Editor) deleteChar(before bool) {
	if before {
		if e.cursor == 0 {
			return
		}
		e.deleteRange(e.cursor-1, e.cursor)
		return
	}
	if e.cursor >= len(e.text) {
		return
	}
	e.deleteRange(e.cursor, e.cursor+1)
}

func (e *Editor) replaceChar(r rune) {
	if e.cursor >= len(e.text) {
		return
	}
	e.pushUndo()
	e.text[e.cursor] = r
}

func (e *Editor) toggleCase() {
	if e.cursor >= len(e.text) {
		return
	}
	e.pushUndo()
	r := e.text[e.cursor]
	if unicode.IsUpper(r) {
		r = unicode.ToLower(r)
	} else {
		r = unicode.ToUpper(r)
	}
	e.text[e.cursor] = r
	e.cursor++
	e.clamp()
}

func (e *Editor) undoEdit() {
	if len(e.undo) == 0 {
		return
	}
	snap := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.redo = append(e.redo, undoSnap{text: append([]rune(nil), e.text...), cursor: e.cursor})
	e.text = snap.text
	e.cursor = snap.cursor
	e.clamp()
}

func (e *Editor) redoEdit() {
	if len(e.redo) == 0 {
		return
	}
	snap := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.undo = append(e.undo, undoSnap{text: append([]rune(nil), e.text...), cursor: e.cursor})
	e.text = snap.text
	e.cursor = snap.cursor
	e.clamp()
}

// ---------------------------------------------------------------------------
// feed
// ---------------------------------------------------------------------------

func (e *Editor) printable(key string) (rune, bool) {
	if len(key) != 1 {
		return 0, false
	}
	r := rune(key[0])
	if r < 0x20 || r >= 0x7f {
		return 0, false
	}
	return r, true
}

func (e *Editor) Feed(key string) Action {
	// A pending r, f, F, t or T is waiting for one literal character.
	if e.pending == 'r' {
		e.pending = 0
		if r, ok := e.printable(key); ok {
			e.replaceChar(r)
			rr := r
			e.lastEdit = func(ed *Editor) { ed.replaceChar(rr) }
		}
		return ActStay
	}
	if e.pending == 'f' || e.pending == 'F' || e.pending == 't' || e.pending == 'T' {
		m := e.pending
		e.pending = 0
		if r, ok := e.printable(key); ok {
			e.findCh = r
			if m == 'f' || m == 't' {
				e.findDir = 1
			} else {
				e.findDir = -1
			}
			if p, ok := e.move(m, e.takeCount()); ok {
				e.cursor = p
			}
		}
		return ActStay
	}

	// An operator is waiting for its motion.
	if e.pending == 'd' || e.pending == 'c' || e.pending == 'y' {
		op := e.pending
		e.pending = 0
		if key == "esc" {
			return ActStay
		}
		if key == string(op) {
			e.operate(op, op, 1)
			return ActStay
		}
		if m, ok := e.printable(key); ok {
			if m == 'f' || m == 'F' || m == 't' || m == 'T' {
				e.pending = m
				e.pendingOp = op
				return ActStay
			}
			e.operate(op, m, e.takeCount())
		}
		return ActStay
	}

	// Numeric prefix. 0 is a motion unless a count is already running.
	if e.mode == ModeNormal {
		if r, ok := e.printable(key); ok {
			if (r >= '1' && r <= '9') || (r == '0' && e.count > 0) {
				e.count = e.count*10 + int(r-'0')
				return ActStay
			}
		}
	}

	switch e.mode {
	case ModeInsert, ModeReplace:
		return e.feedInsert(key)
	default:
		return e.feedNormal(key)
	}
}

func (e *Editor) feedInsert(key string) Action {
	// jj leaves insert mode. The first j is held back so the pair can act as
	// one key; the caller flushes it on a timeout when no second j arrives.
	if e.pendingJ {
		e.pendingJ = false
		if key == "j" {
			return e.feedInsert("esc")
		}
		e.insertRunes([]rune{'j'})
	}
	switch key {
	case "esc":
		e.pendingJ = false
		e.endChange()
		e.mode = ModeNormal
		if e.cursor > 0 {
			e.cursor--
		}
		e.pending = 0
		e.clamp()
		return ActStay
	case "enter":
		return ActAccept
	case "ctrl-c":
		return ActCancel
	case "bs":
		e.deleteChar(true)
		return ActStay
	case "ctrl-u":
		if e.cursor > 0 {
			e.deleteRange(0, e.cursor)
		}
		return ActStay
	case "ctrl-w":
		p, _ := e.move('b', 1)
		if p < e.cursor {
			e.deleteRange(p, e.cursor)
		}
		return ActStay
	case "ctrl-k":
		if e.cursor < len(e.text) {
			e.deleteRange(e.cursor, len(e.text))
		}
		return ActStay
	case "ctrl-y":
		e.insertRunes(e.reg)
		return ActStay
	case "ctrl-d":
		e.deleteChar(false)
		return ActStay
	case "ctrl-n", "down":
		return ActDown
	case "ctrl-p", "up":
		return ActUp
	case "left":
		if e.cursor > 0 {
			e.cursor--
		}
		return ActStay
	case "right":
		if e.cursor < len(e.text) {
			e.cursor++
		}
		return ActStay
	}
	if key == "j" && e.mode == ModeInsert {
		e.pendingJ = true
		return ActStay
	}
	if r, ok := e.printable(key); ok {
		if e.mode == ModeReplace && e.cursor < len(e.text) {
			e.pushUndo()
			e.text[e.cursor] = r
			e.cursor++
			e.clamp()
			return ActStay
		}
		e.insertRunes([]rune{r})
	}
	return ActStay
}

func (e *Editor) feedNormal(key string) Action {
	if r, ok := e.printable(key); ok {
		switch r {
		case 'h':
			p, _ := e.move('h', e.takeCount())
			e.cursor = p
			return ActStay
		case 'l':
			p, _ := e.move('l', e.takeCount())
			e.cursor = p
			return ActStay
		case 'w', 'W', 'b', 'B', 'e', 'E', '0', '^', '$':
			p, _ := e.move(r, e.takeCount())
			e.cursor = p
			return ActStay
		case 'f', 'F', 't', 'T':
			e.pending = r
			return ActStay
		case ';', ',':
			if p, ok := e.move(r, e.takeCount()); ok {
				e.cursor = p
			}
			return ActStay
		case 'i':
			e.mode = ModeInsert
			e.beginChange('i', 1)
			return ActStay
		case 'I':
			e.mode = ModeInsert
			e.cursor, _ = e.move('^', 1)
			e.beginChange('I', 1)
			return ActStay
		case 'a':
			e.mode = ModeInsert
			if e.cursor < len(e.text) {
				e.cursor++
			}
			e.beginChange('a', 1)
			return ActStay
		case 'A':
			e.mode = ModeInsert
			e.cursor = len(e.text)
			e.beginChange('A', 1)
			return ActStay
		case 'R':
			e.mode = ModeReplace
			return ActStay
		case 'x':
			n := e.takeCount()
			for i := 0; i < n; i++ {
				e.deleteChar(false)
			}
			e.lastEdit = func(ed *Editor) {
				for i := 0; i < n; i++ {
					ed.deleteChar(false)
				}
			}
			return ActStay
		case 'X':
			n := e.takeCount()
			for i := 0; i < n; i++ {
				e.deleteChar(true)
			}
			e.lastEdit = func(ed *Editor) {
				for i := 0; i < n; i++ {
					ed.deleteChar(true)
				}
			}
			return ActStay
		case 'r':
			e.pending = 'r'
			return ActStay
		case 's':
			n := e.takeCount()
			end := e.cursor + n
			if end > len(e.text) {
				end = len(e.text)
			}
			e.deleteRange(e.cursor, end)
			e.beginChange('s', n)
			e.mode = ModeInsert
			return ActStay
		case 'd', 'c', 'y':
			e.pending = r
			return ActStay
		case 'D':
			e.deleteRange(e.cursor, len(e.text))
			e.lastEdit = func(ed *Editor) { ed.deleteRange(ed.cursor, len(ed.text)) }
			return ActStay
		case 'C':
			e.deleteRange(e.cursor, len(e.text))
			e.beginChange('C', 1)
			e.mode = ModeInsert
			return ActStay
		case 'Y':
			e.reg = append([]rune(nil), e.text...)
			return ActStay
		case 'p':
			e.paste(false)
			return ActStay
		case 'P':
			e.paste(true)
			return ActStay
		case 'u':
			e.undoEdit()
			return ActStay
		case '~':
			n := e.takeCount()
			for i := 0; i < n; i++ {
				e.toggleCase()
			}
			e.lastEdit = func(ed *Editor) {
				for i := 0; i < n; i++ {
					ed.toggleCase()
				}
			}
			return ActStay
		case 'g':
			return ActFirst
		case 'G':
			return ActLast
		case '.':
			if e.lastEdit != nil {
				e.pushUndo()
				e.lastEdit(e)
			}
			return ActStay
		}
	}

	switch key {
	case "esc":
		if e.pending != 0 {
			e.pending = 0
			e.pendingOp = 0
			e.count = 0
			return ActStay
		}
		return ActOut
	case "enter":
		return ActAccept
	case "ctrl-c":
		return ActCancel
	case "ctrl-r":
		e.redoEdit()
		return ActStay
	case "ctrl-n", "down", "j":
		return ActDown
	case "ctrl-p", "up", "k":
		return ActUp
	case "ctrl-u":
		return ActPageUp
	case "ctrl-d":
		return ActPageDown
	case "left":
		p, _ := e.move('h', e.takeCount())
		e.cursor = p
		return ActStay
	case "right":
		p, _ := e.move('l', e.takeCount())
		e.cursor = p
		return ActStay
	}
	return ActStay
}

// ---------------------------------------------------------------------------
// view model
// ---------------------------------------------------------------------------

// View is the framework-neutral snapshot a consumer renders. The module owns
// the state and the semantics; the caller owns every pixel.
type View struct {
	Label   string
	Text    string
	Cursor  int
	Mode    Mode
	Pending rune
	Count   int
	Focused bool
}

// View returns the renderable snapshot of the Editor.
func (e *Editor) View(label string, focused bool) View {
	return View{
		Label:   label,
		Text:    e.Text(),
		Cursor:  e.cursor,
		Mode:    e.mode,
		Pending: e.pending,
		Count:   e.count,
		Focused: focused,
	}
}
