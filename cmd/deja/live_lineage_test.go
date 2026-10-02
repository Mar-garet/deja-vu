package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vshulcz/deja-vu/internal/index"
)

// lineageStore holds, for Claude and Codex each, a parent session being worked
// in now, the sub-agent it spawned, and an older session that settled the
// question. Each spawned agent asks the question its parent handed it, so its
// own transcript is the best lexical match there is.
func lineageStore(t *testing.T) string {
	t.Helper()
	tmp := hermeticEnv(t)
	now := time.Now().UTC()
	at := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339Nano) }
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	claude := filepath.Join(tmp, "claude", "-w-p")
	write(filepath.Join(claude, "prior-c.jsonl"),
		`{"type":"user","sessionId":"prior-c","cwd":"/w/p","timestamp":"2026-08-04T09:00:00Z","message":{"role":"user","content":"the retry loop in fetch.go spins on HTTP 500"}}`+"\n"+
			`{"type":"assistant","sessionId":"prior-c","cwd":"/w/p","timestamp":"2026-08-04T09:02:00Z","message":{"role":"assistant","content":"Capped the retry loop in fetch.go at MaxAttempts five, so an HTTP 500 stops after five tries."}}`+"\n")
	write(filepath.Join(claude, "parent-c.jsonl"),
		fmt.Sprintf(`{"type":"user","sessionId":"parent-c","cwd":"/w/p","timestamp":%q,"message":{"role":"user","content":"look into the deploy script"}}`, at(-time.Minute))+"\n")
	write(filepath.Join(claude, "parent-c", "subagents", "agent-a6f2.jsonl"),
		fmt.Sprintf(`{"type":"user","isSidechain":true,"agentId":"a6f2","sessionId":"parent-c","cwd":"/w/p","timestamp":%q,"message":{"role":"user","content":"Find what we did about the retry loop in fetch.go spinning on HTTP 500."}}`, at(0))+"\n")

	codex := filepath.Join(tmp, "codex", "sessions", "2026", "10", "02")
	write(filepath.Join(codex, "rollout-2026-08-04T09-00-00-prior-x.jsonl"),
		`{"timestamp":"2026-08-04T09:00:00Z","type":"session_meta","payload":{"id":"prior-x","cwd":"/w/q"}}`+"\n"+
			`{"timestamp":"2026-08-04T09:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"the retry loop in fetch.go spins on HTTP 500"}]}}`+"\n"+
			`{"timestamp":"2026-08-04T09:00:02Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Capped the retry loop in fetch.go at MaxAttempts five."}]}}`+"\n")
	write(filepath.Join(codex, "rollout-2026-10-02T07-00-00-parent-x.jsonl"),
		fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"id":"parent-x","cwd":"/w/q","source":"cli"}}`, at(-time.Minute))+"\n"+
			fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"look into the deploy script"}]}}`, at(-time.Minute))+"\n")
	// codex 0.149.0 names the thread that spawned a sub-agent in the
	// sub-agent's own session_meta.
	write(filepath.Join(codex, "rollout-2026-10-02T07-01-00-child-x.jsonl"),
		fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"id":"child-x","cwd":"/w/q","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent-x","depth":1}}}}}`, at(0))+"\n"+
			fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Find what we did about the retry loop in fetch.go spinning on HTTP 500."}]}}`, at(0))+"\n")

	dir := index.DefaultDir()
	if err := index.Ensure(dir, "", true, nil); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A sub-agent's MCP recall listed its own transcript: hooks fire under the
// parent's id, so the child's is never stamped live, and the child is the one
// asking (#4547). Its parent being live is what says it is being written.
func TestASubAgentsRecallLeavesOutItsOwnTranscript(t *testing.T) {
	dir := lineageStore(t)
	q := json.RawMessage(`{"query":"retry loop fetch.go HTTP 500"}`)

	// Control: with nothing stamped both children are served.
	before, err := callMCPTool(dir, "recall", q)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if !strings.Contains(before, "a6f2") || !strings.Contains(before, "child-x") {
		t.Fatalf("the fixture never served the sub-agents, so this proves nothing:\n%s", before)
	}

	markSessionLive(dir, "parent-c")
	markSessionLive(dir, "parent-x")
	after, err := callMCPTool(dir, "recall", q)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	for _, own := range []string{"a6f2", "child-x"} {
		if strings.Contains(after, own) {
			t.Errorf("a sub-agent of a live session is on its own page (%s):\n%s", own, after)
		}
	}
	for _, prior := range []string{"prior-c", "prior-x"} {
		if !strings.Contains(after, prior) {
			t.Errorf("the session that settled it (%s) is not on the page:\n%s", prior, after)
		}
	}
}
