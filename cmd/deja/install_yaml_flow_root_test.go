package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// A YAML config whose whole document is `{}` got deja's block appended after
// it, which no parser reads: hermes fell back to its defaults and goose dropped
// the config. `mcp_servers: {}` got a second `mcp_servers:` key (#4555). Both
// are refused now, with the file left as it was.
func TestInstallRefusesYAMLItCouldOnlyBreak(t *testing.T) {
	for _, c := range []struct {
		target, seed string
		path         func() string
	}{
		{"hermes", "{}\n", func() string { return filepath.Join(sources.HermesHome(), "config.yaml") }},
		{"hermes", "mcp_servers: {}\n", func() string { return filepath.Join(sources.HermesHome(), "config.yaml") }},
		{"goose", "{}\n", func() string { return filepath.Join(gooseConfigDir(), "config.yaml") }},
		{"continue", "{}\n", continueConfigPath},
		{"aider", "{}\n", func() string { return filepath.Join(homeDir(), ".aider.conf.yml") }},
		{"deepseek", "{}\n", func() string { return filepath.Join(sources.DSHHome(), "cordis.patch.yml") }},
		// The same document opened on the marker's line.
		{"hermes", "--- {}\n", func() string { return filepath.Join(sources.HermesHome(), "config.yaml") }},
		{"goose", "--- {}  # empty\n", func() string { return filepath.Join(gooseConfigDir(), "config.yaml") }},
	} {
		t.Run(c.target+" "+strings.TrimSpace(c.seed), func(t *testing.T) {
			hermeticEnv(t)
			p := c.path()
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(c.seed), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := captureRun(t, "install", c.target, "--no-index", "--no-guidance")
			if err == nil {
				t.Errorf("install did not refuse")
			}
			if got, _ := os.ReadFile(p); string(got) != c.seed {
				t.Errorf("config changed:\n%s", got)
			}
		})
	}
}

// The control: a block mapping takes deja's block.
func TestInstallAppendsToABlockYAMLDocument(t *testing.T) {
	hermeticEnv(t)
	p := filepath.Join(sources.HermesHome(), "config.yaml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("model: hermes-3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "install", "hermes", "--no-index", "--no-guidance"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p); !strings.Contains(string(got), "mcp_servers:\n  deja:") {
		t.Errorf("deja's block missing:\n%s", got)
	}
}
