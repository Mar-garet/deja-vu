package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// On Windows without Git Bash CodeBuddy runs the plugin's shell scripts
// through PowerShell, which runs none of them; doctor has to say so and name
// the install that works (#4753).
func TestDoctorSaysTheCodeBuddyPluginNeedsGitBashOnWindows(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CODEBUDDY_CONFIG_DIR", cfg)
	t.Setenv("CODEBUDDY_CODE_GIT_BASH_PATH", "")
	writeSettings := func(s string) {
		if err := os.WriteFile(filepath.Join(cfg, "settings.json"), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitDir := t.TempDir()
	lookPath := map[string]string{}
	oldGOOS, oldLook := codeBuddyGOOS, codeBuddyLookPath
	t.Cleanup(func() { codeBuddyGOOS, codeBuddyLookPath = oldGOOS, oldLook })
	codeBuddyLookPath = func(name string) (string, error) {
		if p, ok := lookPath[name]; ok {
			return p, nil
		}
		return "", errors.New("not found")
	}

	codeBuddyGOOS = "windows"
	writeSettings(`{"enabledPlugins":{"deja-vu@some-market":true}}`)
	if note := codeBuddyPluginNote(); !strings.Contains(note, "deja install codebuddy-auto") {
		t.Fatalf("plugin on, no Git Bash: note %q", note)
	}

	// Controls: each one condition away from the broken machine says nothing.
	codeBuddyGOOS = "darwin"
	if note := codeBuddyPluginNote(); note != "" {
		t.Errorf("not Windows: note %q", note)
	}
	codeBuddyGOOS = "windows"
	writeSettings(`{"enabledPlugins":{"deja-vu@some-market":false,"other@x":true}}`)
	if note := codeBuddyPluginNote(); note != "" {
		t.Errorf("plugin off: note %q", note)
	}
	writeSettings(`{"enabledPlugins":{"deja-vu@some-market":true}}`)
	// Git for Windows: cmd\git.exe with bin\bash.exe beside it.
	if err := os.MkdirAll(filepath.Join(gitDir, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "bin", "bash.exe"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	lookPath["git"] = filepath.Join(gitDir, "cmd", "git.exe")
	if note := codeBuddyPluginNote(); note != "" {
		t.Errorf("Git Bash beside git: note %q", note)
	}
	delete(lookPath, "git")
	// WSL's bash on PATH is not Git Bash.
	lookPath["bash"] = `C:\Windows\System32\bash.exe`
	if note := codeBuddyPluginNote(); note == "" {
		t.Errorf("only WSL bash: no note")
	}
	t.Setenv("CODEBUDDY_CODE_GIT_BASH_PATH", filepath.Join(gitDir, "bin", "bash.exe"))
	if note := codeBuddyPluginNote(); note != "" {
		t.Errorf("override set: note %q", note)
	}
}
