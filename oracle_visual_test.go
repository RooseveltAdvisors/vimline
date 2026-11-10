package vimline

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNeovimDifferentialVisual drives identical keystroke sequences through
// vimline and live nvim (feedkeys, so " is not eaten as a register).
func TestNeovimDifferentialVisual(t *testing.T) {
	if _, err := exec.LookPath("nvim"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("nvim is required in CI so the live differential gates the suite")
		}
		t.Skip("nvim is not installed; deterministic visual regressions still run")
	}
	cases := []oracleCase{
		{Name: `vi"d`, Text: `one "two" three`, Pos: 5, Keys: splitVisualOracleKeys(`vi"d`)},
		{Name: `va"d`, Text: `xx"yy"zz`, Pos: 3, Keys: splitVisualOracleKeys(`va"d`)},
		{Name: "viwd", Text: "one two three", Pos: 4, Keys: splitVisualOracleKeys("viwd")},
		{Name: "vawd", Text: "  hello x", Pos: 2, Keys: splitVisualOracleKeys("vawd")},
		{Name: "vwd", Text: "one two three", Pos: 4, Keys: splitVisualOracleKeys("vwd")},
		{Name: "Vjd", Text: "one\ntwo\nthree", Pos: 0, Keys: splitVisualOracleKeys("Vjd")},
		{Name: "ve~", Text: "one two", Pos: 0, Keys: splitVisualOracleKeys("ve~")},
		{Name: "veU", Text: "one two", Pos: 0, Keys: splitVisualOracleKeys("veU")},
		{Name: "veu", Text: "Ab Cd", Pos: 0, Keys: splitVisualOracleKeys("veu")},
		{Name: "vllly", Text: "hello world", Pos: 0, Keys: splitVisualOracleKeys("vllly")},
		{Name: "viwcXesc", Text: "one two", Pos: 4, Keys: splitVisualOracleKeys("viwcX<esc>")},
		{Name: "block yank", Text: "hello world", Pos: 0, Keys: splitVisualOracleKeys("<C-v>lly")},
		{Name: "VjJ", Text: "a\nb\nc", Pos: 0, Keys: splitVisualOracleKeys("VjJ")},
		{Name: "vasd", Text: "One two. Three.", Pos: 0, Keys: splitVisualOracleKeys("vasd")},
		{Name: "vipd", Text: "p1\n\np2", Pos: 0, Keys: splitVisualOracleKeys("vipd")},
		{Name: "vi)d", Text: "(ab)", Pos: 1, Keys: splitVisualOracleKeys("vi)d")},
		{Name: "vd", Text: "one two", Pos: 4, Keys: splitVisualOracleKeys("vd")},
		{Name: "v$d", Text: "one two", Pos: 0, Keys: splitVisualOracleKeys("v$d")},
		{Name: "v2ld", Text: "one two", Pos: 0, Keys: splitVisualOracleKeys("v2ld")},
		{Name: "indent >", Text: "abcdef", Pos: 0, Keys: splitVisualOracleKeys("vll>")},
		{Name: "indent <", Text: "\tabcdef", Pos: 1, Keys: splitVisualOracleKeys("V<")},
		// Spaces, not tabs: live nvim 0.12.4 leaves the cursor at the last
		// column after `=` on a leading-tab line, which contradicts both the
		// first-non-blank rule and vimline's indent operator. Spaces agree.
		{Name: "indent =", Text: "  abcdef", Pos: 0, Keys: splitVisualOracleKeys("V=")},
		{Name: `vi'd`, Text: "a 'bc' d", Pos: 3, Keys: splitVisualOracleKeys("vi'd")},
		{Name: "viWd", Text: "one-two three", Pos: 0, Keys: splitVisualOracleKeys("viWd")},
		{Name: "vaWd", Text: "one-two three", Pos: 0, Keys: splitVisualOracleKeys("vaWd")},
		{Name: "vi{d", Text: "x{ab}y", Pos: 2, Keys: splitVisualOracleKeys("vi{d")},
		{Name: "va[d", Text: "x[ab]y", Pos: 2, Keys: splitVisualOracleKeys("va[d")},
		{Name: "vi<d", Text: "x<ab>y", Pos: 2, Keys: splitVisualOracleKeys("vi<d")},
		{Name: "visd", Text: "One two. Three.", Pos: 2, Keys: splitVisualOracleKeys("visd")},
		{Name: "vapd", Text: "p1\n\np2\n", Pos: 0, Keys: splitVisualOracleKeys("vapd")},
		{Name: "line Vd", Text: "one\ntwo", Pos: 0, Keys: splitVisualOracleKeys("Vd")},
		{Name: "char change", Text: "abcd", Pos: 1, Keys: splitVisualOracleKeys("vlcZ<esc>")},
		{Name: "gv reselect", Text: "abcdef", Pos: 0, Keys: splitVisualOracleKeys("vll<esc>gvd")},
		{Name: "endpoint o then d", Text: "abcdef", Pos: 0, Keys: splitVisualOracleKeys("vllood")},
		{Name: "visual backtick", Text: "a `bc` d", Pos: 3, Keys: splitVisualOracleKeys("vi`d")},
		{Name: "v$d includes newline", Text: "abc\ndef", Pos: 0, Keys: splitVisualOracleKeys("v$d")},
		{Name: "empty vi\"d", Text: `a""b`, Pos: 1, Keys: splitVisualOracleKeys(`vi"d`)},
		{Name: "blockwise c", Text: "abcd\nefgh", Pos: 0, Keys: splitVisualOracleKeys("<C-v>jlcX<esc>")},
		{Name: "linewise Vd curswant", Text: "abcdef\nxyz", Pos: 3, Keys: splitVisualOracleKeys("Vd")},
		{Name: "block $ unequal", Text: "abc\nde\nfgh", Pos: 1, Keys: splitVisualOracleKeys("<C-v>j$d")},
		{Name: "counted v entry", Text: "abcdef", Pos: 0, Keys: splitVisualOracleKeys("2vld")},
		{Name: "counted V entry", Text: "a\nb\nc\nd", Pos: 0, Keys: splitVisualOracleKeys("2Vjd")},
		{Name: "counted ctrl-v entry", Text: "abcd\nefgh", Pos: 0, Keys: splitVisualOracleKeys("2<C-v>jd")},
	}
	for i := range cases {
		before := []rune(cases[i].Text)[:cases[i].Pos]
		prefix := string(before)
		cases[i].Row = strings.Count(prefix, "\n") + 1
		linePrefix := prefix
		if at := strings.LastIndexByte(prefix, '\n'); at >= 0 {
			linePrefix = prefix[at+1:]
		}
		cases[i].Col = len([]byte(linePrefix))
		// Prefer named termcodes in the nvim seq so feedkeys sees <C-v>, not a
		// raw 0x16 byte that replace_termcodes leaves untouched.
		cases[i].Seq = visualOracleSeq(cases[i].Keys)
	}

	dir := t.TempDir()
	inPath := filepath.Join(dir, "cases.json")
	outPath := filepath.Join(dir, "results.json")
	raw, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	lua := `
local input = os.getenv('VIMLINE_ORACLE_INPUT')
local output = os.getenv('VIMLINE_ORACLE_OUTPUT')
local cases = vim.json.decode(table.concat(vim.fn.readfile(input), "\n"))
local results = {}
for _, c in ipairs(cases) do
  vim.api.nvim_buf_set_lines(0, 0, -1, false, vim.split(c.text, "\n", { plain = true }))
  vim.api.nvim_win_set_cursor(0, { c.row, c.col })
  vim.fn.setreg('"', '')
  vim.cmd('set undolevels=-1')
  vim.cmd('set undolevels=1000')
  -- feedkeys keeps " as a text-object delimiter; :normal! treats " as a register.
  vim.fn.feedkeys(vim.api.nvim_replace_termcodes(c.seq, true, false, true), 'x')
  local cursor = vim.api.nvim_win_get_cursor(0)
  local lines = vim.api.nvim_buf_get_lines(0, 0, -1, false)
  local pos = 0
  for line = 1, cursor[1] - 1 do
    pos = pos + vim.str_utfindex(lines[line]) + 1
  end
  pos = pos + vim.str_utfindex(lines[cursor[1]]:sub(1, cursor[2]))
  table.insert(results, { text = table.concat(lines, "\n"), pos = pos, reg = vim.fn.getreg('"') })
end
vim.fn.writefile({ vim.json.encode(results) }, output)
vim.cmd('qa!')
`
	luaPath := filepath.Join(dir, "oracle.lua")
	if err := os.WriteFile(luaPath, []byte(lua), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("nvim", "--headless", "-u", "NONE", "-i", "NONE", "-l", luaPath)
	cmd.Env = append(os.Environ(), "HOME="+dir, "XDG_CONFIG_HOME="+filepath.Join(dir, "config"), "XDG_STATE_HOME="+filepath.Join(dir, "state"), "VIMLINE_ORACLE_INPUT="+inPath, "VIMLINE_ORACLE_OUTPUT="+outPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("nvim oracle: %v\n%s", err, output)
	}
	resultRaw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var results []oracleResult
	if err := json.Unmarshal(resultRaw, &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != len(cases) {
		t.Fatalf("oracle returned %d results for %d cases", len(results), len(cases))
	}
	for i, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			e := New(tc.Text)
			e.SetMode(ModeNormal)
			e.SetCursor(tc.Pos)
			for _, key := range tc.Keys {
				switch key {
				case "\x1b":
					key = "esc"
				case "\x16":
					key = "ctrl-v"
				case "\x12":
					key = "ctrl-r"
				case "\x08":
					key = "bs"
				case "\r":
					key = "enter"
				}
				e.Feed(key)
			}
			want := results[i]
			if e.Text() != want.Text || e.Cursor() != want.Pos || e.Register() != want.Reg {
				t.Fatalf("vimline=(text %q, cursor %d, reg %q), nvim=(text %q, cursor %d, reg %q)",
					e.Text(), e.Cursor(), e.Register(), want.Text, want.Pos, want.Reg)
			}
		})
	}
}

func splitVisualOracleKeys(seq string) []string {
	replacer := strings.NewReplacer(
		"<esc>", "\x1b",
		"<Esc>", "\x1b",
		"<ctrl-r>", "\x12",
		"<C-v>", "\x16",
		"<c-v>", "\x16",
		"<bs>", "\x08",
		"<cr>", "\r",
	)
	return strings.Split(replacer.Replace(seq), "")
}

func visualOracleSeq(keys []string) string {
	var b strings.Builder
	for _, key := range keys {
		switch key {
		case "\x1b":
			b.WriteString("<Esc>")
		case "\x16":
			b.WriteString("<C-v>")
		case "\x12":
			b.WriteString("<C-r>")
		case "\x08":
			b.WriteString("<BS>")
		case "\r":
			b.WriteString("<CR>")
		default:
			b.WriteString(key)
		}
	}
	return b.String()
}
