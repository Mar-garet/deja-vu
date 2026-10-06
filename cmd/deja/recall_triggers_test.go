package main

import (
	"strings"
	"testing"
)

// Every trigger deja shipped was a question — "didn't we fix this?", "what did
// we decide about X". A user who states that something of theirs exists ("так
// для этого у меня и есть deja watch") matched none of them, and neither did an
// agent about to answer that the thing is not there. Both are the same signal
// with the search skipped (#4004, #4005), so every surface that tells an agent
// when to recall has to name them, not just the one the sessions went through.
func TestEverySurfaceCarriesTheQuietTriggers(t *testing.T) {
	surfaces := map[string]string{
		"mcp instructions":  mcpInstructions(t.TempDir(), ""),
		"guidance block":    guidanceBody,
		"skill body":        skillBody,
		"skill frontmatter": skillFile(""),
		"cli skill":         cliSkillFile(),
		"hermes provider":   hermesMemoryPy("deja"),
	}
	for name, text := range surfaces {
		low := strings.ToLower(text)
		if !strings.Contains(low, "already have") && !strings.Contains(low, "already exists") {
			t.Errorf("%s never tells the agent to recall when the user states something of theirs exists", name)
		}
		if !strings.Contains(low, "on this machine does not exist") {
			t.Errorf("%s never tells the agent to recall before denying that something exists here", name)
		}
	}
}

// A change is not a question either. "Set the rates timeout to 2s" matched no
// trigger, so Opus on Claude Code called deja on 0 of 10 such tasks, each with
// a decision recorded in a past session, and on 10 of 10 with one sentence in
// the server instructions (#4760). Every text that sits in an agent's context and
// says when to recall carries it, short form included.
func TestEverySurfaceRecallsBeforeAChange(t *testing.T) {
	surfaces := map[string]string{
		"mcp instructions":  mcpInstructions(t.TempDir(), ""),
		"guidance block":    guidanceBody,
		"skill body":        skillBody,
		"skill frontmatter": skillFile(""),
		"cli skill":         cliSkillFile(),
		"hermes provider":   hermesMemoryPy("deja"),
		"kiro steering":     kiroSteering("deja"),
		"gemini extension":  string(repoFile(t, "GEMINI.md")),
		"opencode plugin":   string(repoFile(t, "extensions/opencode/index.js")),
		"dsh plugin":        string(repoFile(t, "extensions/dsh/index.js")),
		"openclaw plugin":   string(repoFile(t, "extensions/openclaw/index.mjs")),
	}
	for name, text := range surfaces {
		low := strings.ToLower(text)
		if !strings.Contains(low, "before you change code, config, a dependency or a schedule") &&
			!strings.Contains(low, "before changing code, config, a dependency or a schedule") {
			t.Errorf("%s never tells the agent to recall before an ordinary change", name)
		}
	}
}

// The repository ships its own copy of the CLI skill, and nothing compared it
// with the installer: the four deja-history copies are pinned, this one was not,
// so it kept the old trigger list while `deja install` wrote the new one.
func TestBundledCLISkillMatchesInstaller(t *testing.T) {
	got := string(repoFile(t, "skills/deja-search/SKILL.md"))
	if want := cliSkillRegistryFile(); got != want {
		t.Fatalf("skills/deja-search/SKILL.md has drifted from cliSkillRegistryFile:\n--- file ---\n%s\n--- installer ---\n%s", got, want)
	}
}
