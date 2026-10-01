package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/model"
)

// aider appends to its history and loads none of it back unless it is started
// with --restore-chat-history, so "run aider there and it continues the same
// history" sent the reader into an empty chat. The flag restores the whole
// file, every launch in it, and the hint has to say so (#4329).
func TestAiderResumeNamesRestoreChatHistory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	s := model.Session{Harness: "aider", ID: "aider-20909474c017-1", Path: filepath.Join(dir, ".aider.chat.history.md")}
	_, _, err := resumeCommand(s)
	if err == nil {
		t.Fatal("aider has no resume of one session; want the hint as an error")
	}
	msg := err.Error()
	for _, want := range []string{"aider --restore-chat-history", dir, "whole", "deja show " + s.ID} {
		if !strings.Contains(msg, want) {
			t.Errorf("hint %q does not say %q", msg, want)
		}
	}
	if strings.Contains(msg, "continues the same history") {
		t.Errorf("hint still says plain aider continues the history: %q", msg)
	}

	// A history moved off the default name is only found by naming it.
	s.Path = filepath.Join(dir, "notes", "chat.md")
	_, _, err = resumeCommand(s)
	if err == nil || !strings.Contains(err.Error(), "--chat-history-file "+s.Path) {
		t.Errorf("a moved history is not named: %v", err)
	}
}
