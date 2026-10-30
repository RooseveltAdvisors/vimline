package vimline

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type oracleCase struct {
	Name   string   `json:"name"`
	Text   string   `json:"text"`
	Pos    int      `json:"pos"`
	Row    int      `json:"row"`
	Col    int      `json:"col"`
	Keys   []string `json:"keys"`
	Seq    string   `json:"seq"`
	Result oracleResult
}

type oracleResult struct {
	Text string `json:"text"`
	Pos  int    `json:"pos"`
	Reg  string `json:"reg"`
}

func TestNeovimDifferentialV031Acceptance(t *testing.T) {
	if _, err := exec.LookPath("nvim"); err != nil {
		t.Skip("nvim is not installed; deterministic oracle regressions still run")
	}
	cases := []oracleCase{
		{Name: "cw dot", Text: "one (a (b) c)", Pos: 0, Keys: splitOracleKeys("cwQQ<esc>.")},
		{Name: "operator underscore", Text: "one (two)\nthree four\n\nfive", Pos: 6, Keys: splitOracleKeys("d_")},
		{Name: "inner paragraph", Text: "one\ntwo\n\nthree", Pos: 2, Keys: splitOracleKeys("cipQ<esc>")},
		{Name: "counted pair ahead", Text: "one (a (b) c)", Pos: 0, Keys: splitOracleKeys("2di(")},
		{Name: "yank placement", Text: "one two three", Pos: 5, Keys: splitOracleKeys("yiw")},
		{Name: "delete placement", Text: "one (two)\nthree four\n\nfive", Pos: 6, Keys: splitOracleKeys("dgg")},
		{Name: "atomic undo", Text: "one (a (b) c)", Pos: 0, Keys: splitOracleKeys("cwQQ<esc>u")},
		{Name: "atomic redo", Text: "one (a (b) c)", Pos: 0, Keys: splitOracleKeys("cwQQ<esc>u<ctrl-r>")},
		{Name: "replace mode", Text: "abcdef", Pos: 0, Keys: splitOracleKeys("RXYZ<esc>")},
		{Name: "open below", Text: "one\ntwo", Pos: 0, Keys: splitOracleKeys("oQQ<esc>")},
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
		cases[i].Seq = strings.Join(cases[i].Keys, "")
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
  vim.cmd('normal! ' .. c.seq)
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
				case "\x12":
					key = "ctrl-r"
				case "\x08":
					key = "bs"
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

func splitOracleKeys(seq string) []string {
	replacer := strings.NewReplacer("<esc>", "\x1b", "<ctrl-r>", "\x12", "<bs>", "\x08")
	return strings.Split(replacer.Replace(seq), "")
}
