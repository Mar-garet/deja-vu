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
	for _, want := range []string{reportPath(dshAutoPath()) + ", which dsh cannot find", "dsh will not start", "deja install deepseek-auto", "deja uninstall deepseek", "still names it"} {
		if !strings.Contains(text, want) {
			t.Errorf("doctor does not say %q:\n%s", want, text)
		}
	}
}

// The header told a reader the file could go, and dsh does not start without
// it while the layer names it (#4292).
func TestDSHPluginFilesDoNotCallThemselvesSafeToDelete(t *testing.T) {
	// auto.js goes with `deja install deepseek`, which keeps the server and
	// /deja; only the whole install comes out with uninstall.
	for name, c := range map[string]struct{ body, how string }{
		"command.js": {dshCommandJS("/bin/deja"), "deja uninstall deepseek"},
		"auto.js":    {dshAutoJS("/bin/deja"), "deja install deepseek"},
	} {
		head, _, _ := strings.Cut(c.body, "\n")
		if strings.Contains(head, "safe to delete") {
			t.Errorf("%s opens with %q", name, head)
		}
		if !strings.HasSuffix(head, c.how) {
			t.Errorf("%s does not end on %q: %q", name, c.how, head)
		}
	}
}

// A row can name its plugin other ways than by absolute path. Measured against
// dsh 0.1.1-rc.2: `./x` resolves against the profile's directory (not the
// layer's), a file:// URL is imported as the file, and `~/` is never expanded,
// so such a row fails whether or not the file is there (#4292).
func TestDSHPluginsMissingReadsEverySpelling(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)
	profile := filepath.Join(home, "profiles", "headless")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "profiles", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	present := filepath.Join(home, "present.js")
	if err := os.WriteFile(present, []byte("export default () => {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "here.js"), []byte("export default () => {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	layer := filepath.Join(home, "cordis.patch.yml")
	body := dshBlockStart + "\n- insert:\n" +
		"    - id: a\n      name: '@deepseek-ai/dsh-mcp-client'\n" +
		"    - id: b\n      name: ./here.js\n" +
		"    - id: c\n      name: ./gone.js\n" +
		"    - id: d\n      name: 'file://" + filepath.ToSlash(present) + "'\n" +
		"    - id: e\n      name: \"file://" + filepath.ToSlash(filepath.Join(home, "gone-url.js")) + "\"\n" +
		"    - id: f\n      name: ~/.dsh/plugins/deja/auto.js\n" +
		dshBlockEnd + "\n"
	if err := os.WriteFile(layer, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := dshPluginsMissing(layer)
	want := []string{filepath.Join(profile, "gone.js"), filepath.Join(home, "gone-url.js"), "~/.dsh/plugins/deja/auto.js"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("missing = %q, want %q", got, want)
	}
}

// Two files gone reads as two, not as one.
func TestDSHPluginsMissingNoteCountsWhatIsGone(t *testing.T) {
	layer := filepath.Join(t.TempDir(), "cordis.patch.yml")
	if err := os.WriteFile(layer, []byte(dshBlockStart+"\n    - id: deja-auto\n"+dshBlockEnd+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	one := dshPluginsMissingNote(layer, []string{"/a/command.js"})
	if !strings.Contains(one, "names /a/command.js, which dsh cannot find") {
		t.Errorf("one missing: %q", one)
	}
	two := dshPluginsMissingNote(layer, []string{"/a/command.js", "/a/auto.js"})
	if !strings.Contains(two, "names /a/command.js and /a/auto.js, which dsh cannot find") ||
		!strings.Contains(two, "writes them again") {
		t.Errorf("two missing: %q", two)
	}
}
