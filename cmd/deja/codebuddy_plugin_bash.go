package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// The CodeBuddy plugin's hooks and server are shell scripts. On Windows
// CodeBuddy runs them through Git Bash and falls back to PowerShell when it
// finds none, and PowerShell runs none of them. The manifest has no per-OS
// command, so there is no form of the plugin that works on a stock Windows;
// `deja install codebuddy-auto` does (#4728). doctor says which one this
// machine needs (#4753).

// Swapped by tests: the platform and PATH lookup doctor sees.
var (
	codeBuddyGOOS     = runtime.GOOS
	codeBuddyLookPath = exec.LookPath
)

// codeBuddyPluginEnabled reports whether CodeBuddy has a deja-vu plugin turned
// on, from any marketplace: `enabledPlugins` in its settings, the same shape
// Claude Code writes.
func codeBuddyPluginEnabled() bool {
	m, ok := jsonAt(readJSONConfig(codeBuddySettingsPath()), "enabledPlugins").(map[string]any)
	if !ok {
		return false
	}
	for k, v := range m {
		if on, _ := v.(bool); on && strings.HasPrefix(k, "deja-vu@") {
			return true
		}
	}
	return false
}

// codeBuddyFindsGitBash follows CodeBuddy's own search: its environment
// override, then bash.exe beside the git on PATH, then a bash on PATH that is
// not WSL's.
func codeBuddyFindsGitBash() bool {
	if p := os.Getenv("CODEBUDDY_CODE_GIT_BASH_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	if git, err := codeBuddyLookPath("git"); err == nil {
		root := filepath.Join(filepath.Dir(git), "..")
		for _, p := range []string{filepath.Join(root, "bin", "bash.exe"), filepath.Join(root, "usr", "bin", "bash.exe")} {
			if _, err := os.Stat(p); err == nil {
				return true
			}
		}
	}
	if bash, err := codeBuddyLookPath("bash"); err == nil {
		low := strings.ToLower(bash)
		return !strings.Contains(low, `\windows\system32\`) && !strings.Contains(low, `\windowsapps\`)
	}
	return false
}

// codeBuddyPluginNote is the line under the codebuddy row when the plugin is
// on and cannot run here, or "".
func codeBuddyPluginNote() string {
	if codeBuddyGOOS != "windows" || !codeBuddyPluginEnabled() || codeBuddyFindsGitBash() {
		return ""
	}
	return "the deja-vu plugin is on, but its hooks and server are shell scripts and CodeBuddy finds no Git Bash here — run `deja install codebuddy-auto`, which works without it"
}
