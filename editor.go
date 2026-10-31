package vimline

// Package vimline is a framework-neutral vim line Editor: modal insert,
// normal, and replace states, motions, operators with counts, one register,
// undo and redo, and the jj exit idiom. It renders nothing; every consumer
// paints the View it gets.

import (
	"regexp"
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
	text          []rune
	cursor        int
	marks         map[rune]int
	redoCursor    int
	hasRedoCursor bool
}

type replaceStep struct {
	pos      int
	original []rune
	inserted bool
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

	reg      []rune
	regMarks map[rune]int
	// regLinewise distinguishes yy/dd-style puts from characterwise puts.
	regLinewise bool

	undo []undoSnap
	redo []undoSnap

	findCh   rune
	findKind rune // 'f', 'F', 't', 'T'
	findDir  int

	// change tracks one insert session so '.' can replay it.
	inChange      bool
	changeM       rune
	changeObj     rune
	changeAround  bool
	changeFindK   rune
	changeFindCh  rune
	changeRepeat  bool
	changeSpan    func(*Editor, int) (int, int, bool)
	changeCount   int
	changeFrom    int
	changeUndoAt  int
	dotCount      int
	dotOverride   int
	changeStart   []rune
	insertCount   int
	historyCursor int
	redoCursor    int
	replaceSteps  []replaceStep
	marks         map[rune]int
	searching     bool
	searchInput   []rune
	searchDir     int
	searchCount   int
	searchOp      rune
	lastSearch    []rune
	lastSearchDir int
	lineUndo      undoSnap
	hasLineUndo   bool
	lineUndoStart int

	lastEdit func(*Editor)
}

func New(s string) *Editor {
	rs := []rune(s)
	e := &Editor{text: rs, cursor: len(rs), mode: ModeInsert, marks: make(map[rune]int)}
	e.beginChange('i', 1)
	return e
}

func (e *Editor) String() string { return string(e.text) }

// Text returns the buffer contents.
func (e *Editor) Text() string { return string(e.text) }

// Register returns the unnamed register, matching Vim's default register.
func (e *Editor) Register() string { return string(e.reg) }

// CanUndo and CanRedo expose logical history availability for consumers and
// differential tests without exposing the snapshot representation.
func (e *Editor) CanUndo() bool { return len(e.undo) > 0 }
func (e *Editor) CanRedo() bool { return len(e.redo) > 0 }

// SetMode forces the editing mode. Consumers use it to refocus a field in
// insert after a mode change elsewhere.
func (e *Editor) SetMode(m Mode) {
	if m != e.mode && e.inChange {
		// SetMode is a host reset, not a typed Vim mode transition. Do not let
		// the implicit New() insert transaction absorb a later normal command.
		e.inChange = false
	}
	e.mode = m
	if m == ModeNormal {
		e.normalCursorClamp()
	}
}

// End puts the cursor after the last character.
func (e *Editor) End() { e.cursor = len(e.text) }

// SetCursor moves the cursor and clamps it to the buffer.
func (e *Editor) SetCursor(p int) {
	e.cursor = p
	e.clamp()
	if e.mode == ModeNormal {
		e.normalCursorClamp()
	}
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
	ls, _, _, _ := e.lineAt(e.cursor)
	if !e.hasLineUndo || ls != e.lineUndoStart {
		e.lineUndo = e.currentSnap()
		e.hasLineUndo = true
		e.lineUndoStart = ls
	}
	e.undo = append(e.undo, undoSnap{text: append([]rune(nil), e.text...), cursor: e.cursor, marks: cloneMarks(e.marks)})
	e.redo = e.redo[:0]
}

func (e *Editor) undoLineEdit() {
	if !e.hasLineUndo {
		return
	}
	current := e.currentSnap()
	e.undo = append(e.undo, current)
	e.redo = nil
	target := e.lineUndo
	e.text = append([]rune(nil), target.text...)
	e.cursor = target.cursor
	e.marks = cloneMarks(target.marks)
	e.lineUndo = current
	e.lineUndoStart, _, _, _ = e.lineAt(e.cursor)
	e.normalCursorClamp()
}

func cloneMarks(src map[rune]int) map[rune]int {
	dst := make(map[rune]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (e *Editor) setLastUndoRedoCursor(cursor int) {
	if len(e.undo) == 0 {
		return
	}
	e.undo[len(e.undo)-1].redoCursor = cursor
	e.undo[len(e.undo)-1].hasRedoCursor = true
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

func snapshotsEqual(a, b undoSnap) bool {
	if a.cursor != b.cursor || len(a.text) != len(b.text) {
		return false
	}
	for i := range a.text {
		if a.text[i] != b.text[i] {
			return false
		}
	}
	return true
}

func (e *Editor) currentSnap() undoSnap {
	return undoSnap{text: append([]rune(nil), e.text...), cursor: e.cursor, marks: cloneMarks(e.marks)}
}

// runAtomic collapses any per-rune/per-operation snapshots produced by fn into
// one Vim change. This is used by dot-repeat and other compound edits.
func (e *Editor) runAtomic(fn func()) { e.runAtomicWithNoop(fn, false) }

func (e *Editor) runAtomicWithNoop(fn func(), recordNoop bool) {
	before := e.currentSnap()
	base := len(e.undo)
	oldRedo := append([]undoSnap(nil), e.redo...)
	fn()
	if snapshotsEqual(before, e.currentSnap()) {
		if recordNoop {
			ls, le, _, _ := e.lineAt(before.cursor)
			before.cursor = ls
			for before.cursor < le && isSpaceNotNL(e.text[before.cursor]) {
				before.cursor++
			}
			before.redoCursor = before.cursor
			before.hasRedoCursor = true
			e.undo = append(e.undo[:base], before)
			e.redo = nil
		} else {
			e.undo = e.undo[:base]
			e.redo = oldRedo
		}
		return
	}
	before.redoCursor = before.cursor
	before.hasRedoCursor = true
	e.undo = append(e.undo[:base], before)
	e.redo = nil
}

func (e *Editor) normalCursorClamp() {
	if len(e.text) == 0 {
		e.cursor = 0
		return
	}
	if e.cursor == len(e.text) && e.text[len(e.text)-1] == '\n' {
		return
	}
	if e.cursor >= len(e.text) {
		e.cursor = len(e.text) - 1
	}
	if e.cursor < 0 {
		e.cursor = 0
	}
	if e.cursor < len(e.text) && (isGraphemeExtend(e.text[e.cursor]) || e.text[e.cursor] == '\u200d' || (e.cursor > 0 && e.text[e.cursor-1] == '\u200d')) {
		e.cursor = e.prevGrapheme(e.cursor + 1)
	}
	ls, _, _, _ := e.lineAt(e.cursor)
	if e.text[e.cursor] == '\n' && e.cursor > ls {
		e.cursor--
	}
}

func (e *Editor) cursorColumn(p int) int {
	ls, _, _, _ := e.lineAt(p)
	return p - ls
}

func (e *Editor) lineIndent(p int) []rune {
	ls, le, _, _ := e.lineAt(p)
	i := ls
	for i < le && isSpaceNotNL(e.text[i]) {
		i++
	}
	return append([]rune(nil), e.text[ls:i]...)
}

func (e *Editor) searchPattern(pattern []rune, dir, count int) (int, bool) {
	if len(pattern) == 0 {
		pattern = e.lastSearch
	}
	if len(pattern) == 0 {
		return e.cursor, false
	}
	re, err := regexp.Compile(string(pattern))
	if err != nil {
		return e.cursor, false
	}
	text := string(e.text)
	byteToRune := func(b int) int { return utf8.RuneCountInString(text[:b]) }
	var matches []int
	for _, loc := range re.FindAllStringIndex(text, -1) {
		matches = append(matches, byteToRune(loc[0]))
	}
	if len(matches) == 0 {
		return e.cursor, false
	}
	if count < 1 {
		count = 1
	}
	target := e.cursor
	for range count {
		found := false
		if dir >= 0 {
			for _, p := range matches {
				if p > target {
					target, found = p, true
					break
				}
			}
			if !found {
				target, found = matches[0], true
			}
		} else {
			for i := len(matches) - 1; i >= 0; i-- {
				if matches[i] < target {
					target, found = matches[i], true
					break
				}
			}
			if !found {
				target, found = matches[len(matches)-1], true
			}
		}
	}
	return target, true
}

func (e *Editor) percentLine(percent int) (int, bool) {
	if percent < 1 || percent > 100 {
		return e.cursor, false
	}
	lines := e.lineBounds()
	line := (len(lines)*percent + 99) / 100
	if line < 1 {
		line = 1
	}
	if line > len(lines) {
		line = len(lines)
	}
	p := lines[line-1][0] + e.cursorColumn(e.cursor)
	if p >= lines[line-1][1] && lines[line-1][1] > lines[line-1][0] {
		p = lines[line-1][1] - 1
	} else if lines[line-1][1] == lines[line-1][0] {
		p = lines[line-1][0]
	}
	return p, true
}

func (e *Editor) unmatchedBracket(prefix, sub rune, count int) (int, bool) {
	var open, close rune
	switch sub {
	case '(', ')':
		open, close = '(', ')'
	case '{', '}':
		open, close = '{', '}'
	default:
		return e.cursor, false
	}
	if count < 1 {
		count = 1
	}
	p := e.cursor
	for range count {
		depth, found := 0, false
		if prefix == '[' && sub == open {
			for i := p - 1; i >= 0; i-- {
				if e.text[i] == close {
					depth++
				} else if e.text[i] == open {
					if depth == 0 {
						p, found = i, true
						break
					}
					depth--
				}
			}
		} else if prefix == ']' && sub == close {
			for i := p + 1; i < len(e.text); i++ {
				if e.text[i] == open {
					depth++
				} else if e.text[i] == close {
					if depth == 0 {
						p, found = i, true
						break
					}
					depth--
				}
			}
		}
		if !found {
			return e.cursor, false
		}
	}
	return p, true
}

func (e *Editor) operatePercent(op rune, percent int) {
	target, ok := e.percentLine(percent)
	if !ok {
		return
	}
	lines := e.lineBounds()
	cur := e.lineIndexAt(lines, e.cursor)
	tgt := e.lineIndexAt(lines, target)
	column := e.cursorColumn(e.cursor)
	cs, ce, ds, de, _ := e.lineSpanDelta(e.cursor, tgt-cur)
	reg := e.linewiseRegister(cs, ce)
	switch op {
	case 'y':
		e.reg, e.regLinewise = reg, true
	case 'd':
		e.deleteRange(ds, de)
		e.reg, e.regLinewise = reg, true
		e.placeAtColumn(ds, column)
	case 'c':
		e.deleteRange(ds, de)
		e.reg, e.regLinewise = reg, true
		e.cursor = ds
		e.beginChange('L', 1)
		e.mode = ModeInsert
	}
}

func (e *Editor) operateIndent(op, motion rune, count int) {
	lines := e.lineBounds()
	if len(lines) > 1 && lines[len(lines)-1][0] == len(e.text) && len(e.text) > 0 && e.text[len(e.text)-1] == '\n' {
		lines = lines[:len(lines)-1]
	}
	cur := e.lineIndexAt(lines, e.cursor)
	first, last := cur, cur
	if motion == op {
		last = cur + count - 1
	} else if motion == 'G' {
		last = len(lines) - 1
	} else if motion == 'j' || motion == '+' {
		last = cur + count
	} else if motion == 'k' || motion == '-' {
		first = cur - count
	} else if s, en, ok := e.rangeFor(motion, count); ok {
		first = e.lineIndexAt(lines, s)
		probe := en
		if probe > s {
			probe--
		}
		last = e.lineIndexAt(lines, probe)
	} else {
		return
	}
	if first < 0 || last >= len(lines) {
		return
	}
	e.pushUndo()
	for line := last; line >= first; line-- {
		start := lines[line][0]
		switch op {
		case '>':
			e.text = append(e.text[:start], append([]rune{'\t'}, e.text[start:]...)...)
		case '<':
			if start < len(e.text) && e.text[start] == '\t' {
				e.text = append(e.text[:start], e.text[start+1:]...)
			} else {
				end := start
				for end < len(e.text) && end < start+8 && e.text[end] == ' ' {
					end++
				}
				e.text = append(e.text[:start], e.text[end:]...)
			}
		case '=':
			end := start
			for end < len(e.text) && isSpaceNotNL(e.text[end]) {
				end++
			}
			e.text = append(e.text[:start], e.text[end:]...)
		}
	}
	lines = e.lineBounds()
	p := lines[first][0]
	if op == '<' && lines[first][1] > lines[first][0] {
		p = lines[first][1] - 1
	}
	e.cursor = p
	oo, mm, cc := op, motion, count
	e.lastEdit = func(ed *Editor) {
		repeat := cc
		if ed.dotCount > 0 {
			repeat = ed.dotCount
		}
		ed.operateIndent(oo, mm, repeat)
	}
}

// placeAtColumn preserves Vim's desired column after a linewise deletion.
func (e *Editor) placeAtColumn(at, column int) {
	if len(e.text) == 0 {
		e.cursor = 0
		return
	}
	if at >= len(e.text) && e.text[len(e.text)-1] == '\n' {
		e.cursor = len(e.text)
		return
	}
	if at >= len(e.text) {
		at = len(e.text) - 1
	}
	if at < 0 {
		at = 0
	}
	ls, le, _, _ := e.lineAt(at)
	if le == ls {
		e.cursor = ls
		return
	}
	e.cursor = ls + column
	if e.cursor >= le {
		e.cursor = le - 1
	}
}

// ---------------------------------------------------------------------------
// motions
// ---------------------------------------------------------------------------

func isWordRune(r rune) bool {
	isVariation := (r >= 0xfe00 && r <= 0xfe0f) || (r >= 0xe0100 && r <= 0xe01ef)
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) ||
		((unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r)) && !isVariation)
}

func isCJK(r rune) bool {
	return (r >= 0x3400 && r <= 0x4dbf) || (r >= 0x4e00 && r <= 0x9fff) ||
		(r >= 0xf900 && r <= 0xfaff) || (r >= 0x20000 && r <= 0x323af)
}

func isGraphemeExtend(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) ||
		unicode.Is(unicode.Me, r) || (r >= 0xfe00 && r <= 0xfe0f) ||
		(r >= 0xe0100 && r <= 0xe01ef) || (r >= 0x1f3fb && r <= 0x1f3ff)
}

func (e *Editor) nextGrapheme(p int) int {
	if p >= len(e.text) {
		return len(e.text)
	}
	i := p + 1
	if e.text[p] >= 0x1f1e6 && e.text[p] <= 0x1f1ff && i < len(e.text) && e.text[i] >= 0x1f1e6 && e.text[i] <= 0x1f1ff {
		i++
	}
	for i < len(e.text) && isGraphemeExtend(e.text[i]) {
		i++
	}
	for i < len(e.text) && e.text[i] == '\u200d' {
		i++
		if i < len(e.text) {
			i++
		}
		for i < len(e.text) && isGraphemeExtend(e.text[i]) {
			i++
		}
	}
	return i
}

func (e *Editor) prevGrapheme(p int) int {
	if p <= 0 {
		return 0
	}
	i := p - 1
	for i > 0 && isGraphemeExtend(e.text[i]) {
		i--
	}
	for i > 1 && e.text[i-1] == '\u200d' {
		i -= 2
		for i > 0 && isGraphemeExtend(e.text[i]) {
			i--
		}
	}
	return i
}

func (e *Editor) wordClass(r rune, big bool) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if big {
		return 1
	}
	if isCJK(r) {
		return 3
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
		if e.text[i] == '\n' && i+1 < n && e.text[i+1] == '\n' {
			return i + 1
		}
		i++
	}
	return i
}

func (e *Editor) wordBack(p int, big bool) int {
	if p <= 0 {
		return 0
	}
	if e.text[p-1] == '\n' && p-1 > 0 && e.text[p-2] == '\n' {
		return p - 1
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
		return n
	}
	cls := e.wordClass(e.text[i], big)
	for i+1 < n && e.wordClass(e.text[i+1], big) == cls {
		i++
	}
	return i
}

// changeWordEnd implements Vim's cw/cW compatibility rule. Unlike a plain e
// motion, cw on the final character of a word changes that character only; it
// does not advance across whitespace into the next word.
func (e *Editor) changeWordEnd(p int, big bool, count int) int {
	n := len(e.text)
	if n == 0 || p >= n {
		return p
	}
	end := p
	if e.text[end] == '\n' {
		return end
	}
	if unicode.IsSpace(e.text[end]) {
		for end+1 < n && e.text[end+1] != '\n' && unicode.IsSpace(e.text[end+1]) {
			end++
		}
		return end
	}
	for step := 0; step < count; step++ {
		if step == 0 && !unicode.IsSpace(e.text[end]) {
			class := e.wordClass(e.text[end], big)
			for end+1 < n && e.wordClass(e.text[end+1], big) == class {
				end++
			}
			continue
		}
		end = e.wordEnd(end, big)
	}
	return end
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
		ls, _, _, _ := e.lineAt(p)
		for range count {
			next := e.prevGrapheme(p)
			if next < ls || next == p {
				break
			}
			p = next
		}
	case 'l', ' ':
		ls, le, _, _ := e.lineAt(p)
		if le == ls {
			return ls, true
		}
		for range count {
			next := e.nextGrapheme(p)
			if next >= le {
				break
			}
			p = next
		}
		limit := le - 1
		if limit < 0 {
			limit = 0
		}
		if p > limit {
			p = limit
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
	case '|':
		ls, le, _, _ := e.lineAt(p)
		p = ls + count - 1
		if le > ls && p >= le {
			p = le - 1
		} else if le == ls {
			p = ls
		}
	case '$':
		for step := 1; step < count; step++ {
			_, le, _, _ := e.lineAt(p)
			if le < n {
				p = le + 1
			}
		}
		ls, le, _, _ := e.lineAt(p)
		if le > ls {
			p = le - 1
		} else {
			p = ls
		}
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
	case '+', '-':
		lines := e.lineBounds()
		cur := e.lineIndexAt(lines, p)
		delta := count
		if m == '-' {
			delta = -count
		}
		target := cur + delta
		if target < 0 {
			target = 0
		}
		if target >= len(lines) {
			target = len(lines) - 1
		}
		if target == cur {
			return p, false
		}
		p = lines[target][0]
		for p < lines[target][1] && isSpaceNotNL(e.text[p]) {
			p++
		}
	case '(', ')':
		return e.sentenceMove(p, m == ')', count)
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
func (e *Editor) lineSpanDelta(p, delta int) (contentStart, contentEnd, delStart, delEnd int, ok bool) {
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
	ok = delta == 0 || other != cur
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
	return contentStart, contentEnd, delStart, delEnd, ok
}

// rangeFor maps an operator motion onto a half-open [start,end) span.
func (e *Editor) rangeFor(m rune, count int) (int, int, bool) {
	start := e.cursor
	target, ok := e.move(m, count)
	if !ok {
		return 0, 0, false
	}
	switch m {
	case 'b', 'B', '0', '^', 'h', '{', '(':
		return target, start, true
	case ')':
		if target == len(e.text)-1 {
			return start, len(e.text), true
		}
		return start, target, true
	case '}':
		ls, le, _, _ := e.lineAt(start)
		blank := isBlankLine(e.text, ls, le)
		firstNonBlank := ls
		for firstNonBlank < le && isSpaceNotNL(e.text[firstNonBlank]) {
			firstNonBlank++
		}
		linewise := blank || start == firstNonBlank
		if linewise {
			start = ls
		}
		if target == len(e.text)-1 && target >= start {
			return start, len(e.text), true
		}
		if !linewise && target > start && target > 0 && e.text[target-1] == '\n' {
			target--
		}
		return start, target, true
	case '$':
		end := target + 1
		if end > len(e.text) {
			end = len(e.text)
		}
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
	if m == 'w' || m == 'W' || m == 'l' || m == ' ' || m == '|' {
		if m == 'w' || m == 'W' {
			if count == 1 && start < len(e.text) && e.text[start] == '\n' {
				return start, start + 1, true
			}
			for i := start; count == 1 && i < target; i++ {
				if e.text[i] == '\n' {
					return start, i, true
				}
			}
		}
		if (m == 'l' || m == ' ') && target == start && start < len(e.text) && e.text[start] != '\n' {
			return start, e.nextGrapheme(start), true
		}
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
	e.regMarks = make(map[rune]int)
	for name, p := range e.marks {
		if p >= a && p < b {
			e.regMarks[name] = p - a
		}
	}
	e.regLinewise = false
	out := make([]rune, 0, len(e.text)-(b-a))
	out = append(out, e.text[:a]...)
	out = append(out, e.text[b:]...)
	e.text = out
	for name, p := range e.marks {
		switch {
		case p >= b:
			e.marks[name] = p - (b - a)
		case p >= a:
			e.marks[name] = a
		}
	}
	e.cursor = a
	e.clamp()
}

func (e *Editor) deleteRangeNoRegister(a, b int) {
	reg := append([]rune(nil), e.reg...)
	regMarks := cloneMarks(e.regMarks)
	linewise := e.regLinewise
	e.deleteRange(a, b)
	e.reg = reg
	e.regMarks = regMarks
	e.regLinewise = linewise
}

func (e *Editor) linewiseRegister(start, end int) []rune {
	reg := append([]rune(nil), e.text[start:end]...)
	return append(reg, '\n')
}

func paragraphRegister(text []rune, around bool, reachesEOF bool) []rune {
	reg := append([]rune(nil), text...)
	if len(reg) == 0 || reg[len(reg)-1] != '\n' || reachesEOF {
		reg = append(reg, '\n')
	}
	return reg
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
	e.changeSpan = nil
	e.changeCount = count
	e.changeFrom = e.cursor
	e.historyCursor = e.cursor
	e.redoCursor = e.cursor
	e.changeStart = append([]rune(nil), e.text...)
	e.insertCount = 1
	e.replaceSteps = e.replaceSteps[:0]
	e.changeUndoAt = len(e.undo)
	// Operator changes and substitute commands call beginChange immediately
	// after their deletion. That deletion's snapshot is the beginning of this
	// logical Vim change and must be retained while later per-rune snapshots are
	// folded into it.
	switch m {
	case 'C', 'L', 'S', 's', 'O', 'o', 'V', 'F', 'g', '$', 'w', 'W', 'e', 'E', 'f', 't', 'T', 'b', 'B', '0', '^', 'h', 'l', ' ', '%', '{', '}':
		if e.changeUndoAt > 0 {
			e.changeUndoAt--
		}
	}
}

// endChange records the finished insert session as the '.' repeat target.
func (e *Editor) endChange() {
	if !e.inChange {
		return
	}
	e.inChange = false
	if len(e.undo) > e.changeUndoAt+1 {
		e.undo = e.undo[:e.changeUndoAt+1]
	}
	if e.changeUndoAt < len(e.undo) {
		e.undo[e.changeUndoAt].cursor = e.historyCursor
		e.undo[e.changeUndoAt].redoCursor = e.redoCursor
		e.undo[e.changeUndoAt].hasRedoCursor = true
	}
	kind := e.changeM
	obj := e.changeObj
	around := e.changeAround
	findK := e.changeFindK
	findCh := e.changeFindCh
	findRep := e.changeRepeat
	count := e.changeCount
	changeSpan := e.changeSpan
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
	e.dotOverride = 0
	e.lastEdit = func(ed *Editor) {
		repeatCount := count
		if ed.dotCount > 0 {
			repeatCount = ed.dotCount
		}
		switch kind {
		case 'i', 'I':
			if kind == 'I' {
				ed.cursor, _ = ed.move('^', 1)
			}
			for range repeatCount {
				ed.insertRunes(ins)
			}
		case 'a':
			if ed.cursor < len(ed.text) {
				ed.cursor++
			}
			for range repeatCount {
				ed.insertRunes(ins)
			}
		case 'A':
			_, end, _, _ := ed.lineAt(ed.cursor)
			ed.cursor = end
			for range repeatCount {
				ed.insertRunes(ins)
			}
		case 'o':
			_, end, _, _ := ed.lineAt(ed.cursor)
			indent := ed.lineIndent(ed.cursor)
			ed.cursor = end
			ed.insertRunes(append([]rune{'\n'}, indent...))
			ed.cursor = end + 1 + len(indent)
			for i := 0; i < repeatCount; i++ {
				if i > 0 {
					ed.insertRunes([]rune{'\n'})
				}
				ed.insertRunes(ins)
			}
		case 'V':
			start, _, _, _ := ed.lineAt(ed.cursor)
			indent := ed.lineIndent(ed.cursor)
			ed.cursor = start
			ed.insertRunes(append(indent, '\n'))
			ed.cursor = start + len(indent)
			for i := 0; i < repeatCount; i++ {
				if i > 0 {
					ed.insertRunes([]rune{'\n'})
				}
				ed.insertRunes(ins)
			}
		case 'C', '$':
			_, end, _, _ := ed.lineAt(ed.cursor)
			ed.deleteRange(ed.cursor, end)
			ed.insertRunes(ins)
		case 'L':
			lineCount := repeatCount
			if lineCount < 1 {
				lineCount = 1
			}
			cs, ce, _, _, ok := ed.lineSpanDelta(ed.cursor, lineCount-1)
			if !ok {
				return
			}
			lineReg := ed.linewiseRegister(cs, ce)
			insertAt := cs
			for insertAt < ce && isSpaceNotNL(ed.text[insertAt]) {
				insertAt++
			}
			ed.deleteRange(insertAt, ce)
			ed.reg = lineReg
			ed.regLinewise = true
			ed.cursor = insertAt
			ed.insertRunes(ins)
		case 'S':
			lineCount := repeatCount
			if lineCount < 1 {
				lineCount = 1
			}
			cs, ce, _, _, ok := ed.lineSpanDelta(ed.cursor, lineCount-1)
			if !ok {
				return
			}
			lineReg := ed.linewiseRegister(cs, ce)
			ed.deleteRange(cs, ce)
			ed.reg = lineReg
			ed.regLinewise = true
			ed.cursor = cs
			ed.insertRunes(ins)
		case 's':
			end := ed.cursor + repeatCount
			if end > len(ed.text) {
				end = len(ed.text)
			}
			ed.deleteRange(ed.cursor, end)
			ed.insertRunes(ins)
		case 'O': // text object repeat
			if s, en, ok := ed.textObjectSpan(around, obj, repeatCount); ok {
				var lineReg []rune
				if obj == 'p' {
					lineReg = paragraphRegister(ed.text[s:en], around, en == len(ed.text))
				}
				if obj == 'p' && en < len(ed.text) && en > s && ed.text[en-1] == '\n' {
					en--
				}
				ed.deleteRange(s, en)
				if obj == 'p' {
					ed.reg = lineReg
					ed.regLinewise = true
				}
				ed.insertRunes(ins)
			}
		case 'w', 'W':
			start := ed.cursor
			end := ed.changeWordEnd(start, kind == 'W', repeatCount) + 1
			if end > len(ed.text) {
				end = len(ed.text)
			}
			ed.deleteRange(start, end)
			ed.insertRunes(ins)
		case 'R':
			for range repeatCount {
				for _, r := range ins {
					if ed.cursor < len(ed.text) && ed.text[ed.cursor] != '\n' {
						ed.text[ed.cursor] = r
						ed.cursor++
					} else {
						ed.insertRunes([]rune{r})
					}
				}
			}
		case 'F': // find-char repeat ('cf', 'ct', 'cF', 'cT', 'c;', 'c,')
			start := ed.cursor
			target, ok := ed.findCharLine(findK, findCh, repeatCount, findRep)
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
			for i := 0; i < repeatCount; i++ {
				q = ed.wordEndBack(q, obj == 'E')
			}
			end := st + 1
			if end > len(ed.text) {
				end = len(ed.text)
			}
			ed.deleteRange(q, end)
			ed.insertRunes(ins)
		default:
			if changeSpan != nil {
				s, en, ok := changeSpan(ed, repeatCount)
				if !ok {
					return
				}
				ed.deleteRange(s, en)
				ed.insertRunes(ins)
			} else if s, en, ok := ed.rangeFor(kind, repeatCount); ok {
				ed.deleteRange(s, en)
				ed.insertRunes(ins)
			}
		}
		if ed.cursor > 0 && !(len(ins) == 0 && (kind == 'o' || kind == 'V')) {
			ed.cursor--
		}
		ed.normalCursorClamp()
	}
}

func (e *Editor) applyInsertCount() {
	if !e.inChange || e.insertCount <= 1 {
		return
	}
	from, to := e.changeFrom, e.cursor
	if from < 0 || from > to || to > len(e.text) {
		return
	}
	typed := append([]rune(nil), e.text[from:to]...)
	for repeat := 1; repeat < e.insertCount; repeat++ {
		switch e.changeM {
		case 'o', 'V':
			e.insertRunes(append([]rune{'\n'}, typed...))
		case 'R':
			for _, r := range typed {
				if e.cursor < len(e.text) && e.text[e.cursor] != '\n' {
					e.pushUndo()
					e.text[e.cursor] = r
					e.cursor++
				} else {
					e.insertRunes([]rune{r})
				}
			}
		default:
			e.insertRunes(typed)
		}
	}
	e.insertCount = 1
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
		e.cursor = start
	case 'd':
		e.cursor = start
		e.deleteRange(start, end)
		e.normalCursorClamp()
		e.setLastUndoRedoCursor(e.cursor)
	case 'c':
		e.deleteRange(start, end)
		e.mode = ModeInsert
	}
}

// markTarget resolves a mark jump to its buffer offset. A ' target lands on
// the first non-blank character of the mark's line.
func (e *Editor) markTarget(kind, r rune) (int, bool) {
	p, exists := e.marks[r]
	if !exists {
		return 0, false
	}
	if kind == '\'' {
		old := e.cursor
		e.cursor = p
		p, _ = e.move('^', 1)
		e.cursor = old
	}
	return p, true
}

// operateMotionSpan applies op over the span a cursor-relative motion resolves
// to and records exact '.' replay for the delete and change forms.
func (e *Editor) operateMotionSpan(op rune, count int, span func(*Editor, int) (int, int, bool)) {
	s, en, ok := span(e, count)
	if !ok {
		return
	}
	if s != en {
		e.operateSpan(op, s, en)
	}
	switch op {
	case 'd':
		cc := count
		e.lastEdit = func(ed *Editor) {
			repeat := cc
			if ed.dotCount > 0 {
				repeat = ed.dotCount
			}
			s, en, ok := span(ed, repeat)
			if !ok || s == en {
				return
			}
			ed.operateSpan('d', s, en)
		}
	case 'c':
		e.beginChange('0', count)
		e.changeSpan = span
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
		e.cursor = s
	case 'd':
		e.cursor = s
		e.deleteRange(s, en)
		e.setLastUndoRedoCursor(s)
		e.normalCursorClamp()
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
	originalCursor := e.cursor
	s, en, ok := e.textObjectSpan(around, obj, count)
	if !ok {
		if obj == 'p' && len(e.text) == 0 {
			e.reg = []rune{'\n'}
			e.regLinewise = true
		}
		if op == 'c' && len(e.text) == 0 && (obj == 'w' || obj == 'W' || obj == 'p') {
			if obj == 'p' {
				if around {
					// cap has no object on an empty buffer; unlike cip it does not
					// enter insert mode.
					return
				}
			}
			e.pushUndo()
			e.beginChange('i', count)
			e.mode = ModeInsert
		} else if (obj == 'w' || obj == 'W') && count > 1 && len(e.text) > 0 {
			e.cursor = len(e.text) - 1
			e.normalCursorClamp()
		}
		return
	}
	paragraphUndoCursor := 0
	if obj == 'p' {
		last := en - 1
		if last >= 0 && last < len(e.text) && e.text[last] == '\n' {
			last--
		}
		if last >= 0 {
			paragraphUndoCursor, _, _, _ = e.lineAt(last)
		}
	}
	switch op {
	case 'y':
		e.reg = append([]rune(nil), e.text[s:en]...)
		if obj == 'p' {
			e.reg = paragraphRegister(e.text[s:en], around, en == len(e.text))
		}
		if obj == 't' && around && s == 0 && en == len(e.text) {
			e.reg = append(e.reg, '\n')
		}
		e.regLinewise = obj == 'p' || (obj == 't' && around && s == 0 && en == len(e.text))
		e.cursor = s
	case 'd':
		column := e.cursorColumn(e.cursor)
		linewiseRegister := append([]rune(nil), e.text[s:en]...)
		deleteStart := s
		if obj == 'p' {
			linewiseRegister = paragraphRegister(e.text[s:en], around, en == len(e.text))
			if !around && en == len(e.text) && s > 0 {
				deleteStart--
			} else if around && en == len(e.text) && s > 0 {
				deleteStart--
			}
		}
		if obj == 't' && around && s == 0 && en == len(e.text) {
			linewiseRegister = append(linewiseRegister, '\n')
		}
		e.cursor = s
		e.deleteRange(deleteStart, en)
		e.setLastUndoRedoCursor(s)
		if obj == 'p' || (obj == 't' && around && s == 0 && en == len(e.text)) {
			e.reg = linewiseRegister
			e.regLinewise = true
		}
		if obj == 'p' {
			e.placeAtColumn(deleteStart, column)
		} else {
			e.normalCursorClamp()
		}
		ar, ob, cc := around, obj, count
		e.lastEdit = func(ed *Editor) {
			ed.operateTextObject('d', ar, ob, cc)
		}
	case 'c':
		if obj == 'p' {
			selLS, selLE, _, _ := e.lineAt(s)
			if around && selLE > selLS && isBlankLine(e.text, selLS, selLE) {
				lineReg := paragraphRegister(e.text[s:en], around, en == len(e.text))
				e.deleteRange(selLE, en)
				e.reg = lineReg
				e.regLinewise = true
				e.cursor = selLE
				e.beginChange('i', count)
				e.mode = ModeInsert
				return
			}
			ls, le, _, _ := e.lineAt(e.cursor)
			if le > ls && isBlankLine(e.text, ls, le) {
				lineReg := paragraphRegister(e.text[s:en], around, en == len(e.text))
				if around {
					e.deleteRange(le, en)
				} else {
					e.pushUndo()
				}
				e.reg = lineReg
				e.regLinewise = true
				e.cursor = le
				e.beginChange('i', count)
				e.mode = ModeInsert
				return
			}
		}
		var linewiseRegister []rune
		if obj == 'p' {
			linewiseRegister = paragraphRegister(e.text[s:en], around, en == len(e.text))
		}
		if obj == 'p' && en > s && en < len(e.text) && e.text[en-1] == '\n' {
			en--
		}
		e.deleteRange(s, en)
		if obj == 'p' {
			e.reg = linewiseRegister
			e.regLinewise = true
		}
		e.cursor = s
		e.beginChange('O', count)
		if obj == 'p' {
			e.historyCursor = paragraphUndoCursor
			e.redoCursor = originalCursor
		}
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
		if explicitCount {
			targetLine = min(count-1, len(lines)-1)
		}
		column := e.cursorColumn(e.cursor)
		cs, ce, ds, de, _ := e.lineSpanDelta(e.cursor, targetLine-curLine)
		lineReg := e.linewiseRegister(cs, ce)
		switch op {
		case 'y':
			e.reg = lineReg
			e.regLinewise = true
		case 'd':
			e.deleteRange(ds, de)
			e.reg = lineReg
			e.regLinewise = true
			e.placeAtColumn(ds, column)
			tl, exp := targetLine, explicitCount
			e.lastEdit = func(ed *Editor) {
				ls := ed.lineBounds()
				cl := ed.lineIndexAt(ls, ed.cursor)
				tgt := 0
				if exp {
					tgt = min(tl, len(ls)-1)
				}
				_, _, a, b, _ := ed.lineSpanDelta(ed.cursor, tgt-cl)
				ed.deleteRange(a, b)
				ed.placeAtColumn(a, column)
			}
		case 'c':
			originalCursor := e.cursor
			insertAt := cs
			for insertAt < ce && isSpaceNotNL(e.text[insertAt]) {
				insertAt++
			}
			if insertAt < ce {
				e.deleteRange(insertAt, ce)
			} else {
				e.pushUndo()
			}
			e.reg = lineReg
			e.regLinewise = true
			e.cursor = insertAt
			e.beginChange('L', 1)
			e.historyCursor = originalCursor
			e.redoCursor = cs
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
	if m == op || m == '_' {
		delta := count - 1
		if delta < 0 {
			delta = 0
		}
		cs, ce, ds, de, ok := e.lineSpanDelta(e.cursor, delta)
		if !ok {
			return
		}
		column := e.cursorColumn(e.cursor)
		lineReg := e.linewiseRegister(cs, ce)
		switch op {
		case 'y':
			e.reg = lineReg
			e.regLinewise = true
		case 'd':
			e.deleteRange(ds, de)
			e.reg = lineReg
			e.regLinewise = true
			e.placeAtColumn(ds, column)
			d := delta
			e.lastEdit = func(ed *Editor) {
				_, _, a, b, ok := ed.lineSpanDelta(ed.cursor, d)
				if !ok {
					return
				}
				ed.deleteRange(a, b)
				ed.placeAtColumn(a, column)
			}
		case 'c':
			insertAt := cs
			for insertAt < ce && isSpaceNotNL(e.text[insertAt]) {
				insertAt++
			}
			if insertAt < ce {
				e.deleteRange(insertAt, ce)
			} else {
				e.pushUndo()
			}
			e.reg = lineReg
			e.regLinewise = true
			e.cursor = insertAt
			e.beginChange('L', 1)
			e.mode = ModeInsert
		}
		return
	}
	if m == 'G' {
		lines := e.lineBounds()
		curLine := e.lineIndexAt(lines, e.cursor)
		targetLine := len(lines) - 1
		column := e.cursorColumn(e.cursor)
		cs, ce, ds, de, _ := e.lineSpanDelta(e.cursor, targetLine-curLine)
		lineReg := e.linewiseRegister(cs, ce)
		switch op {
		case 'y':
			e.reg = lineReg
			e.regLinewise = true
		case 'd':
			e.deleteRange(ds, de)
			e.reg = lineReg
			e.regLinewise = true
			e.placeAtColumn(ds, column)
			e.lastEdit = func(ed *Editor) {
				ls := ed.lineBounds()
				cl := ed.lineIndexAt(ls, ed.cursor)
				_, _, a, b, _ := ed.lineSpanDelta(ed.cursor, (len(ls)-1)-cl)
				ed.deleteRange(a, b)
				ed.placeAtColumn(a, column)
			}
		case 'c':
			originalCursor := e.cursor
			e.deleteRange(cs, ce)
			e.reg = lineReg
			e.regLinewise = true
			e.cursor = cs
			e.beginChange('L', 1)
			if curLine+1 < len(lines) {
				next := lines[curLine+1][0] + column
				if next >= lines[curLine+1][1] {
					next = max(lines[curLine+1][0], lines[curLine+1][1]-1)
				}
				e.historyCursor = next
			} else {
				e.historyCursor = originalCursor
			}
			e.redoCursor = cs
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
	if m == 'j' || m == 'k' || m == '+' || m == '-' {
		delta := count
		if m == 'k' || m == '-' {
			delta = -count
		}
		cs, ce, ds, de, ok := e.lineSpanDelta(e.cursor, delta)
		if !ok {
			return
		}
		column := e.cursorColumn(e.cursor)
		lineReg := e.linewiseRegister(cs, ce)
		switch op {
		case 'y':
			e.reg = lineReg
			e.regLinewise = true
		case 'd':
			e.deleteRange(ds, de)
			e.reg = lineReg
			e.regLinewise = true
			e.placeAtColumn(ds, column)
			d := delta
			e.lastEdit = func(ed *Editor) {
				_, _, a, b, ok := ed.lineSpanDelta(ed.cursor, d)
				if !ok {
					return
				}
				ed.deleteRange(a, b)
				ed.placeAtColumn(a, column)
			}
		case 'c':
			e.deleteRange(cs, ce)
			e.reg = lineReg
			e.regLinewise = true
			e.cursor = cs
			e.beginChange('L', 1)
			e.mode = ModeInsert
		}
		return
	}
	originalMotion := m
	var start, end int
	var ok bool
	if op == 'c' && (m == 'w' || m == 'W') {
		if len(e.text) == 0 {
			e.pushUndo()
			e.beginChange(m, count)
			e.mode = ModeInsert
			return
		}
		if e.cursor == len(e.text) {
			e.pushUndo()
			e.beginChange(m, count)
			e.mode = ModeInsert
			return
		}
		start = e.cursor
		if count > 1 && e.text[start] == '\n' {
			end, _ = e.move(m, count)
		} else {
			end = e.changeWordEnd(start, m == 'W', count) + 1
		}
		if end > len(e.text) {
			end = len(e.text)
		}
		ok = end > start
	} else {
		start, end, ok = e.rangeFor(m, count)
	}
	if !ok {
		return
	}
	beforeLen := len(e.text)
	paragraphLinewise := false
	paragraphBlank := false
	if originalMotion == '}' {
		ls, le, _, _ := e.lineAt(start)
		paragraphBlank = isBlankLine(e.text, ls, le)
		paragraphLinewise = start == ls || paragraphBlank
	}
	switch op {
	case 'y':
		e.reg = append([]rune(nil), e.text[start:end]...)
		e.cursor = start
	case 'd':
		registerStart := start
		deleteStart := start
		beforeText := append([]rune(nil), e.text...)
		if originalMotion == '}' {
			ls, le, _, _ := e.lineAt(start)
			if isBlankLine(e.text, ls, le) && start == ls && start > 0 {
				deleteStart--
			}
		}
		wordLinewise := false
		if (originalMotion == 'w' || originalMotion == 'W') && count > 1 && end == beforeLen {
			ls, _, _, _ := e.lineAt(start)
			wordLinewise = start == ls
			if wordLinewise && start > 0 {
				deleteStart--
			}
		}
		e.cursor = deleteStart
		e.deleteRange(deleteStart, end)
		if registerStart != deleteStart && registerStart < end {
			e.reg = append([]rune(nil), beforeText[registerStart:end]...)
		}
		if wordLinewise && (len(e.reg) == 0 || e.reg[len(e.reg)-1] != '\n') {
			e.reg = append(e.reg, '\n')
			e.regLinewise = true
		}
		if originalMotion == '}' && end == beforeLen && paragraphLinewise && (start == 0 || paragraphBlank) {
			// Paragraph-to-EOF is linewise in Vim even when the buffer has no
			// physical trailing newline.
			if len(e.reg) == 0 || e.reg[len(e.reg)-1] != '\n' {
				e.reg = append(e.reg, '\n')
			}
			e.regLinewise = true
		}
		e.normalCursorClamp()
		e.setLastUndoRedoCursor(e.cursor)
		mc := m
		cc := count
		e.lastEdit = func(ed *Editor) {
			repeat := cc
			if ed.dotCount > 0 {
				repeat = ed.dotCount
			}
			if s, en, ok := ed.rangeFor(mc, repeat); ok {
				ed.deleteRange(s, en)
				ed.normalCursorClamp()
			}
		}
	case 'c':
		if count == 1 && (originalMotion == 'w' || originalMotion == 'W') && start < len(e.text) && e.text[start] == '\n' {
			e.pushUndo()
			e.reg = []rune{'\n'}
			e.regLinewise = false
			e.beginChange(originalMotion, count)
			e.mode = ModeInsert
			return
		}
		e.deleteRange(start, end)
		e.beginChange(originalMotion, count)
		e.mode = ModeInsert
	}
}

func (e *Editor) paste(before bool, count int) {
	if len(e.reg) == 0 {
		return
	}
	if count < 1 {
		count = 1
	}
	e.pushUndo()
	if e.regLinewise {
		ls, le, _, _ := e.lineAt(e.cursor)
		at := ls
		atEnd := false
		chunk := append([]rune(nil), e.reg...)
		if !before {
			if le < len(e.text) {
				at = le + 1
			} else {
				atEnd = true
				at = len(e.text)
				chunk = append([]rune{'\n'}, chunk...)
				if len(chunk) > 0 && chunk[len(chunk)-1] == '\n' {
					chunk = chunk[:len(chunk)-1]
				}
			}
		}
		insert := make([]rune, 0, len(chunk)*count)
		for range count {
			insert = append(insert, chunk...)
		}
		out := make([]rune, 0, len(e.text)+len(insert))
		out = append(out, e.text[:at]...)
		out = append(out, insert...)
		out = append(out, e.text[at:]...)
		e.text = out
		e.cursor = at
		if atEnd {
			e.cursor++
		}
		for name, offset := range e.regMarks {
			markAt := at + offset
			if atEnd {
				markAt++
			}
			e.marks[name] = markAt
		}
		_, lineEnd, _, _ := e.lineAt(e.cursor)
		for e.cursor < lineEnd && isSpaceNotNL(e.text[e.cursor]) {
			e.cursor++
		}
		e.normalCursorClamp()
		return
	}
	at := e.cursor
	if !before && at < len(e.text) {
		at++
	}
	insert := make([]rune, 0, len(e.reg)*count)
	for range count {
		insert = append(insert, e.reg...)
	}
	out := make([]rune, 0, len(e.text)+len(insert))
	out = append(out, e.text[:at]...)
	out = append(out, insert...)
	out = append(out, e.text[at:]...)
	e.text = out
	e.cursor = at + len(insert) - 1
	e.clamp()
}

func (e *Editor) deleteChar(before bool) {
	if before {
		if e.cursor == 0 {
			return
		}
		e.deleteRange(e.cursor-1, e.cursor)
		if e.mode == ModeNormal {
			e.normalCursorClamp()
		}
		return
	}
	if e.cursor >= len(e.text) {
		return
	}
	e.deleteRange(e.cursor, e.cursor+1)
	if e.mode == ModeNormal {
		e.normalCursorClamp()
	}
}

func (e *Editor) replaceChar(r rune) {
	if e.cursor >= len(e.text) {
		return
	}
	e.pushUndo()
	e.text[e.cursor] = r
}

func (e *Editor) replaceChars(r rune, count int) {
	start := e.cursor
	_, lineEnd, _, _ := e.lineAt(start)
	end := start
	available := 0
	for available < count && end < lineEnd {
		end = e.nextGrapheme(end)
		available++
	}
	if count < 1 || available < count || end > lineEnd || end == start {
		return
	}
	last := start
	e.runAtomicWithNoop(func() {
		p := start
		for i := 0; i < count && p < lineEnd; i++ {
			next := e.nextGrapheme(p)
			e.deleteRangeNoRegister(p, next)
			e.cursor = p
			e.insertRunes([]rune{r})
			last = p
			p++
		}
	}, true)
	e.cursor = last
	e.normalCursorClamp()
}

func (e *Editor) toggleCases(count int) {
	if count < 1 || e.cursor >= len(e.text) || e.text[e.cursor] == '\n' {
		return
	}
	start := e.cursor
	_, le, _, _ := e.lineAt(start)
	e.pushUndo()
	p, last := start, start
	for i := 0; i < count && p < le; i++ {
		r := e.text[p]
		if unicode.IsUpper(r) {
			r = unicode.ToLower(r)
		} else {
			r = unicode.ToUpper(r)
		}
		e.text[p] = r
		last = p
		p = e.nextGrapheme(p)
	}
	if p < le {
		e.cursor = p
	} else {
		e.cursor = last
	}
}

func (e *Editor) undoEdit() {
	if len(e.undo) == 0 {
		return
	}
	snap := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	redoCursor := e.cursor
	if snap.hasRedoCursor {
		redoCursor = snap.redoCursor
	}
	e.redo = append(e.redo, undoSnap{text: append([]rune(nil), e.text...), cursor: redoCursor, marks: cloneMarks(e.marks)})
	e.text = snap.text
	e.marks = cloneMarks(snap.marks)
	e.cursor = snap.cursor
	e.clamp()
}

func (e *Editor) redoEdit() {
	if len(e.redo) == 0 {
		return
	}
	snap := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.undo = append(e.undo, undoSnap{text: append([]rune(nil), e.text...), cursor: e.cursor, marks: cloneMarks(e.marks)})
	e.text = snap.text
	e.marks = cloneMarks(snap.marks)
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
	if e.searching {
		switch key {
		case "esc":
			e.searching = false
			e.searchInput = e.searchInput[:0]
			return ActStay
		case "bs":
			if len(e.searchInput) > 0 {
				e.searchInput = e.searchInput[:len(e.searchInput)-1]
			}
			return ActStay
		case "enter":
			e.searching = false
			pattern := e.searchInput
			if len(pattern) == 0 {
				pattern = e.lastSearch
			}
			if p, ok := e.searchPattern(pattern, e.searchDir, e.searchCount); ok {
				if len(pattern) > 0 {
					e.lastSearch = append([]rune(nil), pattern...)
					e.lastSearchDir = e.searchDir
				}
				if e.searchOp != 0 {
					op, dir, cnt := e.searchOp, e.searchDir, e.searchCount
					pat := append([]rune(nil), pattern...)
					e.operateMotionSpan(op, cnt, func(ed *Editor, count int) (int, int, bool) {
						t, ok := ed.searchPattern(pat, dir, count)
						if !ok {
							return 0, 0, false
						}
						if t < ed.cursor {
							return t, ed.cursor, true
						}
						return ed.cursor, t, true
					})
				} else {
					e.cursor = p
				}
			}
			e.searchOp = 0
			e.searchCount = 1
			e.searchInput = e.searchInput[:0]
			return ActStay
		}
		if r, ok := e.printable(key); ok {
			e.searchInput = append(e.searchInput, r)
		}
		return ActStay
	}
	if e.pending == 'm' || e.pending == '`' || e.pending == '\'' {
		kind, op := e.pending, e.pendingOp
		cnt := e.totalOpCount()
		e.pending, e.pendingOp = 0, 0
		if key == "esc" {
			return ActStay
		}
		if r, ok := e.printable(key); ok {
			if kind == 'm' {
				e.marks[r] = e.cursor
			} else if p, exists := e.markTarget(kind, r); exists {
				if op == 0 {
					e.cursor = p
					e.normalCursorClamp()
				} else {
					e.operateMotionSpan(op, cnt, func(ed *Editor, count int) (int, int, bool) {
						t, ok := ed.markTarget(kind, r)
						if !ok {
							return 0, 0, false
						}
						if t < ed.cursor {
							return t, ed.cursor, true
						}
						return ed.cursor, t, true
					})
				}
			}
		}
		return ActStay
	}
	if e.pending == '[' || e.pending == ']' {
		prefix, op := e.pending, e.pendingOp
		cnt := e.totalOpCount()
		e.pending, e.pendingOp = 0, 0
		if sub, ok := e.printable(key); ok {
			if op == 0 {
				if p, found := e.unmatchedBracket(prefix, sub, cnt); found {
					e.cursor = p
				}
			} else {
				e.operateMotionSpan(op, cnt, func(ed *Editor, count int) (int, int, bool) {
					p, found := ed.unmatchedBracket(prefix, sub, count)
					if !found {
						return 0, 0, false
					}
					if p < ed.cursor {
						return p, ed.cursor, true
					}
					return ed.cursor, p + 1, true
				})
			}
		}
		return ActStay
	}
	// A pending r, f, F, t or T is waiting for one literal character (never treat digits as counts here!).
	if e.pending == 'r' {
		cnt := e.takeCount()
		e.clearPendingState()
		if r, ok := e.printable(key); ok {
			e.replaceChars(r, cnt)
			e.dotOverride = 0
			rr, originalCount := r, cnt
			e.lastEdit = func(ed *Editor) {
				repeatCount := originalCount
				if ed.dotCount > 0 {
					repeatCount = ed.dotCount
				}
				ed.replaceChars(rr, repeatCount)
			}
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
	if e.pending == 'd' || e.pending == 'c' || e.pending == 'y' || e.pending == '<' || e.pending == '>' || e.pending == '=' {
		op := e.pending
		if key == "esc" {
			e.clearPendingState()
			return ActStay
		}
		if key == string(op) {
			cnt := e.totalOpCount()
			e.pending = 0
			e.pendingOp = 0
			if op == '<' || op == '>' || op == '=' {
				e.operateIndent(op, op, cnt)
			} else {
				e.operate(op, op, cnt)
			}
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
			explicitCount := e.opCount > 0 || e.count > 0
			if m == 'f' || m == 'F' || m == 't' || m == 'T' || m == 'i' || m == 'a' || m == 'g' || m == '[' || m == ']' || m == '`' || m == '\'' {
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
			if m == '/' || m == '?' {
				cnt := e.totalOpCount()
				e.pending = 0
				e.pendingOp = 0
				e.searching = true
				e.searchInput = e.searchInput[:0]
				e.searchDir = 1
				if m == '?' {
					e.searchDir = -1
				}
				e.searchCount = cnt
				e.searchOp = op
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
			if op == '<' || op == '>' || op == '=' {
				e.operateIndent(op, m, cnt)
			} else if m == '%' && explicitCount {
				e.operatePercent(op, cnt)
			} else {
				e.operate(op, m, cnt)
			}
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
		insertStart := e.changeFrom
		e.applyInsertCount()
		e.endChange()
		e.mode = ModeNormal
		if e.cursor > 0 && e.cursor > insertStart {
			e.cursor--
		}
		e.pending = 0
		e.normalCursorClamp()
		return ActStay
	case "enter":
		return ActAccept
	case "ctrl-c":
		return ActCancel
	case "bs":
		if e.mode == ModeReplace && len(e.replaceSteps) > 0 {
			step := e.replaceSteps[len(e.replaceSteps)-1]
			e.replaceSteps = e.replaceSteps[:len(e.replaceSteps)-1]
			e.cursor = step.pos
			if step.inserted {
				e.deleteRangeNoRegister(step.pos, e.nextGrapheme(step.pos))
			} else if step.pos < len(e.text) {
				e.pushUndo()
				next := e.nextGrapheme(step.pos)
				e.text = append(append(append([]rune(nil), e.text[:step.pos]...), step.original...), e.text[next:]...)
			}
			return ActStay
		}
		if e.cursor > 0 {
			e.deleteRangeNoRegister(e.cursor-1, e.cursor)
		}
		return ActStay
	case "ctrl-u":
		if e.cursor > 0 {
			e.deleteRangeNoRegister(0, e.cursor)
		}
		return ActStay
	case "ctrl-w":
		p, _ := e.move('b', 1)
		if p < e.cursor {
			e.deleteRangeNoRegister(p, e.cursor)
		}
		return ActStay
	case "ctrl-k":
		if e.cursor < len(e.text) {
			e.deleteRangeNoRegister(e.cursor, len(e.text))
		}
		return ActStay
	case "ctrl-y":
		e.insertRunes(e.reg)
		return ActStay
	case "ctrl-d":
		if e.cursor < len(e.text) && e.text[e.cursor] != '\n' {
			e.deleteRangeNoRegister(e.cursor, e.cursor+1)
		}
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
		if e.mode == ModeReplace && e.cursor < len(e.text) && e.text[e.cursor] != '\n' {
			pos := e.cursor
			next := e.nextGrapheme(pos)
			original := append([]rune(nil), e.text[pos:next]...)
			e.pushUndo()
			e.text = append(append(append([]rune(nil), e.text[:pos]...), r), e.text[next:]...)
			e.replaceSteps = append(e.replaceSteps, replaceStep{pos: pos, original: original})
			e.cursor = pos + 1
			e.clamp()
			return ActStay
		}
		if e.mode == ModeReplace {
			pos := e.cursor
			e.insertRunes([]rune{r})
			e.replaceSteps = append(e.replaceSteps, replaceStep{pos: pos, inserted: true})
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
		case 'w', 'W', 'b', 'B', 'e', 'E', '0', '^', '_', '$', ' ', '|', '+', '-', '(', ')':
			p, _ := e.move(r, e.takeCount())
			e.cursor = p
			e.normalCursorClamp()
			return ActStay
		case '%':
			if e.count > 0 {
				cnt := e.takeCount()
				if p, ok := e.percentLine(cnt); ok {
					e.cursor = p
				}
			} else if p, ok := e.move(r, 1); ok {
				e.cursor = p
			}
			return ActStay
		case '{', '}':
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
			n := e.takeCount()
			e.mode = ModeInsert
			e.beginChange('i', 1)
			e.insertCount = n
			return ActStay
		case 'I':
			n := e.takeCount()
			e.mode = ModeInsert
			e.cursor, _ = e.move('^', 1)
			e.beginChange('I', 1)
			e.insertCount = n
			return ActStay
		case 'a':
			n := e.takeCount()
			e.mode = ModeInsert
			if e.cursor < len(e.text) && e.text[e.cursor] != '\n' {
				e.cursor = e.nextGrapheme(e.cursor)
			}
			e.beginChange('a', 1)
			e.insertCount = n
			return ActStay
		case 'A':
			n := e.takeCount()
			e.mode = ModeInsert
			_, end, _, _ := e.lineAt(e.cursor)
			e.cursor = end
			e.beginChange('A', 1)
			e.insertCount = n
			return ActStay
		case 'o':
			n := e.takeCount()
			origin := e.cursor
			_, end, _, _ := e.lineAt(e.cursor)
			indent := e.lineIndent(e.cursor)
			e.cursor = end
			e.insertRunes(append([]rune{'\n'}, indent...))
			e.cursor = end + 1 + len(indent)
			e.beginChange('o', 1)
			e.historyCursor = origin
			e.redoCursor = origin
			e.insertCount = n
			e.mode = ModeInsert
			return ActStay
		case 'O':
			n := e.takeCount()
			start, _, _, _ := e.lineAt(e.cursor)
			indent := e.lineIndent(e.cursor)
			e.cursor = start
			chunk := append(append([]rune(nil), indent...), '\n')
			e.insertRunes(chunk)
			e.cursor = start + len(indent)
			e.beginChange('V', 1)
			e.historyCursor = start
			e.redoCursor = start
			e.insertCount = n
			e.mode = ModeInsert
			return ActStay
		case 'S':
			n := e.takeCount()
			cs, ce, _, _, ok := e.lineSpanDelta(e.cursor, n-1)
			if !ok {
				return ActStay
			}
			lineReg := e.linewiseRegister(cs, ce)
			e.deleteRange(cs, ce)
			e.reg = lineReg
			e.regLinewise = true
			e.cursor = cs
			e.beginChange('S', n)
			e.mode = ModeInsert
			return ActStay
		case 'J':
			cnt := e.takeCount()
			e.joinLines(cnt)
			cc := cnt
			e.lastEdit = func(ed *Editor) {
				repeat := cc
				if ed.dotCount > 0 {
					repeat = ed.dotCount
				}
				ed.joinLines(repeat)
			}
			return ActStay
		case 'R':
			n := e.takeCount()
			e.mode = ModeReplace
			e.beginChange('R', 1)
			e.insertCount = n
			return ActStay
		case 'x':
			n := e.takeCount()
			_, lineEnd, _, _ := e.lineAt(e.cursor)
			end := e.cursor
			for i := 0; i < n && end < lineEnd; i++ {
				end = e.nextGrapheme(end)
			}
			e.deleteRange(e.cursor, end)
			e.normalCursorClamp()
			e.lastEdit = func(ed *Editor) {
				repeat := n
				if ed.dotCount > 0 {
					repeat = ed.dotCount
				}
				_, le, _, _ := ed.lineAt(ed.cursor)
				end := ed.cursor
				for i := 0; i < repeat && end < le; i++ {
					end = ed.nextGrapheme(end)
				}
				ed.deleteRange(ed.cursor, end)
				ed.normalCursorClamp()
			}
			return ActStay
		case 'X':
			n := e.takeCount()
			lineStart, _, _, _ := e.lineAt(e.cursor)
			start := e.cursor - n
			if start < lineStart {
				start = lineStart
			}
			e.deleteRange(start, e.cursor)
			e.normalCursorClamp()
			e.lastEdit = func(ed *Editor) {
				repeat := n
				if ed.dotCount > 0 {
					repeat = ed.dotCount
				}
				ls, _, _, _ := ed.lineAt(ed.cursor)
				start := ed.cursor - repeat
				if start < ls {
					start = ls
				}
				ed.deleteRange(start, ed.cursor)
				ed.normalCursorClamp()
			}
			return ActStay
		case 'r':
			e.pending = 'r'
			return ActStay
		case 's':
			n := e.takeCount()
			_, lineEnd, _, _ := e.lineAt(e.cursor)
			end := e.cursor + n
			if end > lineEnd {
				end = lineEnd
			}
			e.deleteRange(e.cursor, end)
			e.beginChange('s', n)
			e.mode = ModeInsert
			return ActStay
		case 'd', 'c', 'y', '<', '>', '=':
			e.opCount = 0
			if e.count > 0 {
				e.opCount = e.takeCount()
			}
			e.pending = r
			return ActStay
		case 'D':
			e.operate('d', '$', e.takeCount())
			return ActStay
		case 'C':
			e.operate('c', '$', e.takeCount())
			return ActStay
		case 'Y':
			n := e.takeCount()
			start, end, _, _, ok := e.lineSpanDelta(e.cursor, n-1)
			if !ok {
				return ActStay
			}
			e.reg = e.linewiseRegister(start, end)
			e.regLinewise = true
			return ActStay
		case 'p':
			n := e.takeCount()
			e.paste(false, n)
			e.lastEdit = func(ed *Editor) {
				repeat := n
				if ed.dotCount > 0 {
					repeat = ed.dotCount
				}
				ed.paste(false, repeat)
			}
			return ActStay
		case 'P':
			n := e.takeCount()
			e.paste(true, n)
			e.lastEdit = func(ed *Editor) {
				repeat := n
				if ed.dotCount > 0 {
					repeat = ed.dotCount
				}
				ed.paste(true, repeat)
			}
			return ActStay
		case 'u':
			for range e.takeCount() {
				e.undoEdit()
			}
			return ActStay
		case '~':
			n := e.takeCount()
			e.toggleCases(n)
			e.lastEdit = func(ed *Editor) {
				repeat := n
				if ed.dotCount > 0 {
					repeat = ed.dotCount
				}
				ed.toggleCases(repeat)
			}
			return ActStay
		case 'U':
			e.undoLineEdit()
			return ActStay
		case 'm', '`', '\'':
			e.pending = r
			return ActStay
		case '[', ']':
			e.pending = r
			return ActStay
		case '/', '?':
			e.searching = true
			e.searchInput = e.searchInput[:0]
			e.searchDir = 1
			if r == '?' {
				e.searchDir = -1
			}
			e.searchCount = e.takeCount()
			e.searchOp = 0
			return ActStay
		case 'n', 'N':
			dir := e.lastSearchDir
			if r == 'N' {
				dir = -dir
			}
			if p, ok := e.searchPattern(e.lastSearch, dir, e.takeCount()); ok {
				e.cursor = p
			}
			return ActStay
		case 'g':
			return ActFirst
		case 'G':
			return ActLast
		case '.':
			if e.lastEdit != nil {
				if e.count > 0 {
					e.dotOverride = e.takeCount()
				}
				e.dotCount = e.dotOverride
				e.runAtomicWithNoop(func() { e.lastEdit(e) }, true)
				e.dotCount = 0
				e.normalCursorClamp()
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
		for range e.takeCount() {
			e.redoEdit()
		}
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
