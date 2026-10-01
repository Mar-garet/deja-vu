package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A config whose first line is `mcp_servers:` (or that carries a comment or
// trailing spaces on the key) has the block deja must join. Appending a
// second key instead hides the reader's servers: YAML keeps the last one, and
// Hermes loaded deja alone (#4289).
func TestInstallHermesJoinsAnMCPBlockOnTheFirstLine(t *testing.T) {
	for name, cfg := range map[string]string{
		"first line":    "mcp_servers:\n  foo:\n    command: z\n",
		"no newline":    "mcp_servers:\n  foo:\n    command: z",
		"comment":       "model: x\nmcp_servers:  # mine\n  foo:\n    command: z\n",
		"trailing tabs": "model: x\nmcp_servers: \t\n  foo:\n    command: z\n",
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("DEJA_HERMES_HOME", "")
			path := filepath.Join(home, ".hermes", "config.yaml")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := installHermesMCP("/bin/deja", false); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(path)
			got := string(b)
			if n := strings.Count(got, "mcp_servers:"); n != 1 {
				t.Fatalf("%d mcp_servers keys, want one:\n%s", n, got)
			}
			for _, want := range []string{"  foo:", "  deja:"} {
				if !strings.Contains(got, want) {
					t.Fatalf("%q missing:\n%s", want, got)
				}
			}
			if _, err := installHermesMCP("/bin/deja", true); err != nil {
				t.Fatal(err)
			}
			b, _ = os.ReadFile(path)
			if string(b) != cfg {
				t.Fatalf("uninstall did not give the file back:\nwant %q\ngot  %q", cfg, b)
			}
		})
	}
}
