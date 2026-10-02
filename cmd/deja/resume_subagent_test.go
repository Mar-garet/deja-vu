package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/model"
)

// A Kimi or Qwen sub-agent log is a session in deja under
// DEJA_INCLUDE_SUBAGENTS=1, but neither client opens one on its own: resume
// printed `kimi --session agent-1-<parent>`, an id Kimi has never heard of
// (#4483). It says so, and names the session that spawned it.
func TestResumeRefusesAKimiOrQwenSubagent(t *testing.T) {
	tmp := t.TempDir()
	proj := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEJA_QWEN_ROOT", filepath.Join(tmp, "qwen"))
	qwenParent := qwenTranscriptIn(t, filepath.Join(tmp, "qwen"), proj, "q-1", true)
	qwenChild := filepath.Join(filepath.Dir(filepath.Dir(qwenParent)), "subagents", "q-1", "agent-a1.jsonl")
	for _, s := range []model.Session{
		{Harness: "qwen", ID: "agent-a1-q-1", Kind: "subagent", Parent: "q-1", Path: qwenChild},
		{Harness: "kimi", ID: "agent-1-session_t01", Kind: "subagent", Parent: "session_t01",
			Path: filepath.Join(tmp, "kimi", "sessions", "wd", "session_t01", "agents", "agent-1", "wire.jsonl")},
	} {
		dir, cmd, err := resumeCommand(s)
		if err == nil {
			t.Errorf("%s: printed %q for a sub-agent run", s.Harness, resumeCmdLine(dir, cmd))
			continue
		}
		for _, want := range []string{"sub-agent", "deja resume " + s.Parent} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: refusal %q does not say %q", s.Harness, err, want)
			}
		}
	}
	// Control: the parent still resumes.
	if _, cmd, err := resumeCommand(model.Session{Harness: "qwen", ID: "q-1", Path: qwenParent}); err != nil || cmd != "qwen -r q-1" {
		t.Errorf("the parent resumes as %q: %v", cmd, err)
	}
}
