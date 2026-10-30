package vimline

import "unicode"

func isSpaceNotNL(r rune) bool {
	return r == ' ' || r == '\t'
}

func isBlankLine(rs []rune, start, end int) bool {
	for i := start; i < end; i++ {
		if !isSpaceNotNL(rs[i]) {
			return false
		}
	}
	return true
}

func reverseFindKind(k rune) rune {
	switch k {
	case 'f':
		return 'F'
	case 'F':
		return 'f'
	case 't':
		return 'T'
	case 'T':
		return 't'
	default:
		return k
	}
}

// findCharLine runs f, F, t, or T within the current line [lineStart, lineEnd).
// When isRepeat is true (via ';' or ','), an adjacent match from a previous
// t/T stop is skipped on the first step, matching Vim/Neovim.
func (e *Editor) findCharLine(kind rune, ch rune, count int, isRepeat bool) (int, bool) {
	if ch == 0 || len(e.text) == 0 {
		return e.cursor, false
	}
	if count < 1 {
		count = 1
	}
	ls, le, _, _ := e.lineAt(e.cursor)
	step := 1
	if kind == 'F' || kind == 'T' {
		step = -1
	}
	p := e.cursor
	if p > le {
		p = le
	}
	if p < ls {
		p = ls
	}
	for i := 0; i < count; i++ {
		q := p + step
		if i == 0 && isRepeat {
			if kind == 't' && q < le && e.text[q] == ch {
				q++
			} else if kind == 'T' && q >= ls && e.text[q] == ch {
				q--
			}
		}
		for q >= ls && q < le && e.text[q] != ch {
			q += step
		}
		if q < ls || q >= le {
			return e.cursor, false
		}
		p = q
	}
	if kind == 't' {
		p--
	} else if kind == 'T' {
		p++
	}
	if p < ls || p >= le {
		return e.cursor, false
	}
	return p, true
}

// wordEndBack implements Vim 'ge' (big=false) and 'gE' (big=true): backward to
// the end of the previous word.
func (e *Editor) wordEndBack(p int, big bool) int {
	n := len(e.text)
	if n == 0 || p <= 0 {
		return 0
	}
	if p >= n {
		p = n - 1
	}
	i := p
	// If currently inside a non-space word, step out of this word first.
	if !unicode.IsSpace(e.text[i]) {
		cls := e.wordClass(e.text[i], big)
		for i > 0 && !unicode.IsSpace(e.text[i]) && e.wordClass(e.text[i], big) == cls {
			i--
		}
		if i == 0 && !unicode.IsSpace(e.text[0]) && e.wordClass(e.text[0], big) == cls {
			return 0
		}
	}
	for i > 0 && unicode.IsSpace(e.text[i]) {
		i--
	}
	return i
}

// lastNonBlankOnLine implements Vim 'g_'.
func (e *Editor) lastNonBlankOnLine(p int, count int) int {
	if count > 1 {
		_, _, _, _ = e.lineAt(p)
		for step := 1; step < count; step++ {
			_, ce, _, _ := e.lineAt(p)
			if ce < len(e.text) {
				p = ce + 1
			}
		}
	}
	ls, le, _, _ := e.lineAt(p)
	idx := le - 1
	for idx > ls && isSpaceNotNL(e.text[idx]) {
		idx--
	}
	if idx < ls {
		return ls
	}
	return idx
}

// firstNonBlankDown implements Vim '_' (count-1 lines downward, first non-blank).
func (e *Editor) firstNonBlankDown(p int, count int) int {
	for step := 1; step < count; step++ {
		_, ce, _, _ := e.lineAt(p)
		if ce < len(e.text) {
			p = ce + 1
		}
	}
	ls, le, _, _ := e.lineAt(p)
	i := ls
	for i < le && isSpaceNotNL(e.text[i]) {
		i++
	}
	return i
}

// matchBracket implements Vim '%': if cursor is not on (), [], or {}, scans
// forward on the current line to the first bracket, then finds its balanced mate.
func (e *Editor) matchBracket(p int) (int, bool) {
	n := len(e.text)
	if n == 0 {
		return p, false
	}
	if p >= n {
		p = n - 1
	}
	_, le, _, _ := e.lineAt(p)
	at := -1
	for i := p; i < le; i++ {
		switch e.text[i] {
		case '(', ')', '[', ']', '{', '}':
			at = i
		}
		if at >= 0 {
			break
		}
	}
	if at < 0 {
		return p, false
	}
	ch := e.text[at]
	var open, close rune
	dir := 1
	switch ch {
	case '(':
		open, close, dir = '(', ')', 1
	case ')':
		open, close, dir = '(', ')', -1
	case '[':
		open, close, dir = '[', ']', 1
	case ']':
		open, close, dir = '[', ']', -1
	case '{':
		open, close, dir = '{', '}', 1
	case '}':
		open, close, dir = '{', '}', -1
	}
	depth := 1
	for i := at + dir; i >= 0 && i < n; i += dir {
		if e.text[i] == ch {
			depth++
		} else if (dir > 0 && e.text[i] == close) || (dir < 0 && e.text[i] == open) {
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return p, false
}

// lineBounds returns all line [start, end) content spans in e.text.
func (e *Editor) lineBounds() [][2]int {
	var lines [][2]int
	start := 0
	for i, r := range e.text {
		if r == '\n' {
			lines = append(lines, [2]int{start, i})
			start = i + 1
		}
	}
	lines = append(lines, [2]int{start, len(e.text)})
	return lines
}

func (e *Editor) lineIndexAt(lines [][2]int, p int) int {
	idx := 0
	for i, sp := range lines {
		if sp[0] <= p {
			idx = i
		}
	}
	return idx
}

// paragraphMove implements '{' (dir=-1) and '}' (dir=+1).
func (e *Editor) paragraphMove(p int, dir int, count int) int {
	if len(e.text) == 0 {
		return 0
	}
	lines := e.lineBounds()
	cur := e.lineIndexAt(lines, p)
	for c := 0; c < count; c++ {
		i := cur + dir
		// Skip initial blank lines.
		for i >= 0 && i < len(lines) && isBlankLine(e.text, lines[i][0], lines[i][1]) {
			i += dir
		}
		// Advance through non-blank lines until the next blank line or buffer edge.
		for i >= 0 && i < len(lines) && !isBlankLine(e.text, lines[i][0], lines[i][1]) {
			i += dir
		}
		if i < 0 {
			return 0
		}
		if i >= len(lines) {
			return len(e.text)
		}
		cur = i
	}
	return lines[cur][0]
}

// joinLines implements Vim 'J': joins count+1 lines (min 2 lines) using a single space.
func (e *Editor) joinLines(count int) {
	if len(e.text) == 0 {
		return
	}
	steps := count
	if steps < 2 {
		steps = 1
	} else {
		steps = count - 1
	}
	lines := e.lineBounds()
	curLine := e.lineIndexAt(lines, e.cursor)
	if curLine >= len(lines)-1 {
		return
	}
	e.pushUndo()
	var joinCursor int
	for s := 0; s < steps; s++ {
		lines = e.lineBounds()
		curLine = e.lineIndexAt(lines, e.cursor)
		if curLine >= len(lines)-1 {
			break
		}
		end1 := lines[curLine][1]
		start2, end2 := lines[curLine+1][0], lines[curLine+1][1]
		trim2 := start2
		for trim2 < end2 && isSpaceNotNL(e.text[trim2]) {
			trim2++
		}
		needSpace := end1 > lines[curLine][0] && !isSpaceNotNL(e.text[end1-1]) && trim2 < end2 && e.text[trim2] != ')'
		var next []rune
		next = append(next, e.text[:end1]...)
		joinCursor = len(next)
		if needSpace {
			next = append(next, ' ')
		}
		next = append(next, e.text[trim2:]...)
		e.text = next
		e.cursor = joinCursor
	}
	e.clamp()
}

// textObjectSpan computes [start, end) for Vim text objects ('i' or 'a' + obj).
func (e *Editor) textObjectSpan(around bool, obj rune, count int) (int, int, bool) {
	if count < 1 {
		count = 1
	}
	switch obj {
	case 'w', 'W':
		return e.textObjectWord(around, obj == 'W', count)
	case '"', '\'', '`':
		return e.textObjectQuote(around, obj)
	case '(', ')', 'b':
		return e.textObjectPair(around, '(', ')', count)
	case '[', ']':
		return e.textObjectPair(around, '[', ']', count)
	case '{', '}', 'B':
		return e.textObjectPair(around, '{', '}', count)
	case '<', '>':
		return e.textObjectPair(around, '<', '>', count)
	case 'p':
		return e.textObjectParagraph(around, count)
	}
	return 0, 0, false
}

func (e *Editor) runClass(r rune, big bool) int {
	if r == '\n' {
		return -1
	}
	if isSpaceNotNL(r) {
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

func (e *Editor) textObjectWord(around bool, big bool, count int) (int, int, bool) {
	n := len(e.text)
	if n == 0 {
		return 0, 0, false
	}
	p := e.cursor
	if p >= n {
		p = n - 1
	}
	if e.text[p] == '\n' {
		return 0, 0, false
	}
	cls := e.runClass(e.text[p], big)
	start := p
	for start > 0 && e.runClass(e.text[start-1], big) == cls {
		start--
	}
	end := p + 1
	for end < n && e.runClass(e.text[end], big) == cls {
		end++
	}
	if !around {
		for step := 1; step < count && end < n && e.text[end] != '\n'; step++ {
			c2 := e.runClass(e.text[end], big)
			for end < n && e.runClass(e.text[end], big) == c2 {
				end++
			}
		}
		return start, end, true
	}
	// Around word ('aw' / 'aW'):
	for step := 0; step < count; step++ {
		if step > 0 {
			for end < n && isSpaceNotNL(e.text[end]) {
				end++
			}
			if end < n && e.text[end] != '\n' {
				c2 := e.runClass(e.text[end], big)
				for end < n && e.runClass(e.text[end], big) == c2 {
					end++
				}
			}
		}
	}
	if cls == 0 {
		// Cursor started on whitespace: include following word if on same line, else preceding word.
		if end < n && e.text[end] != '\n' {
			c2 := e.runClass(e.text[end], big)
			for end < n && e.runClass(e.text[end], big) == c2 {
				end++
			}
			return start, end, true
		}
		for start > 0 && e.text[start-1] != '\n' {
			c2 := e.runClass(e.text[start-1], big)
			for start > 0 && e.runClass(e.text[start-1], big) == c2 {
				start--
			}
			break
		}
		return start, end, true
	}
	// Cursor started on word/punct: prefer trailing spaces on same line; if none, include leading spaces!
	trail := end
	for trail < n && isSpaceNotNL(e.text[trail]) {
		trail++
	}
	if trail > end {
		return start, trail, true
	}
	lead := start
	for lead > 0 && isSpaceNotNL(e.text[lead-1]) {
		lead--
	}
	return lead, end, true
}

func (e *Editor) textObjectQuote(around bool, q rune) (int, int, bool) {
	if len(e.text) == 0 {
		return 0, 0, false
	}
	ls, le, _, _ := e.lineAt(e.cursor)
	p := e.cursor
	if p >= le && le > ls {
		p = le - 1
	}
	var positions []int
	for i := ls; i < le; i++ {
		if e.text[i] == '\\' && q != '`' {
			i++
			continue
		}
		if e.text[i] == q {
			positions = append(positions, i)
		}
	}
	if len(positions) < 2 {
		return 0, 0, false
	}
	openIdx, closeIdx := -1, -1
	for k := 0; k+1 < len(positions); k += 2 {
		a, b := positions[k], positions[k+1]
		if p >= a && p <= b {
			openIdx, closeIdx = a, b
			break
		}
		if a > p {
			// Cursor precedes first quote pair on line: seek forward like Vim!
			openIdx, closeIdx = a, b
			break
		}
	}
	if openIdx < 0 {
		// Fallback if cursor sits on a closing quote that paired oddly:
		for k := 0; k+1 < len(positions); k++ {
			a, b := positions[k], positions[k+1]
			if p >= a && p <= b {
				openIdx, closeIdx = a, b
				break
			}
		}
	}
	if openIdx < 0 {
		return 0, 0, false
	}
	if !around {
		return openIdx + 1, closeIdx, true
	}
	start, end := openIdx, closeIdx+1
	trail := end
	for trail < le && isSpaceNotNL(e.text[trail]) {
		trail++
	}
	if trail > end {
		return start, trail, true
	}
	lead := start
	for lead > ls && isSpaceNotNL(e.text[lead-1]) {
		lead--
	}
	return lead, end, true
}

func (e *Editor) textObjectPair(around bool, open, close rune, count int) (int, int, bool) {
	n := len(e.text)
	if n == 0 {
		return 0, 0, false
	}
	p := e.cursor
	if p >= n {
		p = n - 1
	}
	// Find enclosing pair(s) at depth `count`, or seek forward on current line if not enclosed.
	findEnclosingFrom := func(pos int, targetDepth int) (int, int, bool) {
		depth := 0
		openPos := -1
		if e.text[pos] == open && targetDepth == 1 {
			openPos = pos
		} else {
			d := 0
			if e.text[pos] == close {
				// Starting on closing delimiter: step inside it first.
				d = 0
				for i := pos - 1; i >= 0; i-- {
					if e.text[i] == close {
						d++
					} else if e.text[i] == open {
						if d == 0 {
							targetDepth--
							if targetDepth == 0 {
								openPos = i
								break
							}
						} else {
							d--
						}
					}
				}
			} else {
				need := targetDepth
				if e.text[pos] == open {
					need--
					if need == 0 {
						openPos = pos
					}
				}
				for i := pos - 1; openPos < 0 && i >= 0; i-- {
					if e.text[i] == close {
						d++
					} else if e.text[i] == open {
						if d == 0 {
							need--
							if need == 0 {
								openPos = i
								break
							}
						} else {
							d--
						}
					}
				}
			}
		}
		if openPos < 0 {
			return 0, 0, false
		}
		depth = 1
		for j := openPos + 1; j < n; j++ {
			if e.text[j] == open {
				depth++
			} else if e.text[j] == close {
				depth--
				if depth == 0 {
					return openPos, j, true
				}
			}
		}
		return 0, 0, false
	}

	o, c, ok := findEnclosingFrom(p, count)
	if !ok && count == 1 {
		// Seek forward on current line for the next opening delimiter, just like Vim!
		_, le, _, _ := e.lineAt(p)
		for i := p + 1; i < le; i++ {
			if e.text[i] == open {
				o, c, ok = findEnclosingFrom(i, 1)
				break
			}
		}
	}
	if !ok {
		return 0, 0, false
	}
	if around {
		return o, c + 1, true
	}
	return o + 1, c, true
}

func (e *Editor) textObjectParagraph(around bool, count int) (int, int, bool) {
	if len(e.text) == 0 {
		return 0, 0, false
	}
	lines := e.lineBounds()
	cur := e.lineIndexAt(lines, e.cursor)
	curBlank := isBlankLine(e.text, lines[cur][0], lines[cur][1])
	first, last := cur, cur
	for first > 0 && isBlankLine(e.text, lines[first-1][0], lines[first-1][1]) == curBlank {
		first--
	}
	for last+1 < len(lines) && isBlankLine(e.text, lines[last+1][0], lines[last+1][1]) == curBlank {
		last++
	}
	for step := 1; step < count && last+1 < len(lines); step++ {
		b := isBlankLine(e.text, lines[last+1][0], lines[last+1][1])
		for last+1 < len(lines) && isBlankLine(e.text, lines[last+1][0], lines[last+1][1]) == b {
			last++
		}
	}
	if around {
		if !curBlank {
			if last+1 < len(lines) && isBlankLine(e.text, lines[last+1][0], lines[last+1][1]) {
				for last+1 < len(lines) && isBlankLine(e.text, lines[last+1][0], lines[last+1][1]) {
					last++
				}
			} else {
				for first > 0 && isBlankLine(e.text, lines[first-1][0], lines[first-1][1]) {
					first--
				}
			}
		} else if last+1 < len(lines) {
			for last+1 < len(lines) && !isBlankLine(e.text, lines[last+1][0], lines[last+1][1]) {
				last++
			}
		}
	}
	start := lines[first][0]
	end := lines[last][1]
	if end < len(e.text) && e.text[end] == '\n' {
		end++
	} else if start > 0 && first > 0 && last == len(lines)-1 && around {
		start--
	}
	return start, end, true
}
