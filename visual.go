package vimline

import "unicode"

// VisualKind is the Visual-mode sub-mode: charwise, linewise, or blockwise.
type VisualKind int

const (
	VisualChar VisualKind = iota
	VisualLine
	VisualBlock
)

// visualStepRight advances one visual l step: onto the past-end newline
// position once, otherwise one character right. It reports progress.
func (e *Editor) visualStepRight() bool {
	_, le, _, _ := e.lineAt(e.cursor)
	if e.visualKind != VisualBlock && e.cursor >= le-1 && le < len(e.text) && e.text[le] == '\n' {
		if e.cursor >= le {
			return false
		}
		e.visAtEOL = true
		e.cursor = le
		return true
	}
	before := e.cursor
	e.visAtEOL = false
	if e.visualKind == VisualBlock {
		e.visBlockAtEOL = false
	}
	p, _ := e.move('l', 1)
	e.setVisualCursor(p)
	return e.cursor != before
}

func (e *Editor) enterVisual(kind VisualKind) {
	n := e.count
	e.clearPendingState()
	e.pendingGV = false
	e.blockChangeCols = nil
	e.visAtEOL = false
	e.visBlockAtEOL = false
	e.mode = ModeVisual
	e.visAnchor = e.cursor
	if len(e.text) > 0 && e.cursor >= len(e.text) {
		e.cursor = len(e.text) - 1
		e.visAnchor = e.cursor
	}
	e.visBlockAnchorCol = e.cursorColumn(e.visAnchor)
	e.visBlockCursorCol = e.visBlockAnchorCol
	useOp := n > 0 && e.hasLastVisOp
	if useOp {
		kind = e.lastVisOpKind
	}
	e.visualKind = kind
	if n <= 0 {
		return
	}
	switch kind {
	case VisualChar:
		if useOp && e.lastVisOpRows > 1 {
			rows := e.lastVisOpRows * n
			lines := e.lineBounds()
			li := e.lineIndexAt(lines, e.cursor) + rows - 1
			if li >= len(lines) {
				li = len(lines) - 1
			}
			e.visualPlaceAtColumn(lines[li][0], e.lastVisOpSpan)
			break
		}
		span := n
		if useOp {
			span = e.lastVisOpSpan * n
		}
		from := e.cursor
		for i := 1; i < span; i++ {
			if !e.visualStepRight() {
				break
			}
		}
		if e.cursor != from {
			e.commitCurswant()
		}
	case VisualLine:
		rows := n
		if useOp {
			rows = e.lastVisOpRows * n
		}
		lines := e.lineBounds()
		li := e.lineIndexAt(lines, e.cursor) + rows - 1
		if li >= len(lines) {
			li = len(lines) - 1
		}
		e.visualPlaceAtColumn(lines[li][0], e.curswant)
	case VisualBlock:
		span, rows := n, 1
		if useOp {
			span, rows = e.lastVisOpSpan*n, e.lastVisOpRows*n
		}
		lines := e.lineBounds()
		li := e.lineIndexAt(lines, e.cursor) + rows - 1
		if li >= len(lines) {
			li = len(lines) - 1
		}
		e.cursor = lines[li][0]
		e.visBlockCursorCol = e.visBlockAnchorCol + span - 1
		e.visualPlaceAtColumn(e.cursor, e.visBlockCursorCol)
	}
}

func (e *Editor) exitVisual(save bool) {
	if save {
		e.rememberVisual()
	}
	e.mode = ModeNormal
	e.visualKind = VisualChar
	e.clearPendingState()
	e.normalCursorClamp()
}

func (e *Editor) rememberVisual() {
	e.lastVisualKind = e.visualKind
	e.lastVisualAnchor = e.visAnchor
	e.lastVisualCursor = e.cursor
	e.lastVisualBlockAnchorCol = e.visBlockAnchorCol
	e.lastVisualBlockCursorCol = e.visBlockCursorCol
	e.hasLastVisual = true
}

func (e *Editor) reselectVisual() {
	if !e.hasLastVisual {
		return
	}
	e.clearPendingState()
	e.mode = ModeVisual
	e.visualKind = e.lastVisualKind
	e.visAnchor = e.clampPos(e.lastVisualAnchor)
	e.cursor = e.clampPos(e.lastVisualCursor)
	e.visBlockAnchorCol = e.lastVisualBlockAnchorCol
	e.visBlockCursorCol = e.lastVisualBlockCursorCol
	if e.visualKind == VisualBlock {
		e.placeAtColumn(e.cursor, e.visBlockCursorCol)
	}
}

func (e *Editor) clampPos(p int) int {
	if p < 0 {
		return 0
	}
	if len(e.text) == 0 {
		return 0
	}
	if p >= len(e.text) {
		return len(e.text) - 1
	}
	return p
}

// visualClampPos allows the cursor to rest on a newline when charwise $
// has set visAtEOL (nvim MAXCOL / past-last-char corner).
func (e *Editor) visualClampPos(p int) int {
	if e.mode == ModeVisual && e.visualKind == VisualChar && e.visAtEOL {
		if p < 0 {
			return 0
		}
		if p > len(e.text) {
			return len(e.text)
		}
		return p
	}
	return e.clampPos(p)
}

func (e *Editor) swapVisualEnds() {
	e.visAnchor, e.cursor = e.cursor, e.visAnchor
	if e.visualKind == VisualBlock {
		e.visBlockAnchorCol, e.visBlockCursorCol = e.visBlockCursorCol, e.visBlockAnchorCol
		e.placeAtColumn(e.cursor, e.visBlockCursorCol)
	}
}

// swapVisualBlockCorners implements Visual-block O: flip left/right on the
// current line only (other corner), leaving the opposite line's corner as anchor.
func (e *Editor) swapVisualBlockCorners() {
	e.visBlockAnchorCol, e.visBlockCursorCol = e.visBlockCursorCol, e.visBlockAnchorCol
	e.placeAtColumn(e.cursor, e.visBlockCursorCol)
}

// visualCharSpan returns an inclusive [start, end] rune range for the active
// charwise or collapsed selection. ok is false when the buffer is empty.
func (e *Editor) visualCharSpan() (start, end int, ok bool) {
	if len(e.text) == 0 {
		return 0, 0, false
	}
	a, b := e.visualClampPos(e.visAnchor), e.visualClampPos(e.cursor)
	if a > b {
		a, b = b, a
	}
	if a >= len(e.text) {
		a = len(e.text) - 1
	}
	if b >= len(e.text) && len(e.text) > 0 {
		b = len(e.text) - 1
	}
	return a, b, true
}

// visualLineSpan returns a content [start, end) covering whole lines in the
// selection, plus the line indices.
func (e *Editor) visualLineSpan() (start, end, firstLine, lastLine int, ok bool) {
	if len(e.text) == 0 {
		return 0, 0, 0, 0, false
	}
	lines := e.lineBounds()
	a := e.lineIndexAt(lines, e.clampPos(e.visAnchor))
	b := e.lineIndexAt(lines, e.clampPos(e.cursor))
	if a > b {
		a, b = b, a
	}
	// operateLinewiseSpan expects end at column 0 of the line after the last
	// selected line (or EOF), so empty lines keep their trailing newline.
	start = lines[a][0]
	end = len(e.text)
	if b+1 < len(lines) {
		end = lines[b+1][0]
	}
	return start, end, a, b, true
}

// blockCell is one line's block slice. pad marks a line that ends before the
// block column: its text is untouched and the register records a spaces line.
type blockCell struct {
	line       int
	start, end int
	pad        bool
}

// visualBlockRect returns the selected rows and the normalized virtual columns
// of the block. Each corner column clamps to its row's end, so a corner row
// ending before the block column moves the left edge to that row's EOL like
// nvim. $ blocks keep their virtual columns (the deferred MAXCOL family).
func (e *Editor) visualBlockRect() (first, last, c1, c2 int) {
	lines := e.lineBounds()
	ai := e.lineIndexAt(lines, e.clampPos(e.visAnchor))
	ci := e.lineIndexAt(lines, e.clampPos(e.cursor))
	first, last = ai, ci
	if first > last {
		first, last = last, first
	}
	ac, bc := e.visBlockAnchorCol, e.visBlockCursorCol
	if !e.visBlockAtEOL {
		if w := lines[ai][1] - lines[ai][0]; ac > w {
			ac = w
		}
		if w := lines[ci][1] - lines[ci][0]; bc > w {
			bc = w
		}
	}
	c1, c2 = ac, bc
	if c1 > c2 {
		c1, c2 = c2, c1
	}
	return first, last, c1, c2
}

// blockCellsAt returns one cell per line in [first..last] for virtual columns
// [c1..c2], clamped at the buffer edge.
func (e *Editor) blockCellsAt(first, last, c1, c2 int, atEOL bool) []blockCell {
	lines := e.lineBounds()
	if first < 0 {
		first = 0
	}
	if last >= len(lines) {
		last = len(lines) - 1
	}
	if first > last {
		return nil
	}
	out := make([]blockCell, 0, last-first+1)
	for i := first; i <= last; i++ {
		ls, le := lines[i][0], lines[i][1]
		width := le - ls
		if c1 > width {
			// Line ends before the block column: nvim leaves the text alone
			// and only the register records a spaces line.
			if atEOL {
				out = append(out, blockCell{line: i, start: le, end: le})
			} else {
				out = append(out, blockCell{line: i, start: le, end: le, pad: true})
			}
			continue
		}
		if atEOL {
			out = append(out, blockCell{line: i, start: ls + c1, end: le})
			continue
		}
		endCol := c2 + 1
		if endCol > width {
			endCol = width
		}
		if c1 >= endCol {
			// Block column sits at EOL: empty cell, the insert lands at EOL.
			out = append(out, blockCell{line: i, start: ls + c1, end: ls + c1})
			continue
		}
		out = append(out, blockCell{line: i, start: ls + c1, end: ls + endCol})
	}
	return out
}

// visualBlockCells returns one cell per line of the active block selection.
func (e *Editor) visualBlockCells() []blockCell {
	if len(e.text) == 0 {
		return nil
	}
	first, last, c1, c2 := e.visualBlockRect()
	return e.blockCellsAt(first, last, c1, c2, e.visBlockAtEOL)
}

// blockRegister builds the blockwise register: one line per cell, spaces for
// pad lines.
func blockRegister(text []rune, cells []blockCell, width int) []rune {
	var reg []rune
	for i, c := range cells {
		if i > 0 {
			reg = append(reg, '\n')
		}
		if c.pad {
			for w := width; w > 0; w-- {
				reg = append(reg, ' ')
			}
			continue
		}
		reg = append(reg, text[c.start:c.end]...)
	}
	return reg
}

// visualSelBounds returns inclusive start/end for View rendering.
// For an inactive or empty selection SelActive is false.
func (e *Editor) visualSelBounds() (start, end int, active bool) {
	if e.mode != ModeVisual {
		return 0, 0, false
	}
	switch e.visualKind {
	case VisualLine:
		s, _, _, last, ok := e.visualLineSpan()
		if !ok {
			return 0, 0, false
		}
		lines := e.lineBounds()
		le := lines[last][1]
		if le > s {
			return s, le - 1, true
		}
		return s, s, true
	case VisualBlock:
		cells := e.visualBlockCells()
		start, end = 0, 0
		found := false
		for _, c := range cells {
			if c.pad || c.start >= c.end {
				continue
			}
			if !found || c.start < start {
				start = c.start
			}
			if !found || c.end-1 > end {
				end = c.end - 1
			}
			found = true
		}
		if !found {
			a := e.clampPos(e.cursor)
			return a, a, true
		}
		return start, end, true
	default:
		s, en, ok := e.visualCharSpan()
		if !ok {
			return 0, 0, false
		}
		return s, en, true
	}
}

func (e *Editor) setVisualCursor(p int) {
	e.cursor = e.clampPos(p)
	if e.visualKind == VisualBlock {
		e.visBlockCursorCol = e.cursorColumn(e.cursor)
	}
}

func (e *Editor) visualMotion(m rune, count int) bool {
	if count < 1 {
		count = 1
	}
	switch m {
	case 'j':
		e.visAtEOL = false
		lines := e.lineBounds()
		idx := e.lineIndexAt(lines, e.cursor)
		col := e.visBlockCursorCol
		if e.visualKind != VisualBlock {
			col = e.curswant
		}
		idx += count
		if idx >= len(lines) {
			idx = len(lines) - 1
		}
		e.visualPlaceAtColumn(lines[idx][0], col)
		// Keep the virtual block column even when the line is shorter.
		if e.visualKind == VisualBlock {
			e.visBlockCursorCol = col
		}
		return true
	case 'k':
		e.visAtEOL = false
		lines := e.lineBounds()
		idx := e.lineIndexAt(lines, e.cursor)
		col := e.visBlockCursorCol
		if e.visualKind != VisualBlock {
			col = e.curswant
		}
		idx -= count
		if idx < 0 {
			idx = 0
		}
		e.visualPlaceAtColumn(lines[idx][0], col)
		if e.visualKind == VisualBlock {
			e.visBlockCursorCol = col
		}
		return true
	case '+', '-':
		e.visAtEOL = false
		lines := e.lineBounds()
		idx := e.lineIndexAt(lines, e.cursor)
		target := idx + count
		if m == '-' {
			target = idx - count
		}
		if target >= len(lines) {
			target = len(lines) - 1
		}
		if target < 0 {
			target = 0
		}
		if target == idx {
			return false
		}
		p := lines[target][0]
		for p < lines[target][1] && isSpaceNotNL(e.text[p]) {
			p++
		}
		e.setVisualCursor(p)
		return true
	case '$':
		if e.visualKind == VisualBlock {
			e.visBlockAtEOL = true
			e.visAtEOL = false
			// Keep the virtual left/right column; do not collapse to the
			// clamped last character of a short line (nvim MAXCOL).
			p, _ := e.move('$', count)
			e.cursor = e.clampPos(p)
			return true
		}
		// Charwise/linewise: sit on the newline (past last char) when one exists.
		lines := e.lineBounds()
		li := e.lineIndexAt(lines, e.cursor) + count - 1
		if li >= len(lines) {
			li = len(lines) - 1
		}
		le := lines[li][1]
		if le < len(e.text) {
			e.cursor = le
			e.visAtEOL = true
		} else {
			p, _ := e.move('$', 1)
			e.cursor = e.clampPos(p)
			e.visAtEOL = false
		}
		return true
	case 'h':
		if e.visualKind == VisualBlock && e.visBlockAtEOL {
			// First h from MAXCOL lands on the last character; clear EOL flag.
			e.visBlockAtEOL = false
			ls, le, _, _ := e.lineAt(e.cursor)
			if le > ls {
				e.cursor = le - 1
			} else {
				e.cursor = ls
			}
			e.visBlockCursorCol = e.cursorColumn(e.cursor)
			for i := 1; i < count; i++ {
				p, _ := e.move('h', 1)
				e.setVisualCursor(p)
			}
			return true
		}
		e.visAtEOL = false
		e.visBlockAtEOL = false
		p, _ := e.move('h', count)
		e.setVisualCursor(p)
		return true
	case 'l', ' ':
		for i := 0; i < count; i++ {
			if !e.visualStepRight() {
				break
			}
		}
		return true
	}
	e.visAtEOL = false
	if e.visualKind == VisualBlock && m != 'j' && m != 'k' {
		e.visBlockAtEOL = false
	}
	p, ok := e.move(m, count)
	if !ok {
		return false
	}
	e.setVisualCursor(p)
	return true
}

func (e *Editor) visualFind(kind, ch rune, count int, isRepeat bool) bool {
	p, ok := e.findCharLine(kind, ch, count, isRepeat)
	if !ok {
		return false
	}
	e.findCh = ch
	e.findKind = kind
	if kind == 'f' || kind == 't' {
		e.findDir = 1
	} else {
		e.findDir = -1
	}
	e.setVisualCursor(p)
	return true
}

// visualObjectSpan resolves the object at the cursor, falling back to the
// delimiters for an empty inner quote like nvim.
func (e *Editor) visualObjectSpan(around bool, obj rune, count int) (int, int, bool) {
	s, en, ok := e.textObjectSpan(around, obj, count)
	if !ok {
		return 0, 0, false
	}
	if en <= s {
		if !around && (obj == '"' || obj == '\'' || obj == '`') {
			s, en, ok = e.textObjectSpan(true, obj, count)
			if !ok || en <= s {
				return 0, 0, false
			}
		} else {
			return 0, 0, false
		}
	}
	return s, en, true
}

// visualObjectKind picks the selection kind after a text object: a single-row
// object keeps a blockwise selection, a paragraph is linewise, the rest are
// charwise (nvim switches V to v).
func (e *Editor) visualObjectKind(s, end int, obj rune) {
	if e.visualKind == VisualBlock && e.lineIndexAt(e.lineBounds(), s) == e.lineIndexAt(e.lineBounds(), end) {
		return
	}
	if obj == 'p' {
		e.visualKind = VisualLine
	} else {
		e.visualKind = VisualChar
	}
}

func (e *Editor) visualTextObject(around bool, obj rune, count int) bool {
	if e.cursor != e.visAnchor {
		// With a live selection nvim extends the cursor end by one object.
		backward := e.cursor < e.visAnchor
		step := e.cursor + 1
		if backward {
			step = e.cursor - 1
		}
		if step < 0 || step >= len(e.text) {
			return false
		}
		saved := e.cursor
		e.cursor = step
		s, en, ok := e.visualObjectSpan(around, obj, count)
		e.cursor = saved
		if !ok {
			return false
		}
		end := en - 1
		if end < s {
			end = s
		}
		e.visAtEOL = false
		if backward {
			e.setVisualCursor(s)
		} else {
			e.setVisualCursor(end)
		}
		e.visualObjectKind(s, end, obj)
		return true
	}
	s, en, ok := e.visualObjectSpan(around, obj, count)
	if !ok {
		return false
	}
	// Text objects yield [start, end); Visual ends are inclusive.
	end := en - 1
	if end < s {
		end = s
	}
	e.visAtEOL = false
	e.visAnchor = s
	e.setVisualCursor(end)
	if e.visualKind == VisualBlock && e.lineIndexAt(e.lineBounds(), s) == e.lineIndexAt(e.lineBounds(), end) {
		e.visBlockAnchorCol = e.cursorColumn(s)
	} else {
		e.visualObjectKind(s, end, obj)
	}
	return true
}

func (e *Editor) operateVisual(op rune, count int) {
	kind := e.visualKind
	e.rememberVisual()
	switch kind {
	case VisualChar:
		if s, en, ok := e.visualCharSpan(); ok {
			e.hasLastVisOp = true
			e.lastVisOpKind = kind
			e.lastVisOpRows = e.lineIndexAt(e.lineBounds(), en) - e.lineIndexAt(e.lineBounds(), s) + 1
			e.lastVisOpSpan = en - s + 1
			if e.lastVisOpRows > 1 {
				e.lastVisOpSpan = e.cursorColumn(en)
			}
		}
	case VisualLine:
		if _, _, first, last, ok := e.visualLineSpan(); ok {
			e.hasLastVisOp = true
			e.lastVisOpKind = kind
			e.lastVisOpSpan = 1
			e.lastVisOpRows = last - first + 1
		}
	case VisualBlock:
		if cells := e.visualBlockCells(); len(cells) > 0 {
			_, _, c1, c2 := e.visualBlockRect()
			e.hasLastVisOp = true
			e.lastVisOpKind = kind
			e.lastVisOpSpan = c2 - c1 + 1
			e.lastVisOpRows = len(cells)
		}
	}

	switch op {
	case 'd', 'c', 'y':
		e.operateVisualYankDelete(op, kind)
	case '>', '<':
		e.operateVisualIndent(op, count)
	case '=':
		e.operateVisualIndent(op, 1)
	case '~':
		e.operateVisualCase(0)
	case 'u':
		e.operateVisualCase(-1)
	case 'U':
		e.operateVisualCase(1)
	case 'J':
		e.operateVisualJoin()
	}
	e.commitCurswant()
}

func (e *Editor) operateVisualYankDelete(op rune, kind VisualKind) {
	switch kind {
	case VisualLine:
		start, end, first, last, ok := e.visualLineSpan()
		if !ok {
			e.exitVisual(false)
			return
		}
		col := e.curswant
		e.mode = ModeNormal
		if op == 'y' {
			e.operateLinewiseSpan('y', start, end, spanLinewise)
			e.cursor = start
			e.normalCursorClamp()
			return
		}
		e.operateLinewiseSpan(op, start, end, spanLinewise)
		if op == 'd' {
			// nvim keeps curswant onto the line that lands under the cursor.
			e.placeAtColumn(e.cursor, col)
			e.normalCursorClamp()
			n := last - first + 1
			e.lastEdit = func(ed *Editor) {
				repeat := n
				if ed.dotCount > 0 {
					repeat = ed.dotCount
				}
				lines := ed.lineBounds()
				li := ed.lineIndexAt(lines, ed.cursor)
				to := li + repeat - 1
				if to >= len(lines) {
					to = len(lines) - 1
				}
				end := len(ed.text)
				if to+1 < len(lines) {
					end = lines[to+1][0]
				}
				column := ed.curswant
				ed.operateLinewiseSpan('d', lines[li][0], end, spanLinewise)
				ed.placeAtColumn(ed.cursor, column)
				ed.normalCursorClamp()
			}
		}
		if op == 'c' {
			e.beginChange('L', last-first+1)
			e.mode = ModeInsert
		}
	case VisualBlock:
		cells := e.visualBlockCells()
		e.mode = ModeNormal
		_, _, c1, c2 := e.visualBlockRect()
		width := c2 - c1 + 1
		if len(cells) == 0 {
			e.normalCursorClamp()
			return
		}
		// Build register as lines joined by newline (nvim blockwise).
		reg := blockRegister(e.text, cells, width)
		if op == 'y' {
			e.reg = reg
			e.regKind = regBlockwise
			e.cursor = cells[0].start
			e.normalCursorClamp()
			return
		}
		e.pushUndo()
		// Delete bottom-up so offsets stay valid.
		for i := len(cells) - 1; i >= 0; i-- {
			c := cells[i]
			e.relocateMarksForDelete(c.start, c.end)
			e.text = append(e.text[:c.start], e.text[c.end:]...)
		}
		e.reg = reg
		e.regKind = regBlockwise
		e.cursor = cells[0].start
		if e.cursor > len(e.text) {
			e.cursor = len(e.text)
		}
		if op == 'c' {
			// After delete, remember other lines' insert columns for blockwise
			// change replication when insert ends.
			e.beginChange('c', len(cells))
			e.changeBlockWidth = width
			for i := 1; i < len(cells); i++ {
				c := cells[i]
				if c.pad {
					continue
				}
				e.blockChangeCols = append(e.blockChangeCols, [2]int{c.line, c1})
			}
			e.mode = ModeInsert
		} else {
			e.normalCursorClamp()
			e.setLastUndoRedoCursor(e.cursor)
			rows := len(cells)
			e.lastEdit = func(ed *Editor) {
				repeat := rows
				if ed.dotCount > 0 {
					repeat = ed.dotCount
				}
				lines := ed.lineBounds()
				li := ed.lineIndexAt(lines, ed.cursor)
				col := ed.cursorColumn(ed.cursor)
				blk := ed.blockCellsAt(li, li+repeat-1, col, col+width-1, false)
				ed.reg = blockRegister(ed.text, blk, width)
				ed.regKind = regBlockwise
				for i := len(blk) - 1; i >= 0; i-- {
					c := blk[i]
					ed.relocateMarksForDelete(c.start, c.end)
					ed.text = append(ed.text[:c.start], ed.text[c.end:]...)
				}
				if len(blk) > 0 {
					ed.cursor = blk[0].start
				}
				ed.normalCursorClamp()
			}
		}
	default:
		s, en, ok := e.visualCharSpan()
		if !ok {
			e.exitVisual(false)
			return
		}
		end := en + 1
		if end > len(e.text) {
			end = len(e.text)
		}
		e.visAtEOL = false
		e.mode = ModeNormal
		if op == 'y' {
			e.reg = append([]rune(nil), e.text[s:end]...)
			e.regKind = regCharwise
			e.cursor = s
			e.normalCursorClamp()
			return
		}
		e.cursor = s
		e.deleteRange(s, end)
		if op == 'c' {
			e.beginChange('c', end-s)
			e.mode = ModeInsert
		} else {
			e.normalCursorClamp()
			e.setLastUndoRedoCursor(e.cursor)
		}
		if op == 'd' {
			cc := end - s
			e.lastEdit = func(ed *Editor) {
				repeat := cc
				if ed.dotCount > 0 {
					repeat = ed.dotCount
				}
				en := ed.cursor
				for i := 0; i < repeat && en < len(ed.text); i++ {
					en = ed.nextGrapheme(en)
				}
				ed.deleteRange(ed.cursor, en)
				ed.normalCursorClamp()
			}
		}
	}
}

func (e *Editor) operateVisualIndent(op rune, levels int) {
	_, _, first, last, ok := e.visualLineSpan()
	e.mode = ModeNormal
	if !ok {
		e.normalCursorClamp()
		return
	}
	lines := e.lineBounds()
	e.placeAtColumn(lines[first][0], e.cursorColumn(e.cursor))
	e.operateIndent(op, op, last-first+1, levels)
	e.normalCursorClamp()
}

// operateVisualCase applies case transform over the visual selection.
// dir: 0 toggle, -1 lower, +1 upper.
func (e *Editor) operateVisualCase(dir int) {
	kind := e.visualKind
	e.mode = ModeNormal
	e.pushUndo()
	switch kind {
	case VisualLine:
		start, end, first, last, ok := e.visualLineSpan()
		if !ok {
			return
		}
		e.applyCaseRange(start, end, dir)
		e.cursor = start
		n := last - first + 1
		e.lastEdit = func(ed *Editor) {
			repeat := n
			if ed.dotCount > 0 {
				repeat = ed.dotCount
			}
			lines := ed.lineBounds()
			li := ed.lineIndexAt(lines, ed.cursor)
			to := li + repeat - 1
			if to >= len(lines) {
				to = len(lines) - 1
			}
			end := len(ed.text)
			if to+1 < len(lines) {
				end = lines[to+1][0]
			}
			ed.applyCaseRange(lines[li][0], end, dir)
		}
	case VisualBlock:
		cells := e.visualBlockCells()
		for _, c := range cells {
			e.applyCaseRange(c.start, c.end, dir)
		}
		if len(cells) > 0 {
			e.cursor = cells[0].start
		}
		_, _, c1, c2 := e.visualBlockRect()
		n := len(cells)
		width := c2 - c1 + 1
		e.lastEdit = func(ed *Editor) {
			repeat := n
			if ed.dotCount > 0 {
				repeat = ed.dotCount
			}
			lines := ed.lineBounds()
			li := ed.lineIndexAt(lines, ed.cursor)
			col := ed.cursorColumn(ed.cursor)
			for _, c := range ed.blockCellsAt(li, li+repeat-1, col, col+width-1, false) {
				ed.applyCaseRange(c.start, c.end, dir)
			}
		}
	default:
		s, en, ok := e.visualCharSpan()
		if !ok {
			return
		}
		e.applyCaseRange(s, en+1, dir)
		e.cursor = s
		n := en + 1 - s
		e.lastEdit = func(ed *Editor) {
			repeat := n
			if ed.dotCount > 0 {
				repeat = ed.dotCount
			}
			end := ed.cursor + repeat
			if end > len(ed.text) {
				end = len(ed.text)
			}
			ed.applyCaseRange(ed.cursor, end, dir)
		}
	}
	e.normalCursorClamp()
}

func (e *Editor) applyCaseRange(start, end int, dir int) {
	if start < 0 {
		start = 0
	}
	if end > len(e.text) {
		end = len(e.text)
	}
	for p := start; p < end; p++ {
		r := e.text[p]
		if r == '\n' {
			continue
		}
		switch dir {
		case 1:
			e.text[p] = unicode.ToUpper(r)
		case -1:
			e.text[p] = unicode.ToLower(r)
		default:
			if unicode.IsUpper(r) {
				e.text[p] = unicode.ToLower(r)
			} else {
				e.text[p] = unicode.ToUpper(r)
			}
		}
	}
}

func (e *Editor) operateVisualJoin() {
	_, _, first, last, ok := e.visualLineSpan()
	e.mode = ModeNormal
	if !ok || first == last {
		// Single-line visual J joins with the next line (nvim).
		e.joinLines(1)
		e.lastEdit = func(ed *Editor) {
			repeat := 1
			if ed.dotCount > 0 {
				repeat = ed.dotCount
			}
			ed.joinLines(repeat)
		}
		return
	}
	count := last - first + 1
	lines := e.lineBounds()
	e.cursor = lines[first][0]
	e.joinLines(count)
	n := count
	e.lastEdit = func(ed *Editor) {
		repeat := n
		if ed.dotCount > 0 {
			repeat = ed.dotCount
		}
		ed.joinLines(repeat)
	}
}

func (e *Editor) feedVisual(key string) Action {
	if key == "ctrl-c" && e.pending != 0 {
		e.pending = 0
		e.count = 0
		return ActCancel
	}
	switch e.pending {
	case 'r', 'm', '`', '\'', '[', ']':
		e.pending = 0
		e.count = 0
		return ActStay
	case '/', '?':
		if key == "enter" || key == "esc" {
			e.pending = 0
		}
		e.count = 0
		return ActStay
	}

	// Pending find character.
	if e.pending == 'f' || e.pending == 'F' || e.pending == 't' || e.pending == 'T' {
		m := e.pending
		e.pending = 0
		if key == "esc" {
			e.count = 0
			return ActStay
		}
		if r, ok := e.printable(key); ok {
			e.visualFind(m, r, e.takeCount(), false)
		}
		e.count = 0
		return ActStay
	}

	// Text-object selector.
	if e.pending == 'i' || e.pending == 'a' {
		around := e.pending == 'a'
		e.pending = 0
		if key == "esc" {
			e.count = 0
			return ActStay
		}
		if obj, ok := e.printable(key); ok {
			if e.visualTextObject(around, obj, e.takeCount()) {
				e.commitCurswant()
			}
		}
		e.count = 0
		return ActStay
	}

	// Numeric prefix.
	if r, ok := e.printable(key); ok {
		if (r >= '1' && r <= '9') || (r == '0' && e.count > 0) {
			e.count = e.count*10 + int(r-'0')
			return ActStay
		}
	}

	// g-prefix motions (ge, gE, g_, gg) before bare motion keys.
	if e.pending == 'g' {
		e.pending = 0
		if key == "esc" {
			e.count = 0
			return ActStay
		}
		if sub, ok := e.printable(key); ok {
			switch sub {
			case 'e', 'E':
				cnt := e.takeCount()
				p := e.wordEndBack(e.cursor, sub == 'E')
				for i := 1; i < cnt; i++ {
					if p == 0 {
						break
					}
					p = e.wordEndBack(p, sub == 'E')
				}
				e.setVisualCursor(p)
			case '_':
				e.setVisualCursor(e.lastNonBlankOnLine(e.cursor, e.takeCount()))
			case 'g':
				lines := e.lineBounds()
				li := e.takeCount() - 1
				if li >= len(lines) {
					li = len(lines) - 1
				}
				col := e.visBlockCursorCol
				if e.visualKind != VisualBlock {
					col = e.curswant
				}
				e.visualPlaceAtColumn(lines[li][0], col)
				if e.visualKind == VisualBlock {
					e.visBlockCursorCol = col
				}
			}
		}
		e.count = 0
		return ActStay
	}

	if r, ok := e.printable(key); ok {
		switch r {
		case 'v':
			e.count = 0
			if e.visualKind == VisualChar {
				e.exitVisual(true)
			} else {
				e.visualKind = VisualChar
			}
			return ActStay
		case 'V':
			e.count = 0
			if e.visualKind == VisualLine {
				e.exitVisual(true)
			} else {
				e.visualKind = VisualLine
			}
			return ActStay
		case 'o':
			e.count = 0
			e.swapVisualEnds()
			e.commitCurswant()
			return ActStay
		case 'O':
			e.count = 0
			if e.visualKind == VisualBlock {
				e.swapVisualBlockCorners()
			} else {
				e.swapVisualEnds()
			}
			e.commitCurswant()
			return ActStay
		case 'h', 'l', 'j', 'k', 'w', 'W', 'b', 'B', 'e', 'E', '0', '^', '_', '$', ' ', '|', '+', '-', '(', ')', '{', '}':
			e.visualMotion(r, e.takeCount())
			return ActStay
		case '%':
			if e.count > 0 {
				cnt := e.takeCount()
				if p, ok := e.percentLine(cnt); ok {
					e.setVisualCursor(p)
				}
			} else if p, ok := e.move('%', 1); ok {
				e.setVisualCursor(p)
			}
			return ActStay
		case 'f', 'F', 't', 'T':
			e.pending = r
			return ActStay
		case ';', ',':
			if e.findCh == 0 {
				e.count = 0
				return ActStay
			}
			kind := e.findKind
			if r == ',' {
				kind = reverseFindKind(kind)
			}
			e.visualFind(kind, e.findCh, e.takeCount(), true)
			return ActStay
		case 'i', 'a':
			e.pending = r
			return ActStay
		case 'd', 'c', 'y', '>', '<', '=', '~', 'u', 'U', 'J':
			e.operateVisual(r, e.takeCount())
			return ActStay
		case 'x', 's':
			n := e.takeCount()
			if r == 's' {
				e.operateVisual('c', n)
			} else {
				e.operateVisual('d', n)
			}
			return ActStay
		case 'p', 'P', 'n', 'N':
			e.count = 0
			return ActStay
		case 'r', 'm', '`', '\'', '[', ']':
			e.pending = r
			return ActStay
		case '/', '?':
			e.pending = r
			return ActStay
		case 'g':
			e.pending = 'g'
			return ActStay
		case 'G':
			lines := e.lineBounds()
			li := len(lines) - 1
			if e.count > 0 {
				li = e.takeCount() - 1
				if li >= len(lines) {
					li = len(lines) - 1
				}
			}
			col := e.visBlockCursorCol
			if e.visualKind != VisualBlock {
				col = e.curswant
			}
			e.visualPlaceAtColumn(lines[li][0], col)
			if e.visualKind == VisualBlock {
				e.visBlockCursorCol = col
			}
			return ActStay
		}
	}

	switch key {
	case "esc":
		// Clear selection and return to Normal without buffer or history mutation.
		e.count = 0
		e.exitVisual(true)
		e.curswant = e.cursorColumn(e.cursor)
		return ActStay
	case "ctrl-v":
		e.count = 0
		if e.visualKind == VisualBlock {
			e.exitVisual(true)
		} else {
			e.visualKind = VisualBlock
			e.visBlockAnchorCol = e.cursorColumn(e.visAnchor)
			e.visBlockCursorCol = e.cursorColumn(e.cursor)
		}
		return ActStay
	case "bs":
		e.visualMotion('h', e.takeCount())
		return ActStay
	case "left":
		e.visualMotion('h', e.takeCount())
		return ActStay
	case "right":
		e.visualMotion('l', e.takeCount())
		return ActStay
	case "down":
		e.visualMotion('j', e.takeCount())
		return ActStay
	case "up":
		e.visualMotion('k', e.takeCount())
		return ActStay
	case "ctrl-c":
		e.count = 0
		e.exitVisual(true)
		e.commitCurswant()
		return ActCancel
	case "enter":
		e.count = 0
		e.exitVisual(true)
		e.commitCurswant()
		return ActAccept
	}
	e.count = 0
	return ActStay
}
