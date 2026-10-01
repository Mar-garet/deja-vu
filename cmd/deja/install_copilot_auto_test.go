package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vshulcz/deja-vu/internal/index"
)

// Copilot CLI 1.0.79 runs command hooks from the `hooks` key of its user
// settings and puts what a sessionStart hook prints as `additionalContext` in
// front of the model (#4231).
type copilotHookFile struct {
	Hooks map[string][]struct {
		Type       string `json:"type"`
		Bash       string `json:"bash"`
		TimeoutSec int    `json:"timeoutSec"`
	} `json:"hooks"`
}

func copilotTestHome(t *testing.T) string {
	t.Helper()
	hermeticEnv(t)
	t.Setenv("COPILOT_HOME", "")
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".copilot"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestInstallCopilotAutoWiresSessionStartAndKeepsTheirs(t *testing.T) {
	home := copilotTestHome(t)
	path := filepath.Join(home, ".copilot", "settings.json")
	before := "{\n  \"model\": \"gpt-5.4\",\n  \"hooks\": {\n    \"sessionStart\": [\n      {\n        \"type\": \"command\",\n        \"bash\": \"/usr/bin/theirs start\",\n        \"timeoutSec\": 5\n      }\n    ],\n    \"userPromptSubmitted\": [\n      {\n        \"type\": \"command\",\n        \"bash\": \"/usr/bin/theirs prompt\"\n      }\n    ]\n  }\n}\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "install", "copilot-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	first := readFile(t, path)
	var cfg copilotHookFile
	if err := json.Unmarshal([]byte(first), &cfg); err != nil {
		t.Fatalf("settings.json is not JSON after install: %v\n%s", err, first)
	}
	start := cfg.Hooks["sessionStart"]
	if len(start) != 2 || start[0].Bash != "/usr/bin/theirs start" {
		t.Fatalf("their sessionStart hook did not survive, or deja's is missing:\n%s", first)
	}
	ours := start[1]
	if ours.Type != "command" || !strings.HasSuffix(ours.Bash, " hook-context --copilot") {
		t.Errorf("deja's entry is not a command hook running hook-context --copilot: %+v", ours)
	}
	// Copilot waits on a session-start hook before the first request, so a
	// slow deja must not hold the prompt for the default minute.
	if ours.TimeoutSec <= 0 || ours.TimeoutSec > 10 {
		t.Errorf("timeoutSec = %d, want a few seconds", ours.TimeoutSec)
	}
	if len(cfg.Hooks["userPromptSubmitted"]) != 1 || !strings.Contains(first, `"model": "gpt-5.4"`) {
		t.Errorf("install touched what was not deja's:\n%s", first)
	}

	if _, err := captureRun(t, "install", "copilot-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	if again := readFile(t, path); again != first {
		t.Errorf("a second install changed the file:\nfirst:\n%s\nsecond:\n%s", first, again)
	}

	if _, err := captureRun(t, "uninstall", "copilot-auto"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != before {
		t.Errorf("uninstall did not give settings.json back byte for byte:\nbefore:\n%s\nafter:\n%s", before, got)
	}
}

// A settings.json deja created holds nothing of the reader's, so uninstall
// takes the file away rather than leaving `{}` behind.
func TestUninstallCopilotAutoRemovesTheSettingsFileItCreated(t *testing.T) {
	home := copilotTestHome(t)
	path := filepath.Join(home, ".copilot", "settings.json")
	if _, err := captureRun(t, "install", "copilot-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, path), "hook-context --copilot") {
		t.Fatalf("install wrote no hook:\n%s", readFile(t, path))
	}
	if _, err := captureRun(t, "uninstall", "copilot-auto"); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err == nil {
		t.Errorf("uninstall left the settings.json deja created:\n%s", b)
	}
}

// Copilot moves a `hooks` key it finds in config.json over the one in
// settings.json on its next start, replacing it whole. An entry written to
// settings.json beside a config.json that still holds hooks would be gone after
// one launch, so deja writes where the reader's hooks are.
func TestCopilotAutoWritesBesideHooksStillInConfigJSON(t *testing.T) {
	home := copilotTestHome(t)
	config := filepath.Join(home, ".copilot", "config.json")
	settings := filepath.Join(home, ".copilot", "settings.json")
	before := `{"banner":"never","hooks":{"userPromptSubmitted":[{"type":"command","bash":"/usr/bin/theirs","timeoutSec":10}]}}` + "\n"
	if err := os.WriteFile(config, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "install", "copilot-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, config)
	if !strings.Contains(got, "hook-context --copilot") || !strings.Contains(got, "/usr/bin/theirs") {
		t.Fatalf("deja's hook did not go beside theirs in config.json:\n%s", got)
	}
	if b, err := os.ReadFile(settings); err == nil && strings.Contains(string(b), "hook-context") {
		t.Errorf("deja also wrote settings.json, which the move overwrites:\n%s", b)
	}
	if _, err := captureRun(t, "uninstall", "copilot-auto"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, config); got != before {
		t.Errorf("uninstall did not give config.json back byte for byte:\nbefore:\n%s\nafter:\n%s", before, got)
	}
}

// Copilot reads `additionalContext` at the top level and nothing nested: the
// Claude envelope runs, succeeds, and reaches no one. The payload names the
// session as `sessionId`, and that session stays out of its own digest.
func TestHookContextCopilotAnswersInCopilotsShape(t *testing.T) {
	tmp := hermeticEnv(t)
	claude := filepath.Join(tmp, "claude")
	t.Setenv("DEJA_CLAUDE_ROOT", claude)
	now := time.Now()
	seedClaudeAt(t, claude, "app", "older-session", "the ledger export drops the last row", "the writer missed a final flush", now.Add(-2*time.Hour))
	seedClaudeAt(t, claude, "app", "live-session", "the glimmerquest cache misses on every cold start", "warming it at boot from the last snapshot fixed it", now.Add(-time.Minute))
	dir := filepath.Join(tmp, "index.db")
	t.Setenv("DEJA_INDEX_DIR", dir)
	if err := index.Ensure(dir, "", true, io.Discard); err != nil {
		t.Fatal(err)
	}
	was := copilotHookOutput
	copilotHookOutput = true
	defer func() { copilotHookOutput = was }()

	withHookStdin(t, `{"sessionId":"live-session","timestamp":1790866280348,"cwd":"/tmp/app","source":"new","initialPrompt":"fix the export"}`)
	out := captureStdout(t, func() {
		if err := runHookContext(dir, false); err != nil {
			t.Error(err)
		}
	})
	var resp map[string]any
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &resp); err != nil {
		t.Fatalf("not one JSON object: %v\n%s", err, out)
	}
	if len(resp) != 1 {
		t.Errorf("want only additionalContext, got keys %v", resp)
	}
	ctx, _ := resp["additionalContext"].(string)
	if !strings.Contains(ctx, "ledger export") {
		t.Fatalf("the digest for the payload's project is missing:\n%s", out)
	}
	if strings.Contains(ctx, "glimmerquest") {
		t.Errorf("the session asking was served its own prompt:\n%s", out)
	}
}

func TestDoctorReportsCopilotAutoRecall(t *testing.T) {
	home := copilotTestHome(t)
	path := filepath.Join(home, ".copilot", "settings.json")
	row := func() string {
		var out bytes.Buffer
		doctorAutoRecall(&out)
		// The row and the lines printed under it, which are indented past the
		// name column.
		var lines []string
		on := false
		for _, line := range strings.Split(out.String(), "\n") {
			switch {
			case strings.HasPrefix(line, "  copilot "):
				on = true
			case !strings.HasPrefix(line, strings.Repeat(" ", 15)):
				on = false
			}
			if on {
				lines = append(lines, line)
			}
		}
		return strings.Join(lines, "\n")
	}
	if got := row(); !strings.Contains(got, "missing") {
		t.Fatalf("no row, or not missing before install:\n%s", got)
	}
	if _, err := captureRun(t, "install", "copilot-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	if got := row(); !strings.Contains(got, "wired") {
		t.Errorf("not wired after install:\n%s", got)
	}

	// The switch that turns every hook off leaves the entry looking installed.
	b := readFile(t, path)
	var root map[string]any
	if err := json.Unmarshal([]byte(b), &root); err != nil {
		t.Fatal(err)
	}
	root["disableAllHooks"] = true
	next, _ := json.Marshal(root)
	if err := os.WriteFile(path, next, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := row(); !strings.Contains(got, "stale") || !strings.Contains(got, "disableAllHooks") {
		t.Errorf("disableAllHooks is on and the row does not say so:\n%s", got)
	}

	// An entry naming a binary that is gone is a hook that exits 127.
	dead := `{"hooks":{"sessionStart":[{"type":"command","bash":"/nonexistent/bin/deja hook-context --copilot","timeoutSec":10}]}}`
	if err := os.WriteFile(path, []byte(dead), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := row(); !strings.Contains(got, "/nonexistent/bin/deja") || !strings.Contains(got, "copilot-auto") {
		t.Errorf("a dead binary in the hook is not reported:\n%s", got)
	}
}
