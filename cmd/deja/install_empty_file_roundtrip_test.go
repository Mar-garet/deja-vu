package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// An empty config the reader created (`touch ~/.codex/config.toml`) came back
// deleted from install plus uninstall, and an empty JSON one came back `{}`
// (#4563). A file that existed comes back with its bytes, none included.
func TestUninstallKeepsAnEmptyConfigTheReaderHad(t *testing.T) {
	for _, c := range []struct {
		target, seed string
		path         func() string
	}{
		{"codex", "", func() string { return filepath.Join(sources.CodexHome(), "config.toml") }},
		{"codex-auto", "", func() string { return filepath.Join(sources.CodexHome(), "config.toml") }},
		{"hermes", "", func() string { return filepath.Join(sources.HermesHome(), "config.yaml") }},
		{"kimi-auto", "", func() string { return filepath.Join(sources.KimiConfigDir(), "config.toml") }},
		{"goose-auto", "", func() string { return filepath.Join(gooseConfigDir(), "config.yaml") }},
		{"aider", "", func() string { return filepath.Join(homeDir(), ".aider.conf.yml") }},
		{"cursor", "", func() string { return filepath.Join(sources.CursorCLIHome(), "mcp.json") }},
		{"claude-code", "", sources.ClaudeJSONPath},
		{"claude-code", "\n\n", sources.ClaudeJSONPath},
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
			if _, err := captureRun(t, "install", c.target, "--no-index", "--no-guidance"); err != nil {
				t.Fatal(err)
			}
			if _, err := captureRun(t, "uninstall", c.target); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("uninstall deleted a config the reader had: %v", err)
			}
			if string(got) != c.seed {
				t.Errorf("uninstall gave back %q, want %q", got, c.seed)
			}
		})
	}
}

// The control: a config deja created goes with the uninstall.
func TestUninstallRemovesTheConfigItCreated(t *testing.T) {
	hermeticEnv(t)
	p := filepath.Join(sources.CodexHome(), "config.toml")
	if _, err := captureRun(t, "install", "codex", "--no-index", "--no-guidance"); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "uninstall", "codex"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("config deja created is still there: %v", err)
	}
}
