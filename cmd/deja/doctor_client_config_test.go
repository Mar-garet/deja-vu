package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Kimi, Qwen, Cursor, Crush, ZCode and Command Code keep deja's hooks in the
// client's own config, which exists whether deja ever wrote to it or not. A
// file holding no deja entry is a machine that was never wired and reads
// missing; stale is for deja's own entry gone wrong (#4275).
func TestDoctorCallsAClientConfigWithoutDejaMissing(t *testing.T) {
	const hook = "/usr/local/bin/deja hook-precompact"
	for _, name := range []string{"kimi", "qwen", "cursor", "crush", "zcode", "commandcode"} {
		t.Run(name, func(t *testing.T) {
			tmp := hermeticEnv(t)
			t.Setenv("KIMI_CODE_HOME", "")
			t.Setenv("CURSOR_CONFIG_DIR", "")
			t.Setenv("CRUSH_GLOBAL_CONFIG", "")
			var a autoWiring
			for _, w := range autoWirings() {
				if w.name == name {
					a = w
				}
			}
			path := a.path()
			if !strings.HasPrefix(path, tmp) {
				t.Fatalf("%s config %s is outside the test home", name, path)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			write := func(s string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			row := func() string {
				t.Helper()
				var buf bytes.Buffer
				doctorAutoRecall(&buf)
				for _, l := range strings.Split(buf.String(), "\n") {
					if strings.HasPrefix(l, "  "+name+" ") {
						return l
					}
				}
				t.Fatalf("no %s row:\n%s", name, buf.String())
				return ""
			}

			// The client's own settings, and an MCP server that is not a hook.
			if name == "kimi" {
				write("default_model = \"luna\"\n")
			} else {
				write(`{"model": "luna", "mcpServers": {"deja": {"command": "/usr/local/bin/deja", "args": ["mcp"]}}}`)
			}
			if state, _ := autoWiringState(a); state != "missing" {
				t.Errorf("a config with no deja hook: state %q, want missing", state)
			}
			if l := row(); !strings.Contains(l, "missing") || strings.Contains(l, "stale") {
				t.Errorf("a config with no deja hook: %q, want missing", l)
			}
			if !nothingWired() {
				t.Error("a config with no deja hook counted as a wired agent")
			}

			// Control: deja's own entry without the hook the row looks for.
			if name == "kimi" {
				write("default_model = \"luna\"\n\n" + kimiHookEntry("PreCompact", hook) + "\n")
			} else {
				write(`{"hooks": {"PreCompact": [{"hooks": [{"type": "command", "command": "` + hook + `"}]}]}}`)
			}
			if state, _ := autoWiringState(a); state != "stale" {
				t.Errorf("deja's entry without the %s call: state %q, want stale", a.marker, state)
			}
			if l := row(); !strings.Contains(l, "stale") {
				t.Errorf("deja's entry without the %s call: %q, want stale", a.marker, l)
			}
			if nothingWired() {
				t.Error("deja's own entry counted as nothing wired")
			}
		})
	}
}
