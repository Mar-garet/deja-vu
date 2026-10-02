package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// OpenClaw 2026.7.1-2 warns twice on every start about a plugin it found only
// by scanning extensions/: the open allow list, and no install or load-path
// provenance. A plugin named in plugins.load.paths is origin "config", which
// both checks accept, and that leaves the user's allow list alone (#4579).
func TestOpenClawPluginInstallNamesItsLoadPath(t *testing.T) {
	for _, tc := range []struct{ name, before string }{
		{"plain", `{"plugins":{"load":{"paths":["/opt/theirs"]}}}` + "\n"},
		{"jsonc", "{\n  // mine\n  \"plugins\": {\n    \"load\": {\n      \"paths\": [\"/opt/theirs\"]\n    }\n  }\n}\n"},
		{"none", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			state := sources.OpenClawStateDir()
			if err := os.MkdirAll(state, 0o755); err != nil {
				t.Fatal(err)
			}
			cfg := filepath.Join(state, "openclaw.json")
			if tc.before != "" {
				if err := os.WriteFile(cfg, []byte(tc.before), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := installOpenClawPlugin("/bin/deja", false); err != nil {
				t.Fatalf("install: %v", err)
			}
			dir := filepath.Join(state, "extensions", openclawPluginID)
			paths := openclawLoadPathsIn(t, cfg)
			if !slices.Contains(paths, dir) {
				t.Fatalf("plugins.load.paths does not name %s, so OpenClaw warns about it on every start: %v", dir, paths)
			}
			if tc.before != "" && !slices.Contains(paths, "/opt/theirs") {
				t.Errorf("their load path is gone: %v", paths)
			}
			// Twice is once.
			if _, err := installOpenClawPlugin("/bin/deja", false); err != nil {
				t.Fatal(err)
			}
			if got := openclawLoadPathsIn(t, cfg); len(got) != len(paths) {
				t.Errorf("a second install added the path again: %v", got)
			}
			if _, err := installOpenClawPlugin("/bin/deja", true); err != nil {
				t.Fatalf("uninstall: %v", err)
			}
			after, _ := os.ReadFile(cfg)
			if strings.Contains(string(after), "extensions") {
				t.Errorf("uninstall left the load path behind:\n%s", after)
			}
			if tc.before != "" && string(after) != tc.before {
				t.Errorf("uninstall did not give the file back:\n%s\nwant:\n%s", after, tc.before)
			}
		})
	}
}

func openclawLoadPathsIn(t *testing.T, cfg string) []string {
	t.Helper()
	b, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var root struct {
		Plugins struct {
			Load struct {
				Paths []string `json:"paths"`
			} `json:"load"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(jsoncToJSON(string(b))), &root); err != nil {
		t.Fatalf("config does not parse: %v\n%s", err, b)
	}
	return root.Plugins.Load.Paths
}

// Taking out a whole block that is the last key in a JSONC file has to take
// the comma in front of it too, or the next run refuses the file.
func TestOpenClawPluginUninstallDropsLastBlockCleanly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	state := sources.OpenClawStateDir()
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(state, "openclaw.json")
	before := "{\n  // mine\n  \"gateway\": {\"mode\": \"local\"},\n  \"plugins\": {\"load\": {\"paths\": []}}\n}\n"
	if err := os.WriteFile(cfg, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installOpenClawPlugin("/bin/deja", false); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := installOpenClawPlugin("/bin/deja", true); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	after, _ := os.ReadFile(cfg)
	var root map[string]any
	if err := json.Unmarshal([]byte(stripJSONComments(string(after))), &root); err != nil {
		t.Fatalf("uninstall left a config that does not parse: %v\n%s", err, after)
	}
	if _, ok := root["gateway"]; !ok {
		t.Errorf("their gateway block is gone:\n%s", after)
	}
}
