package main

import (
	"github.com/vshulcz/deja-vu/internal/index"
	"github.com/vshulcz/deja-vu/internal/search"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// blame is the tool an agent calls before it edits a file, and on a file with
// no history it answered `[]`. An empty array reads as a tool that failed —
// every other mode says what happened in a sentence, and the CLI has all along.
// #2862 drew this distinction for a store with nothing in it; the other half is
// a store full of history where nothing touched this file (#3570).
func TestBlameSaysNoHistoryRatherThanAnEmptyArray(t *testing.T) {
	tmp := hermeticEnv(t)
	claude := filepath.Join(tmp, "claude")
	t.Setenv("DEJA_CLAUDE_ROOT", claude)
	at := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	writeClaudeFixture(t, filepath.Join(claude, "app", "one.jsonl"), "s1", []string{
		`{"type":"user","sessionId":"s1","timestamp":"` + at +
			`","message":{"role":"user","content":"the exporter drops every third retry"}}`,
	})
	if _, err := captureRunStderr(t, "index"); err != nil {
		t.Fatal(err)
	}

	body, hits, err := blameTextResult(index.DefaultDir(), search.BlameOptions{}, "/work/app/untouched.go", 10)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Fatalf("hits = %d, want none for a file nothing touched", hits)
	}
	if strings.TrimSpace(body) == "[]" {
		t.Fatal("blame answered a bare [] — an agent cannot tell that from a tool that failed")
	}
	note := body
	if !strings.Contains(note, "untouched.go") {
		t.Errorf("the note does not name the file: %q", note)
	}
	if !strings.Contains(note, "1 indexed session") {
		t.Errorf("the note does not say how much was searched, which is what separates it from an empty store: %q", note)
	}
}

// An empty store keeps its own sentence: "nobody touched this file" and "there
// is nothing indexed at all" are different answers.
func TestBlameStillSaysTheStoreIsEmptyWhenItIs(t *testing.T) {
	hermeticEnv(t)
	body, hits, err := blameTextResult(index.DefaultDir(), search.BlameOptions{}, "/work/app/untouched.go", 10)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Fatalf("hits = %d", hits)
	}
	if !strings.Contains(body, "no indexed history") {
		t.Fatalf("an empty store answered %q", body)
	}
}
