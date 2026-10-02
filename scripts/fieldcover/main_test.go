package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A store root given on the command line is read as a HOME, and a client's
// own location variable left in the shell does not answer for it (#4161).
func TestCoverReadsTheGivenRoot(t *testing.T) {
	put := func(p, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	session := func(id string) string {
		return `{"type":"user","sessionId":"` + id + `","timestamp":"2026-09-02T10:00:00Z","cwd":"/work/app","message":{"role":"user","content":"run the tests"}}
{"type":"assistant","sessionId":"` + id + `","timestamp":"2026-09-02T10:00:01Z","cwd":"/work/app","message":{"role":"assistant","content":[{"type":"text","text":"running them"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"make test"}}]}}
`
	}
	home := t.TempDir()
	put(filepath.Join(home, ".claude", "projects", "-work-app", "c1.jsonl"), session("c1"))
	empty := t.TempDir()
	// The shell's own store: without useHome clearing it, both roots read it.
	decoy := t.TempDir()
	put(filepath.Join(decoy, "-work-other", "d1.jsonl"), session("d1"))
	put(filepath.Join(decoy, "-work-other", "d2.jsonl"), session("d2"))
	t.Setenv("DEJA_CLAUDE_ROOT", decoy)
	t.Setenv("HOME", t.TempDir())

	claude := func(rows []row) row {
		for _, r := range rows {
			if r.Harness == "claude" {
				return r
			}
		}
		t.Fatal("no claude row")
		return row{}
	}
	restore := useHome(home)
	got := claude(cover(home, "claude"))
	restore()
	if got.Root != home || got.Sessions != 1 || got.Share["assistant"] != 1 || got.Share["command"] != 1 {
		t.Errorf("root with one session read as %+v", got)
	}
	restore = useHome(empty)
	got = claude(cover(empty, "claude"))
	restore()
	if got.Sessions != 0 {
		t.Errorf("empty root read %d sessions; the shell's DEJA_CLAUDE_ROOT answered for it", got.Sessions)
	}
	if os.Getenv("DEJA_CLAUDE_ROOT") != decoy {
		t.Errorf("environment not restored: DEJA_CLAUDE_ROOT=%q", os.Getenv("DEJA_CLAUDE_ROOT"))
	}
	// Control: the decoy is a readable store, so the zero above is the root's.
	if got := claude(cover("", "claude")); got.Sessions != 2 {
		t.Errorf("decoy store read %d sessions, want 2", got.Sessions)
	}
}
