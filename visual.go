package vimline

import "unicode"

// VisualKind is the Visual-mode sub-mode: charwise, linewise, or blockwise.
type VisualKind int

const (
	VisualChar VisualKind = iota
	VisualLine
	VisualBlock
)

func (e *Editor) enterVisual(kind VisualKind) {
	e.clearPendingState()
	e.pendingGV = false
	e.visAtEOL = false
	e.visBlockAtEOL = false
	e.mode = ModeVisual
	e.visualKind = kind
	e.visAnchor = e.cursor
	if len(e.text) > 0 && e.cursor >= len(e.text) {
		e.cursor = len(e.text) - 1
		e.visAnchor = e.cursor
	}
	e.visBlockAnchorCol = e.cursorColumn(e.visAnchor)
	e.visBlockCursorCol = e.visBlockAnchorCol
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
	// Cursor on newline is an exclusive-past-last-char corner: the newline
	// is included, the prior character is only included if it is the anchor.
	if b < len(e.text) && e.text[b] == '\n' && b > a {
		// inclusive end on newline is correct for [a,b]
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

// visualBlockCells returns per-line [start, end) slices for the block selection.
func (e *Editor) visualBlockCells() [][2]int {
	if len(e.text) == 0 {
		return nil
	}
	lines := e.lineBounds()
	a := e.lineIndexAt(lines, e.clampPos(e.visAnchor))
	b := e.lineIndexAt(lines, e.clampPos(e.cursor))
	if a > b {
		a, b = b, a
	}
	c1, c2 := e.visBlockAnchorCol, e.visBlockCursorCol
	if c1 > c2 {
		c1, c2 = c2, c1
	}
	out := make([][2]int, 0, b-a+1)
	for i := a; i <= b; i++ {
		ls, le := lines[i][0], lines[i][1]
		width := le - ls
		startCol := c1
		endCol := c2 + 1
		if e.visBlockAtEOL {
			endCol = width
		}
		if startCol > width {
			// Past EOL on this line: empty block cell (still a register line).
			if e.visBlockAtEOL {
				out = append(out, [2]int{le, le})
			}
			continue
		}
		if endCol > width {
			endCol = width
		}
		if startCol >= endCol {
			if e.visBlockAtEOL {
				out = append(out, [2]int{ls + startCol, ls + startCol})
			}
			continue
		}
		out = append(out, [2]int{ls + startCol, ls + endCol})
	}
	return out
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
		if len(cells) == 0 {
			a := e.clampPos(e.cursor)
			return a, a, true
		}
		start, end = cells[0][0], cells[0][1]-1
		for _, c := range cells[1:] {
			if c[0] < start {
				start = c[0]
			}
			if c[1]-1 > end {
				end = c[1] - 1
			}
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
	case 'j', '+':
		e.visAtEOL = false
		lines := e.lineBounds()
		idx := e.lineIndexAt(lines, e.cursor)
		col := e.visBlockCursorCol
		if e.visualKind != VisualBlock {
			col = e.cursorColumn(e.cursor)
		}
		idx += count
		if idx >= len(lines) {
			idx = len(lines) - 1
		}
		e.placeAtColumn(lines[idx][0], col)
		// Keep the virtual block column even when the line is shorter.
		if e.visualKind == VisualBlock {
			e.visBlockCursorCol = col
		}
		return true
	case 'k', '-':
		e.visAtEOL = false
		lines := e.lineBounds()
		idx := e.lineIndexAt(lines, e.cursor)
		col := e.visBlockCursorCol
		if e.visualKind != VisualBlock {
			col = e.cursorColumn(e.cursor)
		}
		idx -= count
		if idx < 0 {
			idx = 0
		}
		e.placeAtColumn(lines[idx][0], col)
		if e.visualKind == VisualBlock {
			e.visBlockCursorCol = col
		}
		return true
	case '$':
		_, le, _, _ := e.lineAt(e.cursor)
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
		if le < len(e.text) {
			e.cursor = le
			e.visAtEOL = true
		} else {
			p, _ := e.move('$', count)
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
			return true
		}
		e.visAtEOL = false
		e.visBlockAtEOL = false
		p, _ := e.move('h', count)
		e.setVisualCursor(p)
		return true
	case 'l', ' ':
		_, le, _, _ := e.lineAt(e.cursor)
		if e.visualKind != VisualBlock && e.cursor >= le-1 && le < len(e.text) && e.text[le] == '\n' {
			e.visAtEOL = true
			e.cursor = le
			return true
		}
		e.visAtEOL = false
		if e.visualKind == VisualBlock {
			e.visBlockAtEOL = false
		}
		p, _ := e.move('l', count)
		e.setVisualCursor(p)
		return true
	}
	e.visAtEOL = false
	if e.visualKind == VisualBlock && m != 'j' && m != 'k' && m != '+' && m != '-' {
		e.visBlockAtEOL = false
	}
	p, ok := e.move(m, count)
	if !ok && m != 'l' && m != 'h' && m != ' ' {
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

func (e *Editor) visualTextObject(around bool, obj rune, count int) bool {
	e.visAtEOL = false
	s, en, ok := e.textObjectSpan(around, obj, count)
	if !ok {
		return false
	}
	if en <= s {
		// Empty inner quote: nvim selects the delimiters (same as around).
		if !around && (obj == '"' || obj == '\'' || obj == '`') {
			s, en, ok = e.textObjectSpan(true, obj, count)
			if !ok || en <= s {
				return false
			}
		} else {
			return false
		}
	}
	// Text objects yield [start, end); Visual ends are inclusive.
	end := en - 1
	if end < s {
		end = s
	}
	e.visAnchor = s
	e.setVisualCursor(end)
	if obj == 'p' || e.visualKind == VisualLine {
		e.visualKind = VisualLine
	} else if e.visualKind == VisualBlock {
		e.visualKind = VisualChar
	}
	return true
}

func (e *Editor) operateVisual(op rune) {
	kind := e.visualKind
	e.rememberVisual()

	switch op {
	case 'd', 'c', 'y':
		e.operateVisualYankDelete(op, kind)
	case '>', '<', '=':
		e.operateVisualIndent(op)
	case '~':
		e.operateVisualCase(0)
	case 'u':
		e.operateVisualCase(-1)
	case 'U':
		e.operateVisualCase(1)
	case 'J':
		e.operateVisualJoin()
	}
}

func (e *Editor) operateVisualYankDelete(op rune, kind VisualKind) {
	switch kind {
	case VisualLine:
		start, end, _, _, ok := e.visualLineSpan()
		if !ok {
			e.exitVisual(false)
			return
		}
		col := e.cursorColumn(e.cursor)
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
		}
		if op == 'c' {
			e.beginChange('c', 1)
			e.mode = ModeInsert
		}
	case VisualBlock:
		cells := e.visualBlockCells()
		e.mode = ModeNormal
		if len(cells) == 0 {
			e.normalCursorClamp()
			return
		}
		// Build register as lines joined by newline (nvim blockwise).
		var reg []rune
		for i, c := range cells {
			if i > 0 {
				reg = append(reg, '\n')
			}
			reg = append(reg, e.text[c[0]:c[1]]...)
		}
		if op == 'y' {
			e.reg = reg
			e.regLinewise = false
			e.cursor = cells[0][0]
			e.normalCursorClamp()
			return
		}
		insertCol := e.cursorColumn(cells[0][0])
		e.pushUndo()
		// Delete bottom-up so offsets stay valid.
		for i := len(cells) - 1; i >= 0; i-- {
			c := cells[i]
			e.text = append(e.text[:c[0]], e.text[c[1]:]...)
		}
		e.reg = reg
		e.regLinewise = false
		e.cursor = cells[0][0]
		if e.cursor > len(e.text) {
			e.cursor = len(e.text)
		}
		if op == 'c' {
			// After delete, remember other lines' insert columns for blockwise
			// change replication when insert ends.
			e.blockChangeCols = nil
			lines := e.lineBounds()
			firstLine := e.lineIndexAt(lines, e.cursor)
			for i := 1; i < len(cells); i++ {
				li := firstLine + i
				if li >= len(lines) {
					break
				}
				e.blockChangeCols = append(e.blockChangeCols, [2]int{li, insertCol})
			}
			e.blockChangeFrom = e.cursor
			e.beginChange('c', 1)
			e.mode = ModeInsert
		} else {
			e.normalCursorClamp()
			e.setLastUndoRedoCursor(e.cursor)
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
		if e.visAtEOL && end < len(e.text) && e.text[end] == '\n' {
			end++
		}
		e.visAtEOL = false
		e.mode = ModeNormal
		if op == 'y' {
			e.reg = append([]rune(nil), e.text[s:end]...)
			e.regLinewise = false
			e.cursor = s
			e.normalCursorClamp()
			return
		}
		e.cursor = s
		e.deleteRange(s, end)
		if op == 'c' {
			e.beginChange('c', 1)
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

func (e *Editor) operateVisualIndent(op rune) {
	_, _, first, last, ok := e.visualLineSpan()
	e.mode = ModeNormal
	if !ok {
		e.normalCursorClamp()
		return
	}
	lines := e.lineBounds()
	e.pushUndo()
	for line := last; line >= first; line-- {
		start := lines[line][0]
		switch op {
		case '>':
			if start < len(e.text) && e.text[start] != '\n' {
				e.text = append(e.text[:start], append([]rune{'\t'}, e.text[start:]...)...)
			}
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
	e.cursor = lines[first][0]
	if op == '=' {
		e.cursor = e.firstNonBlank(lines[first][0])
	}
	if op == '<' && lines[first][1] > lines[first][0] {
		e.cursor = lines[first][1] - 1
	}
	e.normalCursorClamp()
	oo := op
	ff, ll := first, last
	e.lastEdit = func(ed *Editor) {
		ed.pushUndo()
		ls := ed.lineBounds()
		for line := ll; line >= ff && line < len(ls); line-- {
			start := ls[line][0]
			switch oo {
			case '>':
				if start < len(ed.text) && ed.text[start] != '\n' {
					ed.text = append(ed.text[:start], append([]rune{'\t'}, ed.text[start:]...)...)
				}
			case '<':
				if start < len(ed.text) && ed.text[start] == '\t' {
					ed.text = append(ed.text[:start], ed.text[start+1:]...)
				}
			case '=':
				end := start
				for end < len(ed.text) && isSpaceNotNL(ed.text[end]) {
					end++
				}
				ed.text = append(ed.text[:start], ed.text[end:]...)
			}
		}
		ed.cursor = ed.lineBounds()[ff][0]
		ed.normalCursorClamp()
	}
}

// operateVisualCase applies case transform over the visual selection.
// dir: 0 toggle, -1 lower, +1 upper.
func (e *Editor) operateVisualCase(dir int) {
	kind := e.visualKind
	e.mode = ModeNormal
	e.pushUndo()
	switch kind {
	case VisualLine:
		start, end, _, _, ok := e.visualLineSpan()
		if !ok {
			return
		}
		e.applyCaseRange(start, end, dir)
		e.cursor = start
	case VisualBlock:
		cells := e.visualBlockCells()
		for _, c := range cells {
			e.applyCaseRange(c[0], c[1], dir)
		}
		if len(cells) > 0 {
			e.cursor = cells[0][0]
		}
	default:
		s, en, ok := e.visualCharSpan()
		if !ok {
			return
		}
		e.applyCaseRange(s, en+1, dir)
		e.cursor = s
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
		return
	}
	count := last - first + 1
	lines := e.lineBounds()
	e.cursor = lines[first][0]
	e.joinLines(count)
}

func (e *Editor) feedVisual(key string) Action {
	// Pending find character.
	if e.pending == 'f' || e.pending == 'F' || e.pending == 't' || e.pending == 'T' {
		m := e.pending
		e.pending = 0
		if key == "esc" {
			return ActStay
		}
		if r, ok := e.printable(key); ok {
			e.visualFind(m, r, e.takeCount(), false)
		}
		return ActStay
	}

	// Text-object selector.
	if e.pending == 'i' || e.pending == 'a' {
		around := e.pending == 'a'
		e.pending = 0
		if key == "esc" {
			return ActStay
		}
		if obj, ok := e.printable(key); ok {
			e.visualTextObject(around, obj, e.takeCount())
		}
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
			return ActStay
		}
		if sub, ok := e.printable(key); ok {
			switch sub {
			case 'e', 'E':
				p := e.wordEndBack(e.cursor, sub == 'E')
				for i := 1; i < e.takeCount(); i++ {
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
				e.placeAtColumn(lines[0][0], e.cursorColumn(e.cursor))
				if e.visualKind == VisualBlock {
					e.visBlockCursorCol = e.cursorColumn(e.cursor)
				}
			}
		}
		return ActStay
	}

	if r, ok := e.printable(key); ok {
		switch r {
		case 'v':
			if e.visualKind == VisualChar {
				e.exitVisual(true)
			} else {
				e.visualKind = VisualChar
			}
			return ActStay
		case 'V':
			if e.visualKind == VisualLine {
				e.exitVisual(true)
			} else {
				e.visualKind = VisualLine
			}
			return ActStay
		case 'o':
			e.swapVisualEnds()
			return ActStay
		case 'O':
			if e.visualKind == VisualBlock {
				e.swapVisualBlockCorners()
			} else {
				e.swapVisualEnds()
			}
			return ActStay
		case 'h', 'l', 'w', 'W', 'b', 'B', 'e', 'E', '0', '^', '_', '$', ' ', '|', '+', '-', '(', ')', '{', '}':
			e.visualMotion(r, e.takeCount())
			return ActStay
		case 'j':
			e.visualMotion('j', e.takeCount())
			return ActStay
		case 'k':
			e.visualMotion('k', e.takeCount())
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
			e.count = 0
			e.operateVisual(r)
			return ActStay
		case 'x', 's':
			e.count = 0
			if r == 's' {
				e.operateVisual('c')
			} else {
				e.operateVisual('d')
			}
			return ActStay
		case 'g':
			e.pending = 'g'
			return ActStay
		case 'G':
			lines := e.lineBounds()
			e.placeAtColumn(lines[len(lines)-1][0], e.cursorColumn(e.cursor))
			if e.visualKind == VisualBlock {
				e.visBlockCursorCol = e.cursorColumn(e.cursor)
			}
			return ActStay
		}
	}

	switch key {
	case "esc":
		// Clear selection and return to Normal without buffer or history mutation.
		undoBefore := len(e.undo)
		e.exitVisual(true)
		if len(e.undo) != undoBefore {
			e.undo = e.undo[:undoBefore]
		}
		return ActStay
	case "ctrl-v":
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
		e.exitVisual(true)
		return ActCancel
	case "enter":
		return ActAccept
	}
	return ActStay
}
