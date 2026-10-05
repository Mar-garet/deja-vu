package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/model"
)

// CodeBuddy reopens a session with `codebuddy -r <id>`, found only under the
// folder of the directory it is run from; anywhere else it answers "No
// conversation found with session ID" (2.161.2). The transcript records that
// directory, so resume goes there, and refuses when it is gone (#4707).
//
// With -c as well: CodeBuddy hands SessionStart context to the model on a
// resume only when `continue` is set, and the -r id still picks the session,
// so `-r` alone reopened it without the recall deja had logged (#4718).
func TestResumeCodeBuddyGoesToItsDirectory(t *testing.T) {
	hermeticEnv(t)
	home, _ := os.UserHomeDir()
	work := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "01a10b10-f651-7360-aaab-18f648204bf8"
	path := filepath.Join(home, ".codebuddy", "projects", "app", id+".jsonl")
	line := `{"id":"u1","timestamp":1791187200000,"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}],"sessionId":"` + id + `","cwd":` + jsonString(work) + "}\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	// A sub-agent run is reached through the session that spawned it.
	if _, cmd, err := resumeCommand(model.Session{Harness: "codebuddy", ID: "agent-a1", Kind: "subagent", Parent: id, Path: path}); err == nil || !strings.Contains(err.Error(), "deja resume "+id) {
		t.Fatalf("sub-agent: cmd %q err %v", cmd, err)
	}
	s := model.Session{Harness: "codebuddy", ID: id, Path: path}
	dir, cmd, err := resumeCommand(s)
	if err != nil || dir != work || cmd != "codebuddy -c -r "+id {
		t.Fatalf("got dir %q cmd %q err %v", dir, cmd, err)
	}

	if err := os.RemoveAll(work); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resumeCommand(s); err == nil || !strings.Contains(err.Error(), work) || !strings.Contains(err.Error(), "deja show") {
		t.Fatalf("a gone directory: %v", err)
	}

	// WorkBuddy writes the same store; deja knows no command that reopens it.
	wb := filepath.Join(home, ".workbuddy", "projects", "app", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(wb), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wb, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, cmd, err := resumeCommand(model.Session{Harness: "codebuddy", ID: id, Path: wb}); err == nil || !strings.Contains(err.Error(), "WorkBuddy") {
		t.Fatalf("WorkBuddy session: cmd %q err %v", cmd, err)
	}
}
