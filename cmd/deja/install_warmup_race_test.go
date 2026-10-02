package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vshulcz/deja-vu/internal/index"
)

// goose-auto writes its recall block during install, and that read asked for
// a detached build: install then waited on that build's lock, built nothing
// itself and printed "index: built (0 sessions, 0 messages)" over a full store.
// Install builds the index right after its targets, so nothing a target does
// on the way starts another (#4268).
func TestInstallStartsNoBuildOfItsOwnIndexOnTheSide(t *testing.T) {
	tmp := hermeticEnv(t)
	// hermeticEnv turns warmups off; this test is about one, stubbed below.
	t.Setenv("DEJA_WARMUP_SENTINEL", "")
	claude := filepath.Join(tmp, "claude")
	t.Setenv("DEJA_CLAUDE_ROOT", claude)
	at := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	writeClaudeFixture(t, filepath.Join(claude, "alpha", "one.jsonl"), "c1", []string{
		`{"type":"user","sessionId":"c1","timestamp":"` + at +
			`","message":{"role":"user","content":"why does the retry loop spin"}}`,
	})
	spawned := 0
	saved := spawnWarmup
	spawnWarmup = func(_, _ string) error { spawned++; return nil }
	t.Cleanup(func() { spawnWarmup = saved })

	_, said := captureBoth(t, "install", "goose-auto")
	if spawned != 0 {
		t.Errorf("install started %d background builds beside its own", spawned)
	}
	if !strings.Contains(said, "index: built (1 session") {
		t.Errorf("the build line does not count the store:\n%s", said)
	}
}

// A build another process finished while install waited on its lock left
// this process's own count at zero; the line reads the store (#4268).
func TestInstallBuiltLineCountsAStoreAnotherDejaBuilt(t *testing.T) {
	tmp := hermeticEnv(t)
	claude := filepath.Join(tmp, "claude")
	t.Setenv("DEJA_CLAUDE_ROOT", claude)
	writeClaudeFixture(t, filepath.Join(claude, "alpha", "one.jsonl"), "c1", []string{
		`{"type":"user","sessionId":"c1","timestamp":"2026-10-01T09:00:00Z","message":{"role":"user","content":"why does the retry loop spin"}}`,
	})
	if _, err := captureRunStderr(t, "index"); err != nil {
		t.Fatal(err)
	}
	saved := index.LastBuild
	index.LastBuild = index.BuildSummary{}
	t.Cleanup(func() { index.LastBuild = saved })
	if got := installBuiltLine(index.DefaultDir()); !strings.Contains(got, "(1 session") {
		t.Errorf("line = %q, want the store's one session", got)
	}
}
