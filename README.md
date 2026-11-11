# vimline

A framework-neutral vim line editor and human time grammar for Go. The state
machine lives here; every consumer renders.

Three packages:

- `vimline` - the editor: modal insert, normal, replace, and visual states,
  motions, operators with counts, text objects, search, marks, shifts, one
  unnamed register, undo and redo, replace mode, visual mode (charwise `v`,
  linewise `V`, blockwise `ctrl-v`), dot repeat, and the `jj` exit idiom.
- `vimline/duration` - the time grammar: `3h`, `2w`, `1d12h`, `tomorrow`,
  `clear`, resolving to an absolute time plus a human description.
- `vimline/keys` - the chord layer: `Split` and `Dispatcher`, detailed below.

No bubbletea, lipgloss, tcell, or ANSI anywhere in the module. `Editor.View`
returns a plain-data snapshot and the consumer paints it.

## Editor API

```go
e := vimline.New("alpha beta")
for {
    act := e.Feed(key)        // one logical key name or printable rune
    switch act {
    case vimline.ActAccept:   // enter
    case vimline.ActOut:      // esc from normal: leave the field
    case vimline.ActUp, vimline.ActDown: // move the surrounding list
    }
}
v := e.View("/", true)        // Label, Text, Cursor, Mode, Pending, Count,
                              // SelActive/SelStart/SelEnd/VisualKind/SelRows
```

`Feed` takes the same key names a terminal decoder produces: `enter`, `esc`,
`bs`, `left`, `right`, `up`, `down`, `ctrl-u`, `ctrl-w`, `ctrl-k`, `ctrl-y`,
`ctrl-r`, `ctrl-n`, `ctrl-p`, `ctrl-d`, `ctrl-v`, or a single printable
character.

Visual mode: `v` / `V` / `ctrl-v` enter charwise, linewise, or blockwise
selection. Motions expand the selection; `i`/`a` text objects set it; operators
`d c y > < = ~ u U J` act on it. `o`/`O` swap endpoints. `esc` clears the
selection without buffer or history mutation. `gv` reselects the prior visual
range (`g` still returns `ActFirst` for field navigation; the following `v`
completes the reselect). For a blockwise selection the faithful geometry is
the per-row spans in `View.SelRows` (one inclusive span per selected row);
`SelStart`/`SelEnd` stay a flat bounding range for compatibility.

Known gap: search patterns use Go's `regexp` engine. Vim-specific pattern
atoms are not translated, so Vim pattern syntax is not supported in v0.4.0.
The Vim-to-RE2 translation layer and nvim differential pattern tests are
tracked for a later release in `wiseman-tftd`.

Known gaps recorded against nvim 0.12.5 (each verified live): a counted
`w`/`W` operator that ends in column 0 of a later line
joins the lines where nvim stops at the line end (`d2w`, `c2w` on blanks);
`d'a`/`y'a` with the apostrophe mark run charwise where nvim is linewise; a
charwise put of multi-line text leaves the cursor at the end of the put where
nvim leaves it at the start; and `>>` places the cursor in column 0 where nvim
keeps the virtual column (tabs are one rune here). Visual mode does not
implement `p`, `P`, `r`, `/?` search, or `n`/`N`; those keys are ignored and
their operands are discarded so later keys are not misread. A text object
with a charwise or linewise live selection extends word objects from the
cursor end like nvim; quote, bracket, sentence, and paragraph objects keep
the object span where nvim extends by its per-object rules (probed live:
`vli"y` on `say "hi" now` yanks `"hi"` in nvim). Multi-row blockwise text
objects keep the block rectangle; per-row word expansion is deferred as a
declared gap this release (not implemented), although matched-column probes
of `<C-v>jjiwd` on `abcd\nefgh\nijkl` matched nvim 0.12.4. Live oracle binary is
nvim 0.12.4; `=` on a leading-tab line leaves the cursor at the last column in
that binary while vimline preserves the cursor column as nvim does on
space-indented lines; Visual
`$` / MAXCOL virtual-column edges beyond the accepted unequal-line block `$`
path are the same family as the `>>` virtual-column gap and are deferred - see
task oracle notes when comparing. Within that deferral (probed on nvim 0.12.4):
when the cursor row of a ragged `$` block ends before the block column, nvim
moves the block's left edge to that row's EOL and records empty register lines
for the short rows, where vimline keeps the anchor column (the short rows'
empty register lines agree); when the cursor row is long and only other rows
are ragged, nvim pads those rows' register lines with spaces where vimline
records empty lines.

## Binding acceptance

These seven behaviors are the contract. `editor_test.go` and
`duration/duration_test.go` pin each one and travel as the executable spec.

1. `m` means months, never minutes. Minutes are `min` or `mi`. The resolved
   absolute time is returned in the description, and consumers must surface it
   while the user types so an ambiguous unit is visible before it commits.
2. The time parse is whole-string. Leftover text after the duration tokens
   rejects the input, so `2w extra` fails and nothing parses partially.
3. Months are calendar arithmetic (`AddDate(0, n, 0)`), not fixed 30-day
   spans, so a month from October 31 lands on December 1.
4. Compound sequences work (`1d12h`). A bare unit means one (`h`), `a` and
   `an` mean one, a leading `in ` is stripped, `tomorrow`/`tom`/`tmr` is the
   next day at 09:00, and `clear`/`none`/`off`/`x` removes the reminder.
5. `jj` leaves insert mode. The first `j` is held (`PendingJ`); the consumer
   gives the pair a short window (herdr-recall uses 150 ms) and then calls
   `FlushJ` so a lone `j` is never lost. `pendingJ` clears on `esc` and on
   leaving insert.
6. `j` and `k` in field-normal return `ActDown` and `ActUp`: a single line has
   no vertical motion, so they navigate the surrounding list instead. This is
   what keeps j/k always-navigate in every consumer.
7. `0` is the start-of-line motion unless a count is already running, when it
   is a digit, so `10l` moves ten cells right.

## The keys package (v0.2.0)

`vimline/keys` owns the chord layer. `Split` decodes one terminal read into
logical key names in order, so a burst like `ggt` in a single read keeps every
key and no prefix state is lost to read batching. `Dispatcher` runs the
pending-prefix machine:

```go
d := keys.New()
d.Register("g", map[string]string{"g": "first", "t": "next-account"}, "t account")
switch a, r := d.Feed(key); r {
case keys.Chord:   // a.Name completed, a.Count rode it
case keys.Consumed: // count, prefix, or cancellation: do nothing
case keys.Pass:    // yours: pair it with d.TakeCount() when a count ran
}
```

The discipline is binding and test-pinned: one pending prefix at a time, a
count rides the chord and survives across the prefix, a key matching no
continuation cancels and is swallowed, `esc` cancels clean, and no timers.
Bare `0` passes through so surfaces can bind it; `0` inside a running count is
a digit. The machine ships with zero chords registered: every consumer owns
its table, so `gt` can mean accounts on one surface and tabs on another
without coupling releases.

## Install

```
go get github.com/RooseveltAdvisors/vimline@v0.4.0
```

Tagged releases only. Pin deliberately; do not float a branch head. The pinned
toolchain is Go 1.26.x; `.github/workflows/ci.yml` is the authority for that
version.

## License

MIT.

## Consumers and the drift gate

herdr-recall imports this module at a tagged release and its full suite runs
against the module, so every release is proven by a real consumer.

superhuman (RooseveltAdvisors/superhuman) migrated in PR #32 (commit `705924f`):
its four prefixes (g, Z, space, d) run on the Dispatcher and its hand-rolled
pending flags are deleted. Its `vimline-contract` CI job pins this module and
runs `vimline`, `vimline/duration`, and `vimline/keys` tests unmodified, so
any drift between the two surfaces fails a build instead of a code review.

This module also runs its own gate. `.github/workflows/ci.yml` runs
`go vet ./...` and `go test -count=1 ./...` on every pull request and on each
push to `main`, so a red `vimline` main surfaces on its own check run.

The routing rule for gaps: a consumer that needs different behavior sends the
gap upstream to this module. Neither side patches around the module locally.

One pin rule learned the hard way (superhuman's 705924f raced its own gate,
fixed forward in 08a708f within the hour): a consumer bumps its vimline pin
and the code using the new API in the same commit, so the drift gate never
asserts yesterday's version against today's code.
