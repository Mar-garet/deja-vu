package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vshulcz/deja-vu/internal/model"
)

// `cline --id` reopens the transcript from anywhere but runs its tools in the
// current directory, so the command has to run where the session did (#4318).
func TestResumeClineRunsInTheSessionDirectory(t *testing.T) {
	proj := t.TempDir()
	id := "1790000000000_abcde"
	dir := filepath.Join(t.TempDir(), "sessions", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	man, _ := json.Marshal(map[string]string{"sessionId": id, "cwd": proj})
	if err := os.WriteFile(filepath.Join(dir, id+".json"), man, 0o644); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(dir, id+".messages.json")
	if err := os.WriteFile(transcript, []byte(`{"messages":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, cmd, err := resumeCommand(model.Session{Harness: "cline", ID: id, Path: transcript})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "cline --id "+id {
		t.Errorf("cmd = %q", cmd)
	}
	if filepath.Clean(got) != filepath.Clean(proj) {
		t.Errorf("dir = %q, want the manifest's cwd %q", got, proj)
	}

	// A directory that is gone gets no cd that would fail before cline starts.
	if err := os.RemoveAll(proj); err != nil {
		t.Fatal(err)
	}
	if got, cmd, _ := resumeCommand(model.Session{Harness: "cline", ID: id, Path: transcript}); got != "" || cmd != "cline --id "+id {
		t.Errorf("got (%q, %q) for a directory that is gone", got, cmd)
	}
}
