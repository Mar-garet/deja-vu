package main

import (
	"path/filepath"
	"testing"
)

// goose skips a relative GOOSE_PATH_ROOT or XDG_CONFIG_HOME for its default,
// so deja writing under one wired a config goose never opens (#4285).
func TestGooseConfigDirSkipsARelativeRoot(t *testing.T) {
	hermeticEnv(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GOOSE_PATH_ROOT", filepath.Join("rel", "root"))
	if d := gooseConfigDirFor("linux"); !filepath.IsAbs(d) {
		t.Errorf("a relative GOOSE_PATH_ROOT gave %q", d)
	}
	t.Setenv("GOOSE_PATH_ROOT", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join("rel", "config"))
	if d, want := gooseConfigDirFor("linux"), filepath.Join(homeDir(), ".config", "goose"); d != want {
		t.Errorf("a relative XDG_CONFIG_HOME gave %q, want the default %q", d, want)
	}
	abs := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", abs)
	if d := gooseConfigDirFor("linux"); d != filepath.Join(abs, "goose") {
		t.Errorf("an absolute XDG_CONFIG_HOME gave %q", d)
	}
}

// On Windows goose's config is under %APPDATA%\Block\goose\config whatever
// XDG_CONFIG_HOME says: etcetera's Windows strategy never reads it, and Git
// Bash or scoop setups export it (#4286). GOOSE_PATH_ROOT still wins.
func TestGooseConfigDirOnWindowsIgnoresXDG(t *testing.T) {
	hermeticEnv(t)
	appData := filepath.Join(t.TempDir(), "AppData")
	t.Setenv("APPDATA", appData)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	t.Setenv("GOOSE_PATH_ROOT", "")
	if d, want := gooseConfigDirFor("windows"), filepath.Join(appData, "Block", "goose", "config"); d != want {
		t.Errorf("windows config dir = %q, want %q", d, want)
	}
	root := t.TempDir()
	t.Setenv("GOOSE_PATH_ROOT", root)
	if d := gooseConfigDirFor("windows"); d != filepath.Join(root, "config") {
		t.Errorf("GOOSE_PATH_ROOT on windows gave %q", d)
	}
}
