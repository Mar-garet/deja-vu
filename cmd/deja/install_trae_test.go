package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// `deja install trae` writes the block `traex mcp add deja -- <deja> mcp`
// writes on traecli 0.207.1: [mcp_servers.deja] in ${TRAE_HOME}/traecli.toml,
// command and args, no type.
func TestInstallTraeMCP(t *testing.T) {
	home := filepath.Join(hermeticEnv(t), "home")
	cfg := filepath.Join(home, ".trae", "traecli.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := "model = \"luna\"\n\n[mcp_servers.other]\ncommand = \"x\"\n"
	if err := os.WriteFile(cfg, []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := installTarget("trae", "/bin/deja", false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Path != cfg {
		t.Fatalf("path = %q, want %q", r.Path, cfg)
	}
	got := readString(t, cfg)
	command, args := mcpCommandArgs("/bin/deja")
	want := fmt.Sprintf("[mcp_servers.deja]\ncommand = %q\nargs = %s\n", command, tomlStringArray(args))
	if !strings.HasPrefix(got, mine) || !strings.Contains(got, want) {
		t.Fatalf("traecli.toml:\n%s", got)
	}
	if strings.Contains(got, "type =") {
		t.Fatalf("traecli.toml carries a type key TRAE does not write:\n%s", got)
	}
	if _, err := installTarget("trae", "/bin/deja", true); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, cfg); got != mine {
		t.Fatalf("after uninstall:\n%q\nwant\n%q", got, mine)
	}

	// TRAE_HOME moves the config with the rest of TRAE's home.
	t.Setenv("TRAE_HOME", filepath.Join(home, "th"))
	if r, err := installTarget("trae", "/bin/deja", false); err != nil || r.Path != filepath.Join(home, "th", "traecli.toml") {
		t.Fatalf("TRAE_HOME install = %q, %v", r.Path, err)
	}
}

// `deja install trae-auto` puts Codex's hook set into the hooks.json TRAE
// reads — ${TRAECLI_HOME:-$TRAE_HOME/cli}/hooks.json, the destination `traex
// migrate hooks --user` names — and uninstall returns both files as they were,
// taking with it the trust pins TRAE keeps in traecli.toml.
func TestInstallTraeAuto(t *testing.T) {
	home := filepath.Join(hermeticEnv(t), "home")
	trae := filepath.Join(home, ".trae")
	hooksPath := filepath.Join(trae, "cli", "hooks.json")
	cfg := filepath.Join(trae, "traecli.toml")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o755); err != nil {
		t.Fatal(err)
	}
	ownHooks := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"say done"}]}]}}` + "\n"
	ownCfg := "model = \"luna\"\n"
	if err := os.WriteFile(hooksPath, []byte(ownHooks), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(ownCfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := installTarget("trae-auto", "/bin/deja", false); err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(readString(t, hooksPath)), &root); err != nil {
		t.Fatal(err)
	}
	hooks, _ := root["hooks"].(map[string]any)
	for _, h := range codexHookWiring {
		if !strings.Contains(traeJSON(t, hooks[h.Event]), h.Sub) {
			t.Errorf("%s has no %s: %v", h.Event, h.Sub, hooks[h.Event])
		}
	}
	if hooks["Stop"] == nil {
		t.Errorf("the reader's Stop hook is gone: %v", hooks)
	}
	if !strings.Contains(readString(t, cfg), "[mcp_servers.deja]") {
		t.Errorf("traecli.toml has no server:\n%s", readString(t, cfg))
	}
	// Codex's trust is not TRAE's: nothing deja did here may touch it.
	if _, err := os.Stat(filepath.Join(home, ".codex", "hooks.json")); err == nil {
		t.Errorf("trae-auto wrote codex's hooks.json")
	}

	// TRAE pins each hook it was allowed to run in its own config.
	pin := "\n[hooks.state." + strconv.Quote(hooksPath+":session_start:0:0") + "]\ntrusted_hash = \"sha256:x\"\n"
	if err := os.WriteFile(cfg, []byte(readString(t, cfg)+"\n[hooks.state]\n"+pin), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := installTarget("trae-auto", "/bin/deja", true); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, hooksPath); got != ownHooks {
		t.Errorf("hooks.json after uninstall:\n%q\nwant\n%q", got, ownHooks)
	}
	if got := readString(t, cfg); got != ownCfg {
		t.Errorf("traecli.toml after uninstall:\n%q\nwant\n%q", got, ownCfg)
	}

	// TRAECLI_HOME moves the hooks file, not the config.
	t.Setenv("TRAECLI_HOME", filepath.Join(home, "state"))
	if _, err := installTarget("trae-auto", "/bin/deja", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "state", "hooks.json")); err != nil {
		t.Errorf("TRAECLI_HOME: %v", err)
	}
}

func traeJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
