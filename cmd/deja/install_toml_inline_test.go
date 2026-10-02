package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// A TOML config that already defines the key inline got a `[mcp_servers.deja]`
// or `[[hooks]]` table appended that redefines it, and codex, grok and kimi
// then refused to load the whole file (#4554). Refused now, file untouched.
func TestInstallRefusesATOMLKeyWrittenInline(t *testing.T) {
	for _, c := range []struct {
		target, seed string
		path         func() string
	}{
		{"codex", "model = \"o3\"\nmcp_servers = { mine = { command = \"my-server\" } }\n",
			func() string { return filepath.Join(sources.CodexHome(), "config.toml") }},
		{"grok", "model = \"grok-4\"\nmcp_servers = { mine = { command = \"my-server\" } }\n",
			func() string { return filepath.Join(sources.GrokHome(), "config.toml") }},
		{"kimi-auto", "default_model = \"kimi-k2\"\nhooks = []\n",
			func() string { return filepath.Join(sources.KimiConfigDir(), "config.toml") }},
		// deja itself, inline under the table or as dotted keys: the header deja
		// appends redefines that one key, and TOML refuses it the same way.
		{"codex", "[mcp_servers]\ndeja = { command = \"/usr/local/bin/deja\", args = [\"mcp\"] }\n",
			func() string { return filepath.Join(sources.CodexHome(), "config.toml") }},
		{"codex", "mcp_servers.deja.command = \"/usr/local/bin/deja\"\n",
			func() string { return filepath.Join(sources.CodexHome(), "config.toml") }},
		{"grok", "[mcp_servers]\ndeja = { command = \"/usr/local/bin/deja\" }\n",
			func() string { return filepath.Join(sources.GrokHome(), "config.toml") }},
	} {
		t.Run(c.target, func(t *testing.T) {
			hermeticEnv(t)
			p := c.path()
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(c.seed), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := captureRun(t, "install", c.target, "--no-index", "--no-guidance")
			if err == nil || !strings.Contains(err.Error(), "inline") {
				t.Errorf("install did not refuse the inline key: %v", err)
			}
			if got, _ := os.ReadFile(p); string(got) != c.seed {
				t.Errorf("config changed:\n%s", got)
			}
		})
	}
}

// The control: dotted keys are tables TOML lets a header extend.
func TestInstallExtendsDottedTOMLKeys(t *testing.T) {
	hermeticEnv(t)
	p := filepath.Join(sources.CodexHome(), "config.toml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("mcp_servers.mine.command = \"my-server\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "install", "codex", "--no-index", "--no-guidance"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p); !strings.Contains(string(got), "[mcp_servers.deja]") {
		t.Errorf("deja's table missing:\n%s", got)
	}
}
