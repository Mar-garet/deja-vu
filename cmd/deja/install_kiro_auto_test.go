package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kiro-cli runs the hooks of the agent a chat starts in, and adds what an
// agentSpawn or userPromptSubmit hook prints to the model's context. Measured
// on 2.22.0: both outputs were in the request; preToolUse and postToolUse
// output was not (#4304).
func TestInstallKiroAutoWritesAnAgentWithRecallHooks(t *testing.T) {
	hermeticEnv(t)
	if _, err := captureRun(t, "install", "kiro-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(kiroTestDir(t), "agents", "deja.json")
	var agent struct {
		Name           string   `json:"name"`
		Tools          []string `json:"tools"`
		IncludeMcpJSON bool     `json:"includeMcpJson"`
		Hooks          map[string][]struct {
			Command   string `json:"command"`
			TimeoutMs int    `json:"timeout_ms"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readFile(t, path)), &agent); err != nil {
		t.Fatalf("agent file is not JSON: %v", err)
	}
	if agent.Name != "deja" || !agent.IncludeMcpJSON || len(agent.Tools) != 1 || agent.Tools[0] != "*" {
		t.Errorf("agent is not a full-tool agent that reads the global MCP file: %+v", agent)
	}
	spawn, prompt := agent.Hooks["agentSpawn"], agent.Hooks["userPromptSubmit"]
	if len(spawn) != 1 || !strings.HasSuffix(spawn[0].Command, " hook-context --plain") {
		t.Errorf("agentSpawn does not run hook-context --plain: %+v", spawn)
	}
	if len(prompt) != 1 || !strings.HasSuffix(prompt[0].Command, " hook-prompt --plain") {
		t.Errorf("userPromptSubmit does not run hook-prompt --plain: %+v", prompt)
	}
	if spawn[0].TimeoutMs <= 0 || spawn[0].TimeoutMs > 30000 {
		t.Errorf("timeout_ms = %d", spawn[0].TimeoutMs)
	}
	// The MCP half is the kiro target's.
	if !strings.Contains(readFile(t, kiroMCPSettingsPath()), `"deja"`) {
		t.Error("kiro-auto did not write the MCP entry")
	}

	var row autoWiring
	for _, a := range autoWirings() {
		if a.name == "kiro" {
			row = a
		}
	}
	if row.name == "" {
		t.Fatal("doctor has no auto-recall row for kiro")
	}
	if state, _ := autoWiringState(row); state != "wired" {
		t.Errorf("doctor reads the fresh install as %q, want wired", state)
	}

	if _, err := captureRun(t, "uninstall", "kiro-auto"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("uninstall left the agent behind: %v", err)
	}
}

// An agent the reader named deja is theirs: install leaves it alone and still
// writes the MCP server and steering, which `deja install --auto` used to give
// that machine before kiro-auto existed.
func TestInstallKiroAutoLeavesTheReadersOwnDejaAgent(t *testing.T) {
	hermeticEnv(t)
	path := filepath.Join(kiroTestDir(t), "agents", "deja.json")
	theirs := `{"name":"deja","description":"mine","tools":["read"]}` + "\n"
	writeFileMkdir(t, path, theirs)
	out, err := captureRun(t, "install", "kiro-auto", "--no-index")
	if err != nil {
		t.Fatalf("install refused the whole target over their agent: %v", err)
	}
	if got := readFile(t, path); got != theirs {
		t.Errorf("their agent changed:\n%s", got)
	}
	if !strings.Contains(readFile(t, kiroMCPSettingsPath()), `"deja"`) {
		t.Error("no MCP entry beside their agent")
	}
	if _, err := os.Stat(kiroSteeringPath()); err != nil {
		t.Errorf("no steering file beside their agent: %v", err)
	}
	if !strings.Contains(out, "did not write") {
		t.Errorf("install did not say why the agent was skipped:\n%s", out)
	}
	if _, err := captureRun(t, "uninstall", "kiro-auto"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != theirs {
		t.Errorf("uninstall took their agent:\n%s", got)
	}
}

func kiroTestDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(os.Getenv("HOME"), ".kiro")
}
