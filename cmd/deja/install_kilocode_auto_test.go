package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Kilo CLI is opencode vendored and loads `<config>/kilo/plugins/*.js` with
// opencode 1.x's loader. Measured on 7.8.3: the 1.x plugin's system transform
// put the digest in front of the model, and the 2.x default export was refused
// ("must default export an object with server()"), whichever opencode is on
// PATH (#4398).
func TestInstallKilocodeAutoWritesThePluginKiloLoads(t *testing.T) {
	hermeticEnv(t)
	// The opencode on PATH says nothing about Kilo's loader.
	t.Setenv("DEJA_OPENCODE_MAJOR", "2")
	if _, err := captureRun(t, "install", "kilocode-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(kilocodeCLIConfigDir(), "plugins", "deja.js")
	js := readFile(t, path)
	if strings.Contains(js, "export default") || !strings.Contains(js, `"experimental.chat.system.transform"`) {
		t.Fatalf("plugin is not the opencode 1.x shape Kilo loads:\n%.400s", js)
	}
	if !strings.Contains(js, "hook-context") || !strings.Contains(js, "hook-prompt") {
		t.Errorf("plugin does not run the digest and per-prompt hooks:\n%.400s", js)
	}
	if !strings.Contains(js, "regenerate with: deja install kilocode-auto") {
		t.Errorf("plugin header names another target:\n%.200s", js)
	}
	// The MCP half is the kilocode target's, CLI config included.
	if cfg := readFile(t, kilocodeCLIConfigPath()); !strings.Contains(cfg, `"deja"`) {
		t.Errorf("kilo config has no deja server:\n%s", cfg)
	}
	if _, err := os.Stat(filepath.Join(opencodeConfigHome(), "opencode", "plugins", "deja.js")); err == nil {
		t.Error("kilocode-auto wrote opencode's plugin too")
	}

	var row autoWiring
	for _, a := range autoWirings() {
		if a.name == "kilocode" {
			row = a
		}
	}
	if row.name == "" {
		t.Fatal("doctor has no auto-recall row for kilocode")
	}
	if state, _ := autoWiringState(row); state != "wired" {
		t.Errorf("doctor reads the fresh install as %q, want wired", state)
	}

	if _, err := captureRun(t, "uninstall", "kilocode-auto"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("uninstall left the plugin behind: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Errorf("uninstall left the plugins directory it made: %v", err)
	}
}
