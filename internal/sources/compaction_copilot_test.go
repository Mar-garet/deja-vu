package sources

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// VS Code Copilot Chat's PreCompact names the extension's transcript for the
// chat; the workspace comes from workspace.json beside the storage directory.
func TestReadCompactionCopilotReadsAVSCodeTranscript(t *testing.T) {
	ws := t.TempDir()
	storage := filepath.Join(t.TempDir(), "workspaceStorage", "abc")
	const id = "7c9e2d2c-0000-4000-8000-000000000000"
	path := filepath.Join(storage, "GitHub.copilot-chat", "transcripts", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	uri := "file://" + filepath.ToSlash(ws)
	if !strings.HasPrefix(filepath.ToSlash(ws), "/") {
		uri = "file:///" + filepath.ToSlash(ws)
	}
	if err := os.WriteFile(filepath.Join(storage, "workspace.json"), []byte(`{"folder":"`+uri+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"session.start","data":{"sessionId":"` + id + `","copilotVersion":"0.68.0"},"id":"e1","timestamp":"2026-10-07T10:00:00.000Z"}
{"type":"user.message","data":{"content":"the packer build fails to generate a manifest"},"id":"e2","timestamp":"2026-10-07T10:00:01.000Z"}
{"type":"assistant.message","data":{"content":"The manifest step needs the source name; I will pass it explicitly."},"id":"e3","timestamp":"2026-10-07T10:00:02.000Z"}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsCopilotTranscript(path) {
		t.Fatal("a VS Code transcript is not recognised")
	}
	got, err := ReadCompactionCopilot(path, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Harness != "copilot-chat" || len(got.Session.Messages) == 0 || filepath.Clean(got.Workspace) != filepath.Clean(ws) {
		t.Errorf("harness=%q messages=%d workspace=%q, want copilot-chat, some, %q", got.Harness, len(got.Session.Messages), got.Workspace, ws)
	}
	if _, err := ReadCompactionCopilot(path, "another-chat"); !errors.Is(err, ErrTranscriptIdentity) {
		t.Errorf("a transcript was read for a session it is not: %v", err)
	}
}

// Grok and Reasonix keep an events.jsonl as well; only Copilot CLI's, under
// session-state/<id>/, is read as Copilot's.
func TestOnlyCopilotsEventLogIsCopilots(t *testing.T) {
	for path, want := range map[string]bool{
		filepath.Join("h", ".copilot", "session-state", "abc", "events.jsonl"): true,
		filepath.Join("h", ".grok", "sessions", "abc", "events.jsonl"):         false,
		filepath.Join("h", "events.jsonl"):                                     false,
	} {
		if got := IsCopilotTranscript(path); got != want {
			t.Errorf("IsCopilotTranscript(%s) = %v, want %v", path, got, want)
		}
	}
}
