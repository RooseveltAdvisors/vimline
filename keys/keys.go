// Package keys owns the chord layer: a terminal-read splitter and a
// pending-prefix dispatcher. The machine lives here; every consumer registers
// its own chords, so this module ships no keymap of its own.
//
// The discipline, agreed between consumers: one pending prefix at a time, a
// count rides the chord and survives across the prefix, a key that matches no
// continuation cancels and is swallowed, esc cancels clean, and there are no
// timers anywhere.
package keys

import "unicode/utf8"

// Result reports what Feed did with a key.
type Result int

const (
	// Chord means a registered chord completed and Action is valid.
	Chord Result = iota
	// Consumed means the key was spent as a count, a prefix, or a
	// cancellation. The caller does nothing with it.
	Consumed
	// Pass means the key is not the dispatcher's. The caller handles it and
	// pairs it with TakeCount when a count was running.
	Pass
)

// Action is a completed chord. Count is the typed count, 0 when none was
// typed; apply your own default, usually 1.
type Action struct {
	Name  string
	Count int
}

type leader struct {
	chords map[string]string
	hint   string
}

// Dispatcher runs the pending-prefix state machine for one surface.
type Dispatcher struct {
	count   int
	pending string
	leaders map[string]leader
}

// New returns a dispatcher with no chords registered.
func New() *Dispatcher {
	return &Dispatcher{leaders: map[string]leader{}}
}

// Register installs one leader prefix. Prefixes and continuations are logical
// key names as produced by Split, so multi-byte names such as " " (space) or
// "ctrl+a" work as chords. Hint is returned by Pending for status lines.
func (d *Dispatcher) Register(prefix string, chords map[string]string, hint string) {
	d.leaders[prefix] = leader{chords: chords, hint: hint}
}

// Pending reports the active prefix and its hint.
func (d *Dispatcher) Pending() (prefix, hint string, ok bool) {
	if d.pending == "" {
		return "", "", false
	}
	reg := d.leaders[d.pending]
	return d.pending, reg.hint, true
}

// Count returns the accumulated count without clearing it.
func (d *Dispatcher) Count() int { return d.count }

// TakeCount returns and clears the accumulated count, so a caller can pair a
// passed-through key such as j with whatever count preceded it.
func (d *Dispatcher) TakeCount() int {
	c := d.count
	d.count = 0
	return c
}

// Feed runs one logical key through the machine.
func (d *Dispatcher) Feed(key string) (Action, Result) {
	if d.pending != "" {
		reg := d.leaders[d.pending]
		d.pending = ""
		if name, ok := reg.chords[key]; ok {
			a := Action{Name: name, Count: d.count}
			d.count = 0
			return a, Chord
		}
		// A key that matches no continuation cancels and is swallowed, the
		// way an aborted vim operator consumes its motion key.
		d.count = 0
		return Action{}, Consumed
	}
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		d.count = d.count*10 + int(key[0]-'0')
		return Action{}, Consumed
	}
	// 0 is a count digit only while a count is running; bare 0 passes
	// through so surfaces can bind it (start of line, a view, anything).
	if key == "0" && d.count > 0 {
		d.count *= 10
		return Action{}, Consumed
	}
	if key == "esc" && d.count > 0 {
		d.count = 0
		return Action{}, Consumed
	}
	if reg, ok := d.leaders[key]; ok {
		d.pending = key
		_ = reg
		return Action{}, Consumed
	}
	return Action{}, Pass
}

// ---------------------------------------------------------------------------
// Split
// ---------------------------------------------------------------------------

// Split decodes one terminal read into logical key names, in order. A burst
// that arrives in a single read, such as "ggt", yields every key it contains,
// so no prefix state is lost to read batching. Lone escape is "esc"; escape
// followed by a printable is alt-modifier input and yields "esc" then the
// key, which is the timeout-free resolution of that ambiguity.
func Split(buf []byte) []string {
	var out []string
	i := 0
	for i < len(buf) {
		b := buf[i]
		switch {
		case b == '\r' || b == '\n':
			out = append(out, "enter")
			i++
		case b == 0x7f || b == 0x08:
			out = append(out, "bs")
			i++
		case b == 0x15:
			out = append(out, "ctrl-u")
			i++
		case b == 0x0e:
			out = append(out, "ctrl-n")
			i++
		case b == 0x10:
			out = append(out, "ctrl-p")
			i++
		case b == 0x04:
			out = append(out, "ctrl-d")
			i++
		case b == 0x1b:
			if name, n := decodeEscape(buf[i:]); n > 0 {
				out = append(out, name)
				i += n
			} else {
				out = append(out, "esc")
				i++
			}
		case b >= 0x20 && b < 0x7f:
			out = append(out, string(b))
			i++
		default:
			r, size := utf8.DecodeRune(buf[i:])
			if r == utf8.RuneError && size <= 1 {
				i++
				continue
			}
			out = append(out, string(r))
			i += size
		}
	}
	return out
}

// decodeEscape matches one escape sequence at the start of b and returns its
// logical name and byte length. A length of 0 means b does not open a CSI
// sequence, so the caller emits esc and reprocesses the rest.
func decodeEscape(b []byte) (string, int) {
	if len(b) < 2 || b[1] != '[' {
		return "", 0
	}
	j := 2
	for j < len(b) && !isAlpha(b[j]) {
		j++
	}
	if j >= len(b) {
		return "esc", len(b)
	}
	final := b[j]
	if final == 'u' && j > 2 {
		code := 0
		for _, c := range b[2:j] {
			if c >= '0' && c <= '9' {
				code = code*10 + int(c-'0')
			}
		}
		switch code {
		case 13:
			return "enter", j + 1
		case 27:
			return "esc", j + 1
		case 127, 8:
			return "bs", j + 1
		case 57417:
			return "left", j + 1
		case 57418:
			return "right", j + 1
		case 57419:
			return "up", j + 1
		case 57420:
			return "down", j + 1
		}
		if code >= 32 && code < 127 {
			return string(byte(code)), j + 1
		}
		return "esc", j + 1
	}
	switch final {
	case 'A':
		return "up", j + 1
	case 'B':
		return "down", j + 1
	case 'C':
		return "right", j + 1
	case 'D':
		return "left", j + 1
	}
	return "esc", j + 1
}

func isAlpha(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}
