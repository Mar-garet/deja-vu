package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// With deja already wired under another name, install added a second server
// beside it in nine targets and said only "updated", so the client started
// deja twice (#4556, after #2269 and #2712 for the mcpServers writers). The
// map writers adopt the entry now; the rest say so.
func TestInstallSeesDejaUnderAnotherName(t *testing.T) {
	const old = "/usr/local/bin/deja"
	for _, c := range []struct {
		target, seed string
		path         func() string
		// the writer, for the ones that adopt: driven directly so the binary
		// is named deja, which the test binary is not
		write func(path, exe string, uninstall bool) (installResult, error)
	}{
		{"vscode", `{"servers": {"deja-vu": {"type": "stdio", "command": "` + old + `", "args": ["mcp"]}}}` + "\n",
			func() string { return filepath.Join(vsCodeDefaultUserDir(), "mcp.json") }, installVSCodeMCPAt},
		{"prime", `{"mcpServers": {"deja-vu": {"command": "` + old + `", "args": ["mcp"]}}}` + "\n", primeSettingsPath, installPrimeMCPAt},
		{"amp", `{"amp.mcpServers": {"deja-vu": {"command": "` + old + `", "args": ["mcp"]}}}` + "\n", sources.AmpSettingsFile, installAmpMCPAt},
		{"zcode", `{"mcp": {"servers": {"deja-vu": {"type": "stdio", "command": "` + old + `", "args": ["mcp"]}}}}` + "\n", zcodeConfigPath, zcodeServerAt},
		{"zed", `{"context_servers": {"deja-vu": {"command": "` + old + `", "args": ["mcp"]}}}` + "\n", sources.ZedSettingsPath, nil},
		{"grok", `{"mcp": {"servers": [{"id": "deja-vu", "label": "deja-vu", "transport": "stdio", "command": "` + old + `", "args": ["mcp"], "enabled": true}]}}` + "\n",
			func() string { return filepath.Join(sources.GrokHome(), "user-settings.json") }, nil},
		{"hermes", "mcp_servers:\n  deja-vu:\n    command: " + old + "\n    args: [mcp]\n",
			func() string { return filepath.Join(sources.HermesHome(), "config.yaml") }, nil},
		{"goose", "extensions:\n  deja-vu:\n    enabled: true\n    type: stdio\n    name: deja-vu\n    cmd: " + old + "\n    args: [mcp]\n",
			func() string { return filepath.Join(gooseConfigDir(), "config.yaml") }, nil},
		{"continue", "mcpServers:\n  - name: deja-vu\n    command: " + old + "\n    args: [mcp]\n", continueConfigPath, nil},
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
			if c.write != nil {
				t.Cleanup(func() { removingWiring = false })
				exe := filepath.Join(t.TempDir(), "deja")
				res, err := c.write(p, exe, false)
				if err != nil {
					t.Fatal(err)
				}
				if got, _ := os.ReadFile(p); strings.Contains(string(got), `"deja":`) {
					t.Errorf("a second entry beside deja-vu (%s):\n%s", res.Note, got)
				}
				// And uninstall takes back what it adopted.
				removingWiring = true
				if _, err := c.write(p, exe, true); err != nil {
					t.Fatal(err)
				}
				if got, _ := os.ReadFile(p); strings.Contains(string(got), "deja-vu") {
					t.Errorf("uninstall left the entry it adopted:\n%s", got)
				}
				return
			}
			out, err := captureRun(t, "install", c.target, "--no-index", "--no-guidance")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, `"deja-vu" also runs deja`) {
				t.Errorf("install did not say deja-vu also runs deja:\n%s", out)
			}
		})
	}
}
