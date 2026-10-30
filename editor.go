package vimline

// Package vimline is a framework-neutral vim line Editor: modal insert,
// normal, and replace states, motions, operators with counts, one register,
// undo and redo, and the jj exit idiom. It renders nothing; every consumer
// paints the View it gets.

import (
	"unicode"
	"unicode/utf8"
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

	pending   rune // 'd', 'c', 'y', 'r', 'f', 'F', 't', 'T', 'i', 'a', 'g'
	pendingOp rune // operator held while a multi-key motion or text object completes
	opCount   int  // numeric prefix captured before the operator (multiplied with motion count)
	count     int  // numeric prefix
	pendingJ  bool // insert mode saw one j and may be half of jj

	reg []rune

	undo []undoSnap
	redo []undoSnap

	findCh   rune
	findKind rune // 'f', 'F', 't', 'T'
	findDir  int

	// change tracks one insert session so '.' can replay it.
	inChange     bool
	changeM      rune
	changeObj    rune
	changeAround bool
	changeFindK  rune
	changeFindCh rune
	changeRepeat bool
	changeCount  int
	changeFrom   int

	lastEdit func(*Editor)
}

func New(s string) *Editor {
	rs := []rune(s)
	return &Editor{text: rs, cursor: len(rs), mode: ModeInsert}
}

func (e *Editor) String() string { return string(e.text) }

// Text returns the buffer contents.
func (e *Editor) Text() string { return string(e.text) }

// SetMode forces the editing mode. Consumers use it to refocus a field in
// insert after a mode change elsewhere.
func (e *Editor) SetMode(m Mode) { e.mode = m }

// End puts the cursor after the last character.
func (e *Editor) End() { e.cursor = len(e.text) }

// SetCursor moves the cursor and clamps it to the buffer.
func (e *Editor) SetCursor(p int) {
	e.cursor = p
	e.clamp()
}

// InsertLiteral inserts literal runes at the cursor while preserving undo history.
func (e *Editor) InsertLiteral(s string) {
	if s == "" {
		return
	}
	e.insertRunes([]rune(s))
}

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
	if unicode.IsSpace(r) {
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
	if start != 0 {
		for i < n && e.wordClass(e.text[i], big) == start {
			i++
		}
	}
	for i < n && unicode.IsSpace(e.text[i]) {
		i++
	}
	return i
}

func (e *Editor) wordBack(p int, big bool) int {
	if p <= 0 {
		return 0
	}
	i := p - 1
	for i > 0 && unicode.IsSpace(e.text[i]) {
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
	for i < n && unicode.IsSpace(e.text[i]) {
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

// findChar runs f, F, t or T on the current line. findCh must already be set.
func (e *Editor) findChar(m rune, count int) (int, bool) {
	return e.findCharLine(m, e.findCh, count, false)
}

// move applies one motion and returns the target cursor. ok is false when a
// character search or bracket match finds nothing, which in vim aborts the command.
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
	case 'l', ' ':
		p += count
		if p > n {
			p = n
		}
	case '0':
		p, _, _, _ = e.lineAt(p)
	case '^':
		p, _, _, _ = e.lineAt(p)
		for p < n && isSpaceNotNL(e.text[p]) {
			p++
		}
	case '_':
		p = e.firstNonBlankDown(p, count)
	case '$':
		_, p, _, _ = e.lineAt(p)
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
		return e.findCharLine(m, e.findCh, count, false)
	case ';':
		if e.findCh == 0 {
			return p, false
		}
		k := e.findKind
		if k == 0 {
			if e.findDir < 0 {
				k = 'F'
			} else {
				k = 'f'
			}
		}
		return e.findCharLine(k, e.findCh, count, true)
	case ',':
		if e.findCh == 0 {
			return p, false
		}
		k := e.findKind
		if k == 0 {
			if e.findDir < 0 {
				k = 'f'
			} else {
				k = 'F'
			}
		} else {
			k = reverseFindKind(k)
		}
		return e.findCharLine(k, e.findCh, count, true)
	case '%':
		return e.matchBracket(p)
	case '{':
		return e.paragraphMove(p, -1, count), true
	case '}':
		return e.paragraphMove(p, 1, count), true
	default:
		return p, false
	}
	return p, true
}

// lineAt returns the current line. content is the text without the break.
// del includes the newline, or the preceding newline on the last line, so dd
// removes one line and does not erase the rest of the buffer.
func (e *Editor) lineAt(p int) (contentStart, contentEnd, delStart, delEnd int) {
	n := len(e.text)
	if p > n {
		p = n
	}
	if p < 0 {
		p = 0
	}
	contentStart = p
	for contentStart > 0 && e.text[contentStart-1] != '\n' {
		contentStart--
	}
	contentEnd = p
	for contentEnd < n && e.text[contentEnd] != '\n' {
		contentEnd++
	}
	delStart, delEnd = contentStart, contentEnd
	if contentEnd < n && e.text[contentEnd] == '\n' {
		delEnd = contentEnd + 1
	} else if contentStart > 0 {
		delStart = contentStart - 1
	}
	return contentStart, contentEnd, delStart, delEnd
}

// lineSpanDelta returns the line-wise content and delete span covering the
// current line plus delta lines (+count for j, -count for k).
func (e *Editor) lineSpanDelta(p, delta int) (contentStart, contentEnd, delStart, delEnd int) {
	starts := []int{0}
	for i, r := range e.text {
		if r == '\n' {
			starts = append(starts, i+1)
		}
	}
	cur := 0
	for i, s := range starts {
		if s <= p {
			cur = i
		}
	}
	other := cur + delta
	if other < 0 {
		other = 0
	}
	if other >= len(starts) {
		other = len(starts) - 1
	}
	first, last := cur, other
	if first > last {
		first, last = last, first
	}
	contentStart = starts[first]
	contentEnd = len(e.text)
	if last+1 < len(starts) {
		contentEnd = starts[last+1] - 1
	}
	delStart, delEnd = contentStart, contentEnd
	if contentEnd < len(e.text) && e.text[contentEnd] == '\n' {
		delEnd = contentEnd + 1
	} else if contentStart > 0 {
		delStart = contentStart - 1
	}
	return contentStart, contentEnd, delStart, delEnd
}

// rangeFor maps an operator motion onto a half-open [start,end) span.
func (e *Editor) rangeFor(m rune, count int) (int, int, bool) {
	start := e.cursor
	target, ok := e.move(m, count)
	if !ok {
		return 0, 0, false
	}
	switch m {
	case 'b', 'B', '0', '^', 'h', '{':
		return target, start, true
	case '}':
		return start, target, true
	case '$':
		_, end, _, _ := e.lineAt(e.cursor)
		return e.cursor, end, true
	case '%':
		lo, hi := start, target
		if hi < lo {
			lo, hi = hi, lo
		}
		end := hi + 1
		if end > len(e.text) {
			end = len(e.text)
		}
		return lo, end, true
	case 'F', 'T':
		// Backward character search is exclusive of cursor position in Vim!
		return target, start, true
	case ';', ',':
		effK := e.findKind
		if m == ',' {
			effK = reverseFindKind(effK)
		}
		if effK == 'F' || effK == 'T' {
			return target, start, true
		}
		end := target + 1
		if end > len(e.text) {
			end = len(e.text)
		}
		return start, end, true
	}
	if target < start {
		return target, start, true
	}
	// Forward motions are inclusive of the target character, except w, W, l, space
	// which stop at the exclusive boundary.
	if m == 'w' || m == 'W' || m == 'l' || m == ' ' {
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
	e.changeObj = 0
	e.changeAround = false
	e.changeFindK = 0
	e.changeFindCh = 0
	e.changeRepeat = false
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
	obj := e.changeObj
	around := e.changeAround
	findK := e.changeFindK
	findCh := e.changeFindCh
	findRep := e.changeRepeat
	count := e.changeCount
	from, to := e.changeFrom, e.cursor
	if from < 0 {
		from = 0
	}
	if to < 0 {
		to = 0
	}
	if from > len(e.text) {
		from = len(e.text)
	}
	if to > len(e.text) {
		to = len(e.text)
	}
	// Backspace, Ctrl+U, and left can move the cursor behind the insert
	// start. Slicing [from:to] then panics. Keep the repeat text empty.
	if from > to {
		from = to
	}
	ins := append([]rune(nil), e.text[from:to]...)
	e.lastEdit = func(ed *Editor) {
		switch kind {
		case 'i', 'I':
			if kind == 'I' {
				ed.cursor, _ = ed.move('^', 1)
			}
			ed.insertRunes(ins)
		case 'a':
			if ed.cursor < len(ed.text) {
				ed.cursor++
			}
			ed.insertRunes(ins)
		case 'A':
			_, end, _, _ := ed.lineAt(ed.cursor)
			ed.cursor = end
			ed.insertRunes(ins)
		case 'C', '$':
			_, end, _, _ := ed.lineAt(ed.cursor)
			ed.deleteRange(ed.cursor, end)
			ed.insertRunes(ins)
		case 'L', 'S':
			cs, ce, _, _ := ed.lineAt(ed.cursor)
			ed.deleteRange(cs, ce)
			ed.cursor = cs
			ed.insertRunes(ins)
		case 's':
			end := ed.cursor + count
			if end > len(ed.text) {
				end = len(ed.text)
			}
			ed.deleteRange(ed.cursor, end)
			ed.insertRunes(ins)
		case 'O': // text object repeat
			if s, en, ok := ed.textObjectSpan(around, obj, count); ok {
				ed.deleteRange(s, en)
				ed.insertRunes(ins)
			}
		case 'F': // find-char repeat ('cf', 'ct', 'cF', 'cT', 'c;', 'c,')
			start := ed.cursor
			target, ok := ed.findCharLine(findK, findCh, count, findRep)
			if !ok {
				return
			}
			var s, en int
			if findK == 'F' || findK == 'T' {
				s, en = target, start
			} else {
				s, en = start, target+1
				if en > len(ed.text) {
					en = len(ed.text)
				}
			}
			ed.deleteRange(s, en)
			ed.insertRunes(ins)
		case 'g': // cge / cgE repeat
			st := ed.cursor
			q := st
			for i := 0; i < count; i++ {
				q = ed.wordEndBack(q, obj == 'E')
			}
			end := st + 1
			if end > len(ed.text) {
				end = len(ed.text)
			}
			ed.deleteRange(q, end)
			ed.insertRunes(ins)
		default:
			if s, en, ok := ed.rangeFor(kind, count); ok {
				ed.deleteRange(s, en)
				ed.insertRunes(ins)
			}
		}
	}
}

func (e *Editor) operateSpan(op rune, start, end int) {
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end > len(e.text) {
		end = len(e.text)
	}
	switch op {
	case 'y':
		e.reg = append([]rune(nil), e.text[start:end]...)
	case 'd':
		e.deleteRange(start, end)
	case 'c':
		e.deleteRange(start, end)
		e.mode = ModeInsert
	}
}

func (e *Editor) operateFind(op, kind, ch rune, count int, isRepeat bool) {
	start := e.cursor
	target, ok := e.findCharLine(kind, ch, count, isRepeat)
	if !ok {
		return
	}
	var s, en int
	if kind == 'F' || kind == 'T' {
		s, en = target, start
	} else {
		s, en = start, target+1
		if en > len(e.text) {
			en = len(e.text)
		}
	}
	switch op {
	case 'y':
		e.reg = append([]rune(nil), e.text[s:en]...)
	case 'd':
		e.deleteRange(s, en)
		fk, fc, cc, fr := kind, ch, count, isRepeat
		e.lastEdit = func(ed *Editor) {
			ed.operateFind('d', fk, fc, cc, fr)
		}
	case 'c':
		e.deleteRange(s, en)
		e.beginChange('F', count)
		e.changeFindK = kind
		e.changeFindCh = ch
		e.changeRepeat = isRepeat
		e.mode = ModeInsert
	}
}

func (e *Editor) operateTextObject(op rune, around bool, obj rune, count int) {
	s, en, ok := e.textObjectSpan(around, obj, count)
	if !ok {
		return
	}
	switch op {
	case 'y':
		e.reg = append([]rune(nil), e.text[s:en]...)
	case 'd':
		e.deleteRange(s, en)
		ar, ob, cc := around, obj, count
		e.lastEdit = func(ed *Editor) {
			if a, b, ok := ed.textObjectSpan(ar, ob, cc); ok {
				ed.deleteRange(a, b)
			}
		}
	case 'c':
		e.deleteRange(s, en)
		e.beginChange('O', count)
		e.changeObj = obj
		e.changeAround = around
		e.mode = ModeInsert
	}
}

func (e *Editor) operateG(op, sub rune, count int, explicitCount bool) {
	switch sub {
	case 'g':
		// dgg / cgg / ygg: linewise from line 1 (or line `count` if explicit) through current line.
		lines := e.lineBounds()
		curLine := e.lineIndexAt(lines, e.cursor)
		targetLine := 0
		if explicitCount && count-1 < len(lines) {
			targetLine = count - 1
		}
		cs, ce, ds, de := e.lineSpanDelta(e.cursor, targetLine-curLine)
		switch op {
		case 'y':
			e.reg = append([]rune(nil), e.text[ds:de]...)
		case 'd':
			e.deleteRange(ds, de)
			tl, exp := targetLine, explicitCount
			e.lastEdit = func(ed *Editor) {
				ls := ed.lineBounds()
				cl := ed.lineIndexAt(ls, ed.cursor)
				tgt := 0
				if exp && tl < len(ls) {
					tgt = tl
				}
				_, _, a, b := ed.lineSpanDelta(ed.cursor, tgt-cl)
				ed.deleteRange(a, b)
			}
		case 'c':
			e.deleteRange(cs, ce)
			e.cursor = cs
			e.beginChange('L', 1)
			e.mode = ModeInsert
		}
	case 'e', 'E':
		// dge / dgE: backward to end of previous word (inclusive of start char in Vim!).
		start := e.cursor
		p := start
		for i := 0; i < count; i++ {
			p = e.wordEndBack(p, sub == 'E')
		}
		lo, hi := p, start+1
		if hi > len(e.text) {
			hi = len(e.text)
		}
		if lo > hi {
			lo = hi
		}
		e.operateSpan(op, lo, hi)
		if op == 'd' {
			big, cc := sub == 'E', count
			e.lastEdit = func(ed *Editor) {
				st := ed.cursor
				q := st
				for i := 0; i < cc; i++ {
					q = ed.wordEndBack(q, big)
				}
				end := st + 1
				if end > len(ed.text) {
					end = len(ed.text)
				}
				ed.deleteRange(q, end)
			}
		} else if op == 'c' {
			insStart := e.cursor
			e.beginChange('g', count)
			e.changeObj = sub
			e.changeFrom = insStart
		}
	case '_':
		// dg_ / cg_ / yg_: through last non-blank character on line (inclusive!).
		start := e.cursor
		target := e.lastNonBlankOnLine(start, count)
		lo, hi := start, target+1
		if target < start {
			lo, hi = target, start
		}
		if hi > len(e.text) {
			hi = len(e.text)
		}
		e.operateSpan(op, lo, hi)
		if op == 'd' {
			cc := count
			e.lastEdit = func(ed *Editor) {
				st := ed.cursor
				tgt := ed.lastNonBlankOnLine(st, cc)
				a, b := st, tgt+1
				if tgt < st {
					a, b = tgt, st
				}
				if b > len(ed.text) {
					b = len(ed.text)
				}
				ed.deleteRange(a, b)
			}
		} else if op == 'c' {
			e.beginChange('$', 1)
		}
	}
}

func (e *Editor) operate(op, m rune, count int) {
	if m == op {
		delta := count - 1
		if delta < 0 {
			delta = 0
		}
		cs, ce, ds, de := e.lineSpanDelta(e.cursor, delta)
		switch op {
		case 'y':
			e.reg = append([]rune(nil), e.text[ds:de]...)
		case 'd':
			e.deleteRange(ds, de)
			d := delta
			e.lastEdit = func(ed *Editor) {
				_, _, a, b := ed.lineSpanDelta(ed.cursor, d)
				ed.deleteRange(a, b)
			}
		case 'c':
			e.deleteRange(cs, ce)
			e.cursor = cs
			e.beginChange('L', 1)
			e.mode = ModeInsert
		}
		return
	}
	if m == 'G' {
		lines := e.lineBounds()
		curLine := e.lineIndexAt(lines, e.cursor)
		targetLine := len(lines) - 1
		cs, ce, ds, de := e.lineSpanDelta(e.cursor, targetLine-curLine)
		switch op {
		case 'y':
			e.reg = append([]rune(nil), e.text[ds:de]...)
		case 'd':
			e.deleteRange(ds, de)
			e.lastEdit = func(ed *Editor) {
				ls := ed.lineBounds()
				cl := ed.lineIndexAt(ls, ed.cursor)
				_, _, a, b := ed.lineSpanDelta(ed.cursor, (len(ls)-1)-cl)
				ed.deleteRange(a, b)
			}
		case 'c':
			e.deleteRange(cs, ce)
			e.cursor = cs
			e.beginChange('L', 1)
			e.mode = ModeInsert
		}
		return
	}
	if m == ';' || m == ',' {
		if e.findCh == 0 {
			return
		}
		k := e.findKind
		if k == 0 {
			if e.findDir < 0 {
				k = 'F'
			} else {
				k = 'f'
			}
		}
		if m == ',' {
			k = reverseFindKind(k)
		}
		e.operateFind(op, k, e.findCh, count, true)
		return
	}
	if m == 'j' || m == 'k' {
		delta := count
		if m == 'k' {
			delta = -count
		}
		cs, ce, ds, de := e.lineSpanDelta(e.cursor, delta)
		switch op {
		case 'y':
			e.reg = append([]rune(nil), e.text[ds:de]...)
		case 'd':
			e.deleteRange(ds, de)
			d := delta
			e.lastEdit = func(ed *Editor) {
				_, _, a, b := ed.lineSpanDelta(ed.cursor, d)
				ed.deleteRange(a, b)
			}
		case 'c':
			e.deleteRange(cs, ce)
			e.cursor = cs
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
	if key == "" || utf8.RuneCountInString(key) != 1 {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(key)
	if r < 0x20 || r == 0x7f || unicode.IsControl(r) {
		return 0, false
	}
	return r, true
}

func (e *Editor) clearPendingState() {
	e.pending = 0
	e.pendingOp = 0
	e.opCount = 0
	e.count = 0
}

func (e *Editor) totalOpCount() int {
	oc := e.opCount
	if oc <= 0 {
		oc = 1
	}
	e.opCount = 0
	return oc * e.takeCount()
}

func (e *Editor) Feed(key string) Action {
	// A pending r, f, F, t or T is waiting for one literal character (never treat digits as counts here!).
	if e.pending == 'r' {
		e.clearPendingState()
		if r, ok := e.printable(key); ok {
			e.replaceChar(r)
			rr := r
			e.lastEdit = func(ed *Editor) { ed.replaceChar(rr) }
		}
		return ActStay
	}
	if e.pending == 'f' || e.pending == 'F' || e.pending == 't' || e.pending == 'T' {
		m := e.pending
		op := e.pendingOp
		cnt := e.totalOpCount()
		e.pending = 0
		e.pendingOp = 0
		if key == "esc" {
			return ActStay
		}
		if r, ok := e.printable(key); ok {
			e.findCh = r
			e.findKind = m
			if m == 'f' || m == 't' {
				e.findDir = 1
			} else {
				e.findDir = -1
			}
			if op != 0 {
				e.operateFind(op, m, r, cnt, false)
			} else if p, ok := e.findCharLine(m, r, cnt, false); ok {
				e.cursor = p
			}
		}
		return ActStay
	}

	// Text-object selector ('i' or 'a' under an operator).
	if (e.pending == 'i' || e.pending == 'a') && e.pendingOp != 0 {
		around := e.pending == 'a'
		op := e.pendingOp
		cnt := e.totalOpCount()
		e.pending = 0
		e.pendingOp = 0
		if key == "esc" {
			return ActStay
		}
		if obj, ok := e.printable(key); ok {
			e.operateTextObject(op, around, obj, cnt)
		}
		return ActStay
	}

	// 'g' sub-motion under an operator (dgg, dge, dgE, dg_).
	if e.pending == 'g' && e.pendingOp != 0 {
		op := e.pendingOp
		explicit := e.opCount > 0 || e.count > 0
		cnt := e.totalOpCount()
		e.pending = 0
		e.pendingOp = 0
		if key == "esc" {
			return ActStay
		}
		if sub, ok := e.printable(key); ok {
			e.operateG(op, sub, cnt, explicit)
		}
		return ActStay
	}

	// Numeric prefix (allowed both before an operator and between operator and motion, e.g. d2w / d2fx!).
	if e.mode == ModeNormal {
		if r, ok := e.printable(key); ok {
			if (r >= '1' && r <= '9') || (r == '0' && e.count > 0) {
				e.count = e.count*10 + int(r-'0')
				return ActStay
			}
		}
	}

	// An operator is waiting for its motion or text-object prefix.
	if e.pending == 'd' || e.pending == 'c' || e.pending == 'y' {
		op := e.pending
		if key == "esc" {
			e.clearPendingState()
			return ActStay
		}
		if key == string(op) {
			cnt := e.totalOpCount()
			e.pending = 0
			e.pendingOp = 0
			e.operate(op, op, cnt)
			return ActStay
		}
		if key == "left" || key == "bs" {
			cnt := e.totalOpCount()
			e.pending = 0
			e.pendingOp = 0
			e.operate(op, 'h', cnt)
			return ActStay
		}
		if key == "right" {
			cnt := e.totalOpCount()
			e.pending = 0
			e.pendingOp = 0
			e.operate(op, 'l', cnt)
			return ActStay
		}
		if m, ok := e.printable(key); ok {
			if m == 'f' || m == 'F' || m == 't' || m == 'T' || m == 'i' || m == 'a' || m == 'g' {
				// Preserve accumulated count across prefix (e.g. d2fx or d2iw).
				if e.count > 0 {
					if e.opCount <= 0 {
						e.opCount = 1
					}
					e.opCount *= e.count
					e.count = 0
				}
				e.pending = m
				e.pendingOp = op
				return ActStay
			}
			if m == 'G' && (e.opCount > 0 || e.count > 0) {
				explicitLine := e.totalOpCount()
				e.pending = 0
				e.pendingOp = 0
				e.operateG(op, 'g', explicitLine, true)
				return ActStay
			}
			cnt := e.totalOpCount()
			e.pending = 0
			e.pendingOp = 0
			e.operate(op, m, cnt)
			return ActStay
		}
		e.clearPendingState()
		return ActStay
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
		case 'w', 'W', 'b', 'B', 'e', 'E', '0', '^', '_', '$', ' ':
			p, _ := e.move(r, e.takeCount())
			e.cursor = p
			return ActStay
		case '%', '{', '}':
			if p, ok := e.move(r, e.takeCount()); ok {
				e.cursor = p
			}
			return ActStay
		case 'f', 'F', 't', 'T':
			e.opCount = 0
			e.pending = r
			return ActStay
		case ';', ',':
			if p, ok := e.move(r, e.takeCount()); ok {
				e.cursor = p
			}
			return ActStay
		case 'i':
			e.count = 0
			e.mode = ModeInsert
			e.beginChange('i', 1)
			return ActStay
		case 'I':
			e.count = 0
			e.mode = ModeInsert
			e.cursor, _ = e.move('^', 1)
			e.beginChange('I', 1)
			return ActStay
		case 'a':
			e.count = 0
			e.mode = ModeInsert
			if e.cursor < len(e.text) && e.text[e.cursor] != '\n' {
				e.cursor++
			}
			e.beginChange('a', 1)
			return ActStay
		case 'A':
			e.count = 0
			e.mode = ModeInsert
			_, end, _, _ := e.lineAt(e.cursor)
			e.cursor = end
			e.beginChange('A', 1)
			return ActStay
		case 'S':
			e.count = 0
			cs, ce, _, _ := e.lineAt(e.cursor)
			e.deleteRange(cs, ce)
			e.cursor = cs
			e.beginChange('S', 1)
			e.mode = ModeInsert
			return ActStay
		case 'J':
			cnt := e.takeCount()
			e.joinLines(cnt)
			cc := cnt
			e.lastEdit = func(ed *Editor) { ed.joinLines(cc) }
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
			e.opCount = 0
			if e.count > 0 {
				e.opCount = e.takeCount()
			}
			e.pending = r
			return ActStay
		case 'D':
			_, end, _, _ := e.lineAt(e.cursor)
			e.deleteRange(e.cursor, end)
			e.lastEdit = func(ed *Editor) {
				_, lineEnd, _, _ := ed.lineAt(ed.cursor)
				ed.deleteRange(ed.cursor, lineEnd)
			}
			return ActStay
		case 'C':
			_, end, _, _ := e.lineAt(e.cursor)
			e.deleteRange(e.cursor, end)
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
		if e.pending != 0 || e.pendingOp != 0 || e.count > 0 || e.opCount > 0 {
			e.clearPendingState()
			return ActStay
		}
		return ActOut
	case "bs":
		p, _ := e.move('h', e.takeCount())
		e.cursor = p
		return ActStay
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
	pend := e.pending
	if e.pendingOp != 0 {
		pend = e.pendingOp
	}
	cnt := e.count
	if e.opCount > 0 {
		if cnt > 0 {
			cnt = e.opCount * cnt
		} else {
			cnt = e.opCount
		}
	}
	return View{
		Label:   label,
		Text:    e.Text(),
		Cursor:  e.cursor,
		Mode:    e.mode,
		Pending: pend,
		Count:   cnt,
		Focused: focused,
	}
}
