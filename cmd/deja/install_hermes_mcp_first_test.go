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
		"bare key last": "model: x\nmcp_servers:",
		"comment child": "mcp_servers:\n    # my servers\n  foo:\n    command: z\n",
		"bom":           "\ufeffmcp_servers:\n  foo:\n    command: z\n",
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
			if !strings.Contains(got, "\n  deja:\n") {
				t.Fatalf("deja is not an entry at the servers' indent:\n%s", got)
			}
			if strings.Contains(cfg, "foo:") && !strings.Contains(got, "\n  foo:") {
				t.Fatalf("the reader's server is gone:\n%s", got)
			}
			if !doctorHermesWired(path) {
				t.Fatalf("doctor does not see the entry install wrote:\n%s", got)
			}
			// A second install changes nothing.
			if _, err := installHermesMCP("/bin/deja", false); err != nil {
				t.Fatal(err)
			}
			if b, _ := os.ReadFile(path); string(b) != got {
				t.Fatalf("a second install changed the file:\nfirst  %q\nsecond %q", got, b)
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

// A key deja cannot join without guessing — written twice, or with the
// servers on its own line — is refused and the file left alone, here and for
// goose's extensions:.
func TestInstallRefusesATopLevelKeyItCannotJoin(t *testing.T) {
	for name, cfg := range map[string]string{
		"twice":  "mcp_servers:\n  foo:\n    command: z\nmodel: x\nmcp_servers:\n  bar:\n    command: y\n",
		"inline": "mcp_servers: {foo: {command: z}}\n",
		"quoted": "\"mcp_servers\":\n  foo:\n    command: z\n",
		"spaced": "mcp_servers :\n  foo:\n    command: z\n",
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
			if _, err := installHermesMCP("/bin/deja", false); err == nil {
				b, _ := os.ReadFile(path)
				t.Fatalf("install went ahead:\n%s", b)
			}
			if b, _ := os.ReadFile(path); string(b) != cfg {
				t.Fatalf("a refused install changed the file:\n%s", b)
			}
		})
	}
}

func TestInstallGooseJoinsACommentedExtensionsKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GOOSE_PATH_ROOT", "")
	path := filepath.Join(gooseConfigDir(), "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "extensions: # mine\n  foo:\n    cmd: z\n"
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installGoose("/bin/deja", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if n := strings.Count(string(b), "extensions:"); n != 1 {
		t.Fatalf("%d extensions keys, want one:\n%s", n, b)
	}
	if _, err := installGoose("/bin/deja", true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != cfg {
		t.Fatalf("uninstall did not give the file back:\nwant %q\ngot  %q", cfg, b)
	}
}
