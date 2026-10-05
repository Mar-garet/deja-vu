package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/index"
)

// `deja install trae-ide` writes the entry TRAE IDE 3.5.104 loaded live: under
// mcpServers in <user data>/User/mcp.json, command and args and no type key,
// which the IDE's schema rejects. The skill goes to ~/.trae/skills. A second
// run changes nothing, and uninstall leaves both files as they were.
func TestInstallTraeIDEMCPAndSkill(t *testing.T) {
	home := filepath.Join(hermeticEnv(t), "home")
	dir := index.DefaultDir()
	mcp := traeIDEMCPPath()
	if err := os.MkdirAll(filepath.Dir(mcp), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := `{"mcpServers":{"other":{"command":"x","args":["y"],"disabled":true}}}` + "\n"
	if err := os.WriteFile(mcp, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	install := func(uninstall bool) {
		t.Helper()
		captureStdout(t, func() {
			if err := runInstall(dir, []string{"trae-ide"}, uninstall); err != nil {
				t.Fatal(err)
			}
		})
	}

	// The exact entry, from installTarget with a known binary.
	if _, err := installTarget("trae-ide", "/bin/deja", false); err != nil {
		t.Fatal(err)
	}
	var root struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(readString(t, mcp)), &root); err != nil {
		t.Fatal(err)
	}
	command, args := mcpCommandArgs("/bin/deja")
	wantArgs := make([]any, len(args))
	for i, a := range args {
		wantArgs[i] = a
	}
	if want := map[string]any{"command": command, "args": wantArgs}; !reflect.DeepEqual(root.MCPServers["deja"], want) {
		t.Errorf("deja entry = %v, want %v (no type key: TRAE IDE rejects it)", root.MCPServers["deja"], want)
	}
	if !reflect.DeepEqual(root.MCPServers["other"], map[string]any{"command": "x", "args": []any{"y"}, "disabled": true}) {
		t.Errorf("the reader's server changed: %v", root.MCPServers["other"])
	}

	install(false)
	first := readString(t, mcp)
	skill := filepath.Join(home, ".trae", "skills", "deja-history", "SKILL.md")
	if !strings.Contains(readString(t, skill), "name: deja-history") {
		t.Errorf("no skill at %s", skill)
	}

	install(false)
	if got := readString(t, mcp); got != first {
		t.Errorf("a second install changed mcp.json:\n%s\nwas\n%s", got, first)
	}

	install(true)
	if got := readString(t, mcp); got != mine {
		t.Errorf("mcp.json after uninstall:\n%q\nwant\n%q", got, mine)
	}
	if _, err := os.Stat(skill); !os.IsNotExist(err) {
		t.Errorf("skill left after uninstall: %v", err)
	}
}

// `deja install trae-ide-auto` puts Claude-shaped hooks in ~/.trae/hooks.json,
// keeps the reader's own, says the IDE runs none of them until hooks are on,
// and leaves TRAE CLI's hooks file alone. Uninstall returns the file byte for
// byte.
func TestInstallTraeIDEAuto(t *testing.T) {
	home := filepath.Join(hermeticEnv(t), "home")
	hooksPath := filepath.Join(home, ".trae", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o755); err != nil {
		t.Fatal(err)
	}
	own := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"say done"}]}]}}` + "\n"
	if err := os.WriteFile(hooksPath, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "install", "trae-ide-auto", "--no-index")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Settings > Hooks") {
		t.Errorf("install did not say where hooks are turned on:\n%s", out)
	}
	first := readString(t, hooksPath)
	var root map[string]any
	if err := json.Unmarshal([]byte(first), &root); err != nil {
		t.Fatal(err)
	}
	hooks, _ := root["hooks"].(map[string]any)
	for _, h := range traeIDEHookWiring {
		if !strings.Contains(traeJSON(t, hooks[h.Event]), h.Sub) {
			t.Errorf("%s has no %s: %v", h.Event, h.Sub, hooks[h.Event])
		}
	}
	if hooks["SessionEnd"] != nil {
		t.Errorf("TRAE IDE has no SessionEnd event: %v", hooks["SessionEnd"])
	}
	if !strings.Contains(traeJSON(t, hooks["Stop"]), "say done") {
		t.Errorf("the reader's Stop hook is gone: %v", hooks["Stop"])
	}
	if _, err := os.Stat(filepath.Join(home, ".trae", "cli", "hooks.json")); err == nil {
		t.Error("trae-ide-auto wrote TRAE CLI's hooks.json")
	}
	if !strings.Contains(readString(t, traeIDEMCPPath()), `"deja"`) {
		t.Error("trae-ide-auto wrote no MCP entry")
	}

	if _, err := captureRun(t, "install", "trae-ide-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, hooksPath); got != first {
		t.Errorf("a second install changed hooks.json:\n%s\nwas\n%s", got, first)
	}

	if _, err := captureRun(t, "uninstall", "trae-ide-auto"); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, hooksPath); got != own {
		t.Errorf("hooks.json after uninstall:\n%q\nwant\n%q", got, own)
	}
}

// The CN build is "Trae CN" with ~/.trae-cn. Where only it has user data,
// every file goes there.
func TestTraeIDECNEdition(t *testing.T) {
	home := filepath.Join(hermeticEnv(t), "home")
	cn := filepath.Join(traeIDEAppDir("Trae CN"), "User")
	if err := os.MkdirAll(cn, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := traeIDEMCPPath(); got != filepath.Join(cn, "mcp.json") {
		t.Errorf("mcp path %q", got)
	}
	if got := traeIDEHooksPath(); got != filepath.Join(home, ".trae-cn", "hooks.json") {
		t.Errorf("hooks path %q", got)
	}
	if got := guidancePath("trae-ide"); got != filepath.Join(home, ".trae-cn", "skills", "deja-history", "SKILL.md") {
		t.Errorf("skill path %q", got)
	}
	// Both present: the international build wins.
	if err := os.MkdirAll(filepath.Join(traeIDEAppDir("Trae"), "User"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := traeIDEHooksPath(); got != filepath.Join(home, ".trae", "hooks.json") {
		t.Errorf("hooks path with both builds %q", got)
	}
}

// Doctor names both halves and says the hooks may not run: the IDE's switch is
// off by default and not in a file deja can read.
func TestDoctorTraeIDERows(t *testing.T) {
	hermeticEnv(t)
	if _, err := captureRun(t, "install", "trae-ide-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	var auto bytes.Buffer
	doctorAutoRecall(&auto)
	var row string
	for _, l := range strings.Split(auto.String(), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "trae-ide ") {
			row = l
		}
	}
	if !strings.Contains(row, "wired") {
		t.Errorf("hooks row %q:\n%s", row, auto.String())
	}
	if !strings.Contains(auto.String(), traeIDEHooksOffNote) {
		t.Errorf("doctor did not say the hooks may be off:\n%s", auto.String())
	}
	for _, c := range doctorMCPConfigs() {
		if c.name == "trae-ide" && !c.wired(c.path) {
			t.Errorf("MCP row reads %s as not wired", c.path)
		}
	}
}

// TRAE IDE's hook payload names the tool llm_tool_name; until a live capture
// says whether tool_name is sent too, either is read.
func TestHookToolReadsLLMToolName(t *testing.T) {
	for _, payload := range []string{
		`{"llm_tool_name":"write_to_file","tool_input":{"file_path":"/p/a.go"}}`,
		`{"tool_name":"write_to_file","llm_tool_name":"ignored","tool_input":{"file_path":"/p/a.go"}}`,
	} {
		var in toolHookInput
		if err := json.Unmarshal([]byte(payload), &in); err != nil {
			t.Fatal(err)
		}
		in.adopt()
		if in.ToolName != "write_to_file" || in.ToolInput.FilePath != "/p/a.go" {
			t.Errorf("%s: tool %q path %q", payload, in.ToolName, in.ToolInput.FilePath)
		}
	}
	var after toolAfterInput
	if err := json.Unmarshal([]byte(`{"llm_tool_name":"run_command"}`), &after); err != nil {
		t.Fatal(err)
	}
	if after.LLMToolName != "run_command" || !isCommandTool(after.LLMToolName) {
		t.Errorf("hook-tool-after does not read llm_tool_name: %+v", after)
	}
}
