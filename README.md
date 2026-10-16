# vimline

A framework-neutral vim line editor and human time grammar for Go. The state
machine lives here; every consumer renders.

Two packages:

- `vimline` - the editor: modal insert, normal, and replace states, motions,
  operators with counts, one unnamed register, undo and redo, replace mode,
  dot repeat, and the `jj` exit idiom.
- `vimline/duration` - the time grammar: `3h`, `2w`, `1d12h`, `tomorrow`,
  `clear`, resolving to an absolute time plus a human description.

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
v := e.View("/", true)        // Label, Text, Cursor, Mode, Pending, Count
```

`Feed` takes the same key names a terminal decoder produces: `enter`, `esc`,
`bs`, `left`, `right`, `up`, `down`, `ctrl-u`, `ctrl-w`, `ctrl-k`, `ctrl-y`,
`ctrl-r`, `ctrl-n`, `ctrl-p`, `ctrl-d`, or a single printable character.

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

## Install

```
go get github.com/RooseveltAdvisors/vimline@v0.1.0
```

Tagged releases only. Pin deliberately; do not float a branch head.

## License

MIT.
