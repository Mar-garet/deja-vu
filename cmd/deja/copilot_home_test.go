package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// Copilot CLI keeps its config and sessions under $COPILOT_HOME when it is
// set. deja read and wrote ~/.copilot regardless, so a user with the variable
// set had no sessions indexed and an MCP entry and skill Copilot never read
// (#4240).
func TestCopilotFollowsCopilotHome(t *testing.T) {
	tmp := hermeticEnv(t)
	t.Setenv("DEJA_COPILOT_ROOT", "")
	home := filepath.Join(tmp, "copilot-home")
	t.Setenv("COPILOT_HOME", home)

	if got, want := sources.CopilotRoot(), filepath.Join(home, "session-state"); got != want {
		t.Errorf("session root = %s, want %s", got, want)
	}
	if got, want := copilotMCPConfigPath(), filepath.Join(home, "mcp-config.json"); got != want {
		t.Errorf("MCP config = %s, want %s", got, want)
	}
	if got, want := guidancePath("copilot"), filepath.Join(home, "skills", "deja-history", "SKILL.md"); got != want {
		t.Errorf("skill = %s, want %s", got, want)
	}
	if got := existingTargetChecks()["copilot"]; got != home {
		t.Errorf("copilot is detected at %s, want %s", got, home)
	}

	r, err := installTarget("copilot", "/usr/local/bin/deja", false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Path != filepath.Join(home, "mcp-config.json") {
		t.Errorf("install wrote %s, want it under COPILOT_HOME", r.Path)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".copilot")); err == nil {
		t.Error("install wrote ~/.copilot with COPILOT_HOME set")
	}

	// The read override still wins for the session root alone.
	t.Setenv("DEJA_COPILOT_ROOT", filepath.Join(tmp, "elsewhere"))
	if got := sources.CopilotRoot(); got != filepath.Join(tmp, "elsewhere") {
		t.Errorf("DEJA_COPILOT_ROOT lost to COPILOT_HOME: %s", got)
	}
}
