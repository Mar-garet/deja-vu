package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/index"
)

// An agent about to edit a file recalls by its path. When history holds a rule
// for the file's directory and nothing about the file, the answer has to say
// so: under "No session is about this" the agent read the rule as a stray lead
// and changed the file without the check it asked for (#4762).
func TestRecallByFileSaysTheSessionsAreAboutItsDirectory(t *testing.T) {
	hermeticEnv(t)
	proj := filepath.Join(os.Getenv("DEJA_CLAUDE_ROOT"), "-work")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, text := range []string{
		"Rule: every change in cmd/reconcile is checked with the dry-run replay of the August bank file; expect 4117 matched.",
		"pgx v5.7.0 broke COPY of the NUMERIC amount columns in the bulk import.",
		"Added a two-line README intro for ledgerd.",
	} {
		line := fmt.Sprintf(`{"type":"user","message":{"role":"user","content":%q},"timestamp":"2026-08-0%dT10:00:00Z","sessionId":"n%d","cwd":"/work"}`, text, i+1, i) + "\n"
		if err := os.WriteFile(filepath.Join(proj, fmt.Sprintf("n%d.jsonl", i)), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := index.DefaultDir()
	if err := index.Ensure(dir, "", false, nil); err != nil {
		t.Fatal(err)
	}
	text, err := recallText(dir, "cmd/reconcile/match.go amount tolerance", "", 5, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "4117 matched") {
		t.Fatalf("the directory rule was not served:\n%s", firstLines(text, 8))
	}
	if strings.Contains(text, "No session is about this") || !strings.Contains(text, "about its directory cmd/reconcile") {
		t.Errorf("the answer does not say the sessions are about the file's directory:\n%s", firstLines(text, 4))
	}
	if strings.Index(text, "4117 matched") > strings.Index(text, "NUMERIC") && strings.Contains(text, "NUMERIC") {
		t.Errorf("a session sharing one word outranks the directory rule:\n%s", text)
	}
}
