package main

import (
	"os"
	"path/filepath"
	"testing"
)

// goose registers every plugin it finds under `plugins:` in config.yaml, keyed
// by the plugin's directory, on its first start. Uninstall removed deja's
// plugin directory and left that key pointing at nothing (#4270).
func TestGooseAutoUninstallDropsGoosesPluginEntry(t *testing.T) {
	hermeticEnv(t)
	exe := filepath.Join(t.TempDir(), "deja")
	config := filepath.Join(gooseConfigDir(), "config.yaml")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	want := "GOOSE_PROVIDER: openai\n"
	if err := os.WriteFile(config, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installTarget("goose-auto", exe, false); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Dir(filepath.Dir(gooseHookPath()))
	// What goose 1.46 appends on its first start with the plugin there.
	b, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(homeDir(), ".agents", "plugins", "mine")
	started := string(b) + "plugins:\n  " + plugin + ":\n    enabled: true\n"
	if err := os.WriteFile(config, []byte(started), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installTarget("goose-auto", exe, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(config); string(got) != want {
		t.Errorf("config.yaml after uninstall = %q, want %q", got, want)
	}

	// Another plugin's entry stays, and so does the map it is in.
	if _, err := installTarget("goose-auto", exe, false); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(config)
	started = string(b) + "plugins:\n  '" + other + "':\n    enabled: false\n  '" + plugin + "':\n    enabled: true\n"
	if err := os.WriteFile(config, []byte(started), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installTarget("goose-auto", exe, true); err != nil {
		t.Fatal(err)
	}
	if got, want := readString(t, config), "GOOSE_PROVIDER: openai\nplugins:\n  '"+other+"':\n    enabled: false\n"; got != want {
		t.Errorf("config.yaml with another plugin = %q, want %q", got, want)
	}
}
