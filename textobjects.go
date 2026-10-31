package vimline

import (
	"sort"
	"strings"
	"unicode"
)

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
	isMotionBlank := func(i int) bool { return lines[i][0] == lines[i][1] }
	target := p
	for range count {
		cur := e.lineIndexAt(lines, target)
		blank := isMotionBlank(cur)
		if dir > 0 {
			i := cur
			if blank {
				for i < len(lines) && isMotionBlank(i) {
					i++
				}
			}
			for i < len(lines) && !isMotionBlank(i) {
				i++
			}
			if i < len(lines) {
				target = lines[i][0]
			} else if len(e.text) > 0 && target != len(e.text)-1 {
				target = len(e.text) - 1
			} else {
				return p
			}
		} else {
			i := cur
			if blank {
				for i >= 0 && isMotionBlank(i) {
					i--
				}
				for i >= 0 && !isMotionBlank(i) {
					i--
				}
				if i+1 < cur {
					target = lines[i+1][0]
				} else {
					return p
				}
			} else {
				i--
				for i >= 0 && !isMotionBlank(i) {
					i--
				}
				if i >= 0 {
					target = lines[i][0]
				} else if target != 0 {
					target = 0
				} else {
					return p
				}
			}
		}
	}
	return target
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
	originalCursor := e.cursor
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
	e.setLastUndoRedoCursor(originalCursor)
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
		return e.textObjectQuote(around, obj, count)
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
	case 's':
		return e.textObjectSentence(around, count)
	case 't':
		return e.textObjectTag(around, count)
	}
	return 0, 0, false
}

func sentenceEndRune(r rune) bool { return r == '.' || r == '!' || r == '?' }

type sentencePart struct {
	start, end int
	space      bool
}

func (e *Editor) sentenceParts() []sentencePart {
	var parts []sentencePart
	for i := 0; i < len(e.text); {
		if unicode.IsSpace(e.text[i]) {
			start := i
			for i < len(e.text) && unicode.IsSpace(e.text[i]) {
				i++
			}
			parts = append(parts, sentencePart{start: start, end: i, space: true})
			continue
		}
		start := i
		for i < len(e.text) {
			i++
			if !sentenceEndRune(e.text[i-1]) {
				continue
			}
			j := i
			for j < len(e.text) && strings.ContainsRune(")]}'\"", e.text[j]) {
				j++
			}
			if j == len(e.text) || unicode.IsSpace(e.text[j]) {
				i = j
				break
			}
		}
		parts = append(parts, sentencePart{start: start, end: i})
	}
	return parts
}

func (e *Editor) sentenceMove(p int, forward bool, count int) (int, bool) {
	parts := e.sentenceParts()
	if count < 1 {
		count = 1
	}
	target := p
	for range count {
		found := false
		if forward {
			for _, part := range parts {
				if !part.space && part.start > target {
					target, found = part.start, true
					break
				}
			}
		} else {
			for i := len(parts) - 1; i >= 0; i-- {
				if !parts[i].space && parts[i].start < target {
					target, found = parts[i].start, true
					break
				}
			}
		}
		if !found {
			if forward && len(e.text) > 0 && target != len(e.text)-1 {
				target = len(e.text) - 1
				continue
			}
			return p, false
		}
	}
	return target, true
}

func (e *Editor) textObjectSentence(around bool, count int) (int, int, bool) {
	parts := e.sentenceParts()
	if len(parts) == 0 {
		return 0, 0, false
	}
	p := e.cursor
	if p >= len(e.text) {
		p = len(e.text) - 1
	}
	idx := -1
	for i, part := range parts {
		if p >= part.start && p < part.end {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, 0, false
	}
	start, end := parts[idx].start, parts[idx].end
	if around && !parts[idx].space {
		if idx+1 < len(parts) && parts[idx+1].space {
			end = parts[idx+1].end
		} else if idx > 0 && parts[idx-1].space {
			start = parts[idx-1].start
		}
	}
	for step := 1; step < count; step++ {
		next := idx + step
		if next >= len(parts) {
			break
		}
		end = parts[next].end
	}
	return start, end, end > start
}

func (e *Editor) textObjectTag(around bool, count int) (int, int, bool) {
	type tagOpen struct {
		name       string
		start, end int
	}
	type tagPair struct {
		openStart, openEnd, closeStart, closeEnd int
	}
	var stack []tagOpen
	var pairs []tagPair
	var selfClosingSpans [][2]int
	for i := 0; i < len(e.text); {
		if e.text[i] != '<' {
			i++
			continue
		}
		j := i + 1
		for j < len(e.text) && e.text[j] != '>' {
			j++
		}
		if j >= len(e.text) {
			break
		}
		inside := strings.TrimSpace(string(e.text[i+1 : j]))
		closing := strings.HasPrefix(inside, "/")
		selfClosing := strings.HasSuffix(inside, "/")
		nameText := strings.TrimSpace(strings.TrimPrefix(inside, "/"))
		if cut := strings.IndexAny(nameText, " \t/\n"); cut >= 0 {
			nameText = nameText[:cut]
		}
		if nameText != "" && !strings.HasPrefix(nameText, "!") && !strings.HasPrefix(nameText, "?") {
			if closing {
				for k := len(stack) - 1; k >= 0; k-- {
					if stack[k].name == nameText {
						o := stack[k]
						stack = stack[:k]
						pairs = append(pairs, tagPair{o.start, o.end, i, j + 1})
						break
					}
				}
			} else if !selfClosing {
				stack = append(stack, tagOpen{nameText, i, j + 1})
			} else {
				selfClosingSpans = append(selfClosingSpans, [2]int{i, j + 1})
			}
		}
		i = j + 1
	}
	p := min(e.cursor, len(e.text))
	for _, span := range selfClosingSpans {
		if p >= span[0] && p < span[1] {
			return 0, 0, false
		}
	}
	var candidates []tagPair
	for _, pair := range pairs {
		if p >= pair.openStart && p < pair.closeEnd {
			candidates = append(candidates, pair)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].closeEnd-candidates[i].openStart < candidates[j].closeEnd-candidates[j].openStart
	})
	if len(candidates) == 0 {
		for _, pair := range pairs {
			if pair.openStart >= p && (len(candidates) == 0 || pair.openStart < candidates[0].openStart) {
				candidates = []tagPair{pair}
			}
		}
	}
	if count < 1 {
		count = 1
	}
	if len(candidates) < count {
		return 0, 0, false
	}
	pair := candidates[count-1]
	if around {
		return pair.openStart, pair.closeEnd, true
	}
	return pair.openEnd, pair.closeStart, true
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
	if isCJK(r) {
		return 3
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
		for step := 1; step < count; step++ {
			if end >= n || e.text[end] == '\n' {
				return 0, 0, false
			}
			c2 := e.runClass(e.text[end], big)
			for end < n && e.runClass(e.text[end], big) == c2 {
				end++
			}
		}
		return start, end, true
	}
	// Around word ('aw' / 'aW'). Counts count words, while intervening
	// whitespace is included in the selected span.
	if cls == 0 {
		for step := 0; step < count; step++ {
			for end < n && isSpaceNotNL(e.text[end]) {
				end++
			}
			if end >= n || e.text[end] == '\n' {
				e.cursor = max(0, min(n-1, end))
				return 0, 0, false
			}
			c2 := e.runClass(e.text[end], big)
			for end < n && e.runClass(e.text[end], big) == c2 {
				end++
			}
			if step+1 < count {
				for end < n && isSpaceNotNL(e.text[end]) {
					end++
				}
			}
		}
		return start, end, true
	}
	for step := 1; step < count; step++ {
		for end < n && isSpaceNotNL(e.text[end]) {
			end++
		}
		if end >= n || e.text[end] == '\n' {
			e.cursor = max(0, min(n-1, end))
			return 0, 0, false
		}
		c2 := e.runClass(e.text[end], big)
		for end < n && e.runClass(e.text[end], big) == c2 {
			end++
		}
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
	if lead == 0 || (lead > 0 && e.text[lead-1] == '\n') {
		lead = start
	}
	return lead, end, true
}

func (e *Editor) textObjectQuote(around bool, q rune, count int) (int, int, bool) {
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
		if e.text[i] == '\\' {
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
			if k > 0 && positions[k-1] < p {
				if around {
					return positions[k-1], a + 1, true
				}
				return positions[k-1] + 1, a, true
			}
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
		if count >= 2 {
			return openIdx, closeIdx + 1, true
		}
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
	if !ok && !(e.text[p] == open && count > 1) {
		// When the cursor is before a pair, Vim searches forward. A count then
		// selects the counted nested opening delimiter (2di( from before
		// "(a (b) c)" selects b), while a count from inside walks outward.
		first := -1
		for i := p; i < n; i++ {
			if e.text[i] == open {
				first = i
				break
			}
		}
		if first >= 0 {
			o, c, ok = findEnclosingFrom(first, 1)
			for level := 1; ok && level < count; level++ {
				nested := -1
				for i := o + 1; i < c; i++ {
					if e.text[i] == open {
						nested = i
						break
					}
				}
				if nested < 0 {
					ok = false
					break
				}
				o, c, ok = findEnclosingFrom(nested, 1)
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
	isBlank := func(i int) bool { return isBlankLine(e.text, lines[i][0], lines[i][1]) }
	first, last := cur, cur
	blank := isBlank(cur)
	for first > 0 && isBlank(first-1) == blank {
		first--
	}
	for last+1 < len(lines) && isBlank(last+1) == blank {
		last++
	}
	if !around {
		for step := 1; step < count; step++ {
			if last+1 >= len(lines) {
				return 0, 0, false
			}
			nextBlank := isBlank(last + 1)
			for last+1 < len(lines) && isBlank(last+1) == nextBlank {
				last++
			}
		}
	} else {
		if blank {
			if last+1 >= len(lines) {
				return 0, 0, false
			}
			last++
			for last+1 < len(lines) && !isBlank(last+1) {
				last++
			}
		}
		for step := 1; step < count; step++ {
			i := last + 1
			for i < len(lines) && isBlank(i) {
				i++
			}
			if i >= len(lines) {
				return 0, 0, false
			}
			last = i
			for last+1 < len(lines) && !isBlank(last+1) {
				last++
			}
		}
		if last+1 < len(lines) {
			for last+1 < len(lines) && isBlank(last+1) {
				last++
			}
		} else if !blank {
			for first > 0 && isBlank(first-1) {
				first--
			}
		}
	}
	start := lines[first][0]
	end := lines[last][1]
	if end < len(e.text) && e.text[end] == '\n' {
		end++
	}
	return start, end, true
}
