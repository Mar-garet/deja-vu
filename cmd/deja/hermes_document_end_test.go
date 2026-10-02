package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A `...` document end with a comment after it is still the end: install put
// its mcp_servers and plugins blocks after `... # end`, outside the document,
// and Hermes refused the file (#4348).
func TestHermesInstallStopsAtACommentedDocumentEnd(t *testing.T) {
	for _, end := range []string{"... # end", "...  ", "...\t# end"} {
		t.Run(end, func(t *testing.T) {
			hermeticEnv(t)
			t.Setenv("DEJA_HERMES_HOME", "")
			path := filepath.Join(os.Getenv("HOME"), ".hermes", "config.yaml")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			cfg := "model: x\n" + end + "\n"
			if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := installTarget("hermes-auto", "/bin/deja", false); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(path)
			if !strings.HasSuffix(string(b), "\n"+end+"\n") {
				t.Errorf("something was written after the document end:\n%s", b)
			}
			if !strings.Contains(string(b), "mcp_servers:") || !strings.Contains(string(b), "plugins:") {
				t.Errorf("install wrote neither block:\n%s", b)
			}
			if _, err := installTarget("hermes-auto", "/bin/deja", true); err != nil {
				t.Fatal(err)
			}
			if b, _ := os.ReadFile(path); string(b) != cfg {
				t.Errorf("uninstall gave back %q, want %q", b, cfg)
			}
		})
	}
}
