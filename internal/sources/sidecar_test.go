package sources

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Cline rewrites taskHistory.json on every turn of any task. Only a change to
// the fields the reader takes from a task's own entry moves its fingerprint,
// or each turn would re-read every task the extension ever ran.
func TestClineVSCodeSidecarFollowsTheTasksOwnEntry(t *testing.T) {
	root := t.TempDir()
	transcript := filepath.Join(root, "tasks", "1767300000000", "api_conversation_history.json")
	history := filepath.Join(root, "state", "taskHistory.json")
	if err := os.MkdirAll(filepath.Dir(history), 0o755); err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	write := func(body string) (int64, int64) {
		t.Helper()
		at = at.Add(time.Second)
		if err := os.WriteFile(history, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(history, at, at); err != nil {
			t.Fatal(err)
		}
		return clineVSCodeSidecar(transcript)
	}
	_, base := write(`[{"id":"1767300000000","ts":1767300000000,"task":"fix the retry loop","tokensIn":1},{"id":"2","task":"other"}]`)
	if base == 0 {
		t.Fatal("the task's entry left no fingerprint")
	}
	if _, got := write(`[{"id":"1767300000000","ts":1767300000000,"task":"fix the retry loop","tokensIn":9},{"id":"2","task":"other, renamed"}]`); got != base {
		t.Error("a token count and another task's rename moved this task's fingerprint")
	}
	if _, got := write(`[{"id":"1767300000000","ts":1767300000000,"task":"cap the retry at three","tokensIn":9}]`); got == base {
		t.Error("a rename of this task left its fingerprint as it was")
	}
}
