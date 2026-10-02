package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vshulcz/deja-vu/internal/index"
)

// Grok Build 1.0.41 shows a SessionStart or UserPromptSubmit hook's
// systemMessage and drops its additionalContext — its hook guide says stdout
// is ignored for SessionStart and discarded for an allowing UserPromptSubmit,
// and the session's chat_history.jsonl carried no deja-recall. deja answered
// both with the full receipt ("1.7 KB of context") and logged them as memory
// that arrived (#4588). Grok runs every command hook with GROK_HOOK_EVENT set.
func TestGrokSessionStartAndPromptClaimNothingGrokDrops(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	claude := filepath.Join(tmp, "claude")
	if err := os.MkdirAll(filepath.Join(claude, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEJA_CLAUDE_ROOT", claude)
	t.Setenv("DEJA_CODEX_ROOT", filepath.Join(tmp, "codex"))
	t.Setenv("DEJA_OPENCODE_DB", filepath.Join(tmp, "none.db"))
	t.Setenv("DEJA_NOTES_FILE", filepath.Join(tmp, "notes.jsonl"))
	t.Setenv("GROK_HOOK_EVENT", "")
	at := time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339)
	for i, text := range []string{
		"the retry_loop in fetcher drops the last attempt",
		"unrelated work about deployments and dashboards",
	} {
		line := fmt.Sprintf(`{"type":"user","sessionId":"s%d","timestamp":%q,"cwd":%q,"message":{"role":"user","content":%q}}`,
			i, at, filepath.Join(tmp, "proj"), text)
		if err := os.WriteFile(filepath.Join(claude, "proj", fmt.Sprintf("s%d.jsonl", i)), []byte(line+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(tmp, "index.db")
	if err := index.Ensure(dir, "", true, nil); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)
	log := strings.TrimSuffix(dir, string(filepath.Separator)) + ".injections.jsonl"

	start := func(id string) string {
		withHookStdin(t, `{"hookEventName":"session_start","sessionId":"`+id+`","cwd":"`+cwd+`","source":"new","hook_event_name":"SessionStart","session_id":"`+id+`"}`)
		return captureStdout(t, func() { _ = runHookContext(dir, false) })
	}
	prompt := func(id string) string {
		var out bytes.Buffer
		in := strings.NewReader(`{"hookEventName":"user_prompt_submit","sessionId":"` + id + `","cwd":"` + cwd + `","prompt":"what did we change in the retry_loop in fetcher","hook_event_name":"UserPromptSubmit","session_id":"` + id + `"}`)
		if err := runHookPrompt(dir, in, &out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}

	t.Setenv("GROK_HOOK_EVENT", "session_start")
	if out := start("grok-1"); strings.Contains(out, "additionalContext") || strings.Contains(out, "of context") || strings.Contains(out, "recalled") {
		t.Errorf("grok's session start was answered with context it drops: %q", out)
	}
	t.Setenv("GROK_HOOK_EVENT", "user_prompt_submit")
	if out := prompt("grok-1"); strings.Contains(out, "retry_loop") || strings.Contains(out, "additionalContext") {
		t.Errorf("grok's prompt was answered with context it drops: %q", out)
	}
	logged, _ := os.ReadFile(log)
	if strings.Contains(string(logged), "grok-1") {
		t.Errorf("an answer grok dropped was logged as memory that arrived:\n%s", logged)
	}
	t.Setenv("GROK_HOOK_EVENT", "")

	// The control, after: the same payloads outside grok are answered and
	// logged, so the silence above is grok's and not the fixture's.
	if out := start("ctl-1"); !strings.Contains(out, "additionalContext") {
		t.Fatalf("session start delivered nothing outside grok, so this measures nothing: %q", out)
	}
	if out := prompt("ctl-1"); !strings.Contains(out, "retry_loop") {
		t.Fatalf("the prompt recalled nothing outside grok, so this measures nothing: %q", out)
	}

	after, _ := os.ReadFile(log)
	if !strings.Contains(string(after), "ctl-1") {
		t.Fatalf("the control was not logged, so this measures nothing:\n%s", after)
	}
}
