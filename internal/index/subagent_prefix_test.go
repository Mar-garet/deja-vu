package index

import (
	"path/filepath"
	"testing"
)

// A Kimi or Qwen sub-agent's id was its parent's with the agent's name after
// it, so the parent's whole id was a prefix of every child's, and a prefix
// opens the newest session it matches: `deja resume q-1` reached a child
// (#4483).
func TestAWholeIDOpensThatSessionNotANewerOneItPrefixes(t *testing.T) {
	tmp := hermeticIndexEnv(t)
	root := filepath.Join(tmp, "qwen")
	t.Setenv("DEJA_QWEN_ROOT", root)
	t.Setenv("DEJA_INCLUDE_SUBAGENTS", "1")
	project := filepath.Join(root, "projects", "-tmp-proj")
	write(t, filepath.Join(project, "chats", "q-1.jsonl"),
		`{"type":"user","sessionId":"q-1","timestamp":"2026-01-02T03:04:05Z","cwd":"/tmp/proj","message":{"role":"user","parts":[{"text":"parent question"}]}}`+"\n")
	write(t, filepath.Join(project, "subagents", "q-1", "agent-a1.jsonl"),
		`{"type":"user","sessionId":"q-1","timestamp":"2026-01-02T03:05:05Z","cwd":"/tmp/proj","message":{"role":"user","parts":[{"text":"trace the backoff jitter"}]}}`+"\n")
	dir := filepath.Join(tmp, "idx")
	if err := Ensure(dir, "", true, nil); err != nil {
		t.Fatal(err)
	}
	s, ok, err := FindByPrefix(dir, "q-1")
	if err != nil || !ok {
		t.Fatalf("q-1 resolved to nothing: %v", err)
	}
	if s.ID != "q-1" {
		t.Errorf("the whole id q-1 opened %s", s.ID)
	}
	if n := PrefixMatches(dir, "q-1"); n != 1 {
		t.Errorf("q-1 counts %d matches, want the one it names", n)
	}
	// Control: the child is still reached by its own id.
	if s, ok, _ := FindByPrefix(dir, "agent-a1"); !ok || s.Parent != "q-1" {
		t.Errorf("agent-a1 opened %+v", s)
	}
}
