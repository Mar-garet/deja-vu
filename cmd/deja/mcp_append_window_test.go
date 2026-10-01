package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vshulcz/deja-vu/internal/index"
)

// The hook starts an incremental pass and the agent's first recall lands in
// it: records.bin is already longer, the manifest not yet rewritten. The
// snapshot is complete, so recall answers from it instead of "deja is
// indexing" (#4266).
func TestRecallAnswersDuringAnIncrementalAppend(t *testing.T) {
	tmp := hermeticEnv(t)
	dir := filepath.Join(tmp, "index.db")
	store := filepath.Join(os.Getenv("DEJA_CLAUDE_ROOT"), "-proj")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := `{"type":"user","message":{"role":"user","content":"vorpelsnark retry budget"},"timestamp":"2026-07-11T10:00:00Z","sessionId":"s1","cwd":"/proj"}` + "\n"
	if err := os.WriteFile(filepath.Join(store, "s1.jsonl"), []byte(rec), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := index.Ensure(dir, "", true, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "warmup.sentinel"),
		[]byte(strconv.FormatInt(time.Now().UnixNano(), 10)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := buildingNowForAgent(dir); got != "" {
		t.Fatalf("the fixture is wrong — a quiet built index already declines: %q", got)
	}

	f, err := os.OpenFile(filepath.Join(dir, "records.bin"), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("records of the new session, manifest not committed")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if got := buildingNowForAgent(dir); got != "" {
		t.Errorf("an incremental append turned the agent away: %q", got)
	}
	if indexNeedsRebuild(dir) {
		t.Error("hooks read an incremental append as a store that needs rebuilding")
	}
	text, _, _, _, err := recallTextResult(dir, "vorpelsnark", "", 5, 0, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "vorpelsnark") {
		t.Errorf("recall answered nothing during the append:\n%s", text)
	}
}
