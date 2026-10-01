package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cordis.patch.yml names deja's plugin files by path, and dsh refuses the whole
// profile when one of them is gone. Doctor read the layer for the server alone
// and called it wired while dsh would not start (#4292).
func TestDoctorSaysDSHWillNotStartWhenAPluginFileIsGone(t *testing.T) {
	hermeticEnv(t)
	home := os.Getenv("HOME")
	t.Setenv("DSH_HOME", filepath.Join(home, ".dsh"))
	bin := filepath.Join(home, "bin", "deja")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := installDeepSeekAuto(bin, false); err != nil {
		t.Fatal(err)
	}
	mcpRow := func() map[string]any {
		t.Helper()
		out, err := captureRun(t, "doctor", "--json")
		if err != nil {
			t.Fatal(err)
		}
		var report struct {
			MCP []map[string]any `json:"mcp"`
		}
		if err := json.Unmarshal([]byte(out), &report); err != nil {
			t.Fatalf("doctor --json: %v: %s", err, out)
		}
		for _, r := range report.MCP {
			if r["name"] == "deepseek" {
				return r
			}
		}
		t.Fatalf("no deepseek row:\n%s", out)
		return nil
	}

	// Control: everything the layer names is there.
	if got := mcpRow(); got["state"] != "wired" || got["plugin_missing"] != nil {
		t.Errorf("a complete install reads %v, want wired and no plugin_missing", got)
	}
	text, err := captureRun(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "dsh will not start") {
		t.Errorf("a complete install was reported as broken:\n%s", text)
	}

	if err := os.Remove(dshAutoPath()); err != nil {
		t.Fatal(err)
	}
	if got := mcpRow(); got["plugin_missing"] != true {
		t.Errorf("a layer naming a deleted plugin reads %v, want plugin_missing", got)
	}
	text, err = captureRun(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{reportPath(dshAutoPath()) + ", which is not there", "dsh will not start", "deja install deepseek-auto", "deja uninstall deepseek", "still names it"} {
		if !strings.Contains(text, want) {
			t.Errorf("doctor does not say %q:\n%s", want, text)
		}
	}
}

// The header told a reader the file could go, and dsh does not start without
// it while the layer names it (#4292).
func TestDSHPluginFilesDoNotCallThemselvesSafeToDelete(t *testing.T) {
	for name, body := range map[string]string{"command.js": dshCommandJS("/bin/deja"), "auto.js": dshAutoJS("/bin/deja")} {
		head, _, _ := strings.Cut(body, "\n")
		if strings.Contains(head, "safe to delete") {
			t.Errorf("%s opens with %q", name, head)
		}
		if !strings.Contains(head, "deja uninstall deepseek") {
			t.Errorf("%s does not say how to take it out: %q", name, head)
		}
	}
}
