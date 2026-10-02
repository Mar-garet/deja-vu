package sources

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The trajectory log is OpenClaw's runtime artifact, not a conversation: read
// as one, it was a second session for every run (#4477).
func TestOpenClawTrajectoryIsNotATranscript(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DEJA_OPENCLAW_ROOT", root)
	dir := filepath.Join(root, "main", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"s1.jsonl", "s1.trajectory.jsonl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := OpenClawSessionFiles()
	if slices.Contains(got, filepath.Join(dir, "s1.trajectory.jsonl")) || len(got) != 1 {
		t.Errorf("transcripts = %v, want only s1.jsonl", got)
	}
}
