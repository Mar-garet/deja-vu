package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `kiro-cli mcp add` writes a server as command, args, env. Install then
// uninstall put args first in every other server, because only the top-level
// order was given back; the file never came back to its bytes (#4306). Cursor
// and zcode write their configs the same way.
func TestInstallUninstallGivesTheConfigBackByteForByte(t *testing.T) {
	const kiroAdd = "{\n  \"mcpServers\": {\n    \"other\": {\n      \"command\": \"/bin/echo\",\n      \"args\": [\n        \"hi\"\n      ],\n      \"env\": {}\n    }\n  }\n}\n"
	for _, tc := range []struct {
		target, rel, body string
	}{
		{"kiro", ".kiro/settings/mcp.json", kiroAdd},
		{"cursor", ".cursor/mcp.json", kiroAdd},
		{"zcode", ".zcode/cli/config.json", "{\n  \"theme\": \"dark\",\n  \"mcp\": {\n    \"servers\": {\n      \"other\": {\n        \"type\": \"stdio\",\n        \"command\": \"/bin/echo\",\n        \"args\": [\n          \"hi\"\n        ]\n      }\n    }\n  },\n  \"editor\": \"vim\"\n}\n"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			path := filepath.Join(home, filepath.FromSlash(tc.rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := installTarget(tc.target, "/bin/deja", false); err != nil {
				t.Fatalf("install: %v", err)
			}
			installed, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(installed), `"deja"`) {
				t.Fatalf("install wrote no entry:\n%s", installed)
			}
			if strings.Index(string(installed), `"command": "/bin/echo"`) > strings.Index(string(installed), `"args"`) {
				t.Errorf("install re-sorted the other server's keys:\n%s", installed)
			}
			if _, err := installTarget(tc.target, "/bin/deja", true); err != nil {
				t.Fatalf("uninstall: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.body {
				t.Errorf("uninstall did not give the file back:\n--- before\n%s--- after\n%s", tc.body, got)
			}
		})
	}
}

// The order is the reader's at every depth, and a key deja adds goes after
// theirs.
func TestMarshalConfigLikeKeepsNestedOrder(t *testing.T) {
	old := []byte("{\n  \"b\": {\n    \"z\": 1,\n    \"a\": [\n      {\n        \"y\": 1,\n        \"x\": 2\n      }\n    ]\n  },\n  \"a\": 1\n}")
	root := map[string]any{
		"a": 1.0,
		"b": map[string]any{
			"z": 1.0,
			"a": []any{map[string]any{"y": 1.0, "x": 2.0}, map[string]any{"x": 3.0, "y": 4.0}},
			"m": "new",
		},
	}
	got, err := marshalConfigLike(old, root)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"b\": {\n    \"z\": 1,\n    \"a\": [\n      {\n        \"y\": 1,\n        \"x\": 2\n      },\n      {\n        \"y\": 4,\n        \"x\": 3\n      }\n    ],\n    \"m\": \"new\"\n  },\n  \"a\": 1\n}"
	if string(got) != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
