package sources

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// Continue stores no time per turn, and a fork copies the whole history under a
// fresh dateCreated: 17 items one second apart put the last turn 16 s past the
// moment the file was written. No turn may be dated after the file's mtime,
// and the order stays (#4376).
func TestContinueTurnsNeverPassTheFileMtime(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DEJA_CONTINUE_ROOT", root)
	sid := "8f1c2a3e-0000-4000-8000-000000000000"
	created := time.Date(2026, 10, 1, 20, 33, 0, 0, time.UTC)
	var history []any
	for i := 0; i < 17; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		history = append(history, map[string]any{"message": map[string]any{"role": role, "content": fmt.Sprintf("turn %d", i)}})
	}
	path := writeContinueStore(t, root,
		map[string]any{"sessionId": sid, "history": history},
		[]any{map[string]any{"sessionId": sid, "dateCreated": created.Format(time.RFC3339Nano), "workspaceDirectory": "/w/api"}})
	written := created.Add(2 * time.Second)
	if err := os.Chtimes(path, written, written); err != nil {
		t.Fatal(err)
	}

	ss, err := ParseContinueFile(path)
	if err != nil || len(ss) != 1 {
		t.Fatalf("parsed %d sessions: %v", len(ss), err)
	}
	s := ss[0]
	if !s.Started.Equal(created) {
		t.Fatalf("started = %v, want dateCreated %v", s.Started, created)
	}
	if s.Updated.After(written) {
		t.Fatalf("updated = %v, after the file was written at %v", s.Updated, written)
	}
	for i := 1; i < len(s.Messages); i++ {
		if !s.Messages[i].Time.After(s.Messages[i-1].Time) {
			t.Fatalf("turn %d at %v is not after turn %d at %v", i, s.Messages[i].Time, i-1, s.Messages[i-1].Time)
		}
	}
}
