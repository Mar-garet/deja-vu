package main

import (
	"os"
	"path/filepath"
	"testing"
)

// goose looks for user plugins under $GOOSE_PATH_ROOT/.agents/plugins when
// that is set (config/paths.rs get_dir), not under the home directory, so a
// hook written to ~/.agents/plugins never ran (#4569).
func TestGooseAutoHookFollowsGoosePathRoot(t *testing.T) {
	hermeticEnv(t)
	root := t.TempDir()
	t.Setenv("GOOSE_PATH_ROOT", root)
	exe := filepath.Join(t.TempDir(), "deja")
	if _, err := installTarget("goose-auto", exe, false); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".agents", "plugins", "deja", "hooks", "hooks.json")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("no hook where goose looks with GOOSE_PATH_ROOT set: %v", err)
	}
	home := filepath.Join(homeDir(), ".agents", "plugins", "deja")
	if isRealDir(home) {
		t.Errorf("hook written under the home directory, which goose does not read with GOOSE_PATH_ROOT set: %s", home)
	}

	// A relative root is ignored by goose, so the home directory stays.
	t.Setenv("GOOSE_PATH_ROOT", "rel/root")
	if got, want := gooseHookPath(), filepath.Join(homeDir(), ".agents", "plugins", "deja", "hooks", "hooks.json"); got != want {
		t.Errorf("gooseHookPath with a relative GOOSE_PATH_ROOT = %s, want %s", got, want)
	}
}
