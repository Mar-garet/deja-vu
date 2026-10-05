package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/index"
)

// CodeBuddy reads skills from <config>/skills and user commands from
// <config>/commands (loadSkills, getHomeCommandsDir in 2.161.2), not from
// ~/.agents/skills, so an install left it without the skill and the /deja
// command Claude Code gets (#4708).
func TestInstallCodeBuddyWritesSkillAndCommand(t *testing.T) {
	home := filepath.Join(hermeticEnv(t), "home")
	dir := index.DefaultDir()
	skill := filepath.Join(home, ".codebuddy", "skills", "deja-history", "SKILL.md")
	command := filepath.Join(home, ".codebuddy", "commands", "deja.md")
	captureStdout(t, func() {
		if err := runInstall(dir, []string{"codebuddy-auto"}, false); err != nil {
			t.Fatal(err)
		}
	})
	b, err := os.ReadFile(skill)
	if err != nil || !strings.Contains(string(b), "name: deja-history") {
		t.Fatalf("skill: %v %.80q", err, b)
	}
	if b, err := os.ReadFile(command); err != nil || !strings.Contains(string(b), "$ARGUMENTS") {
		t.Fatalf("command: %v %.80q", err, b)
	}
	if got := guidanceStatus("codebuddy-auto"); got != "written" {
		t.Fatalf("guidance status %q", got)
	}

	cfg := filepath.Join(home, "cb-profile")
	t.Setenv("CODEBUDDY_CONFIG_DIR", cfg)
	if got := guidancePath("codebuddy"); got != filepath.Join(cfg, "skills", "deja-history", "SKILL.md") {
		t.Fatalf("CODEBUDDY_CONFIG_DIR skill path %q", got)
	}
	t.Setenv("CODEBUDDY_CONFIG_DIR", "")

	captureStdout(t, func() {
		if err := runInstall(dir, []string{"codebuddy-auto"}, true); err != nil {
			t.Fatal(err)
		}
	})
	for _, p := range []string{skill, command, filepath.Join(home, ".codebuddy", "skills"), filepath.Join(home, ".codebuddy", "commands")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s left behind after uninstall", p)
		}
	}
}
