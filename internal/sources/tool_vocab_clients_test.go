package sources

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/model"
)

// vocabCheck asserts that every want is among the records of ss and no
// unwanted one is.
func vocabCheck(t *testing.T, ss []model.Session, want, unwanted []string) {
	t.Helper()
	got := map[string]bool{}
	var all []string
	for _, s := range ss {
		for _, m := range s.Messages {
			got[m.Role+": "+m.Text] = true
			all = append(all, fmt.Sprintf("%s: %q", m.Role, m.Text))
		}
	}
	bad := false
	for _, w := range want {
		if !got[w] {
			bad = true
			t.Errorf("missing %q", w)
		}
	}
	for _, u := range unwanted {
		if got[u] {
			bad = true
			t.Errorf("unexpected %q", u)
		}
	}
	if bad {
		t.Logf("got:\n  %s", strings.Join(all, "\n  "))
	}
}

// Amp 0.0.1774959077, the last build with a local thread store: shell_command
// {command, workdir} and apply_patch {patchText} beside Bash and edit_file. A
// patch whose run was rejected changed nothing (#4527).
func TestAmpShellCommandAndApplyPatch(t *testing.T) {
	use := func(id, name string, in any) string {
		return vocabJSON(map[string]any{"type": "tool_use", "id": id, "name": name, "complete": true, "input": in})
	}
	res := func(id, status string) string {
		return vocabJSON(map[string]any{"type": "tool_result", "toolUseID": id, "run": map[string]any{"status": status, "result": map[string]any{"output": "ok", "exitCode": 0}}})
	}
	relPatch := "*** Begin Patch\n*** Update File: notes.go\n@@\n-func oldNotes() {}\n+func notes() error { return errRetry }\n*** End Patch"
	refused := "*** Begin Patch\n*** Update File: /tmp/proj/refused.go\n@@\n-func kept() {}\n+func refusedChange() error { return nil }\n*** End Patch"
	msgs := []string{
		`{"role":"user","content":[{"type":"text","text":"fix the retry loop"}],"meta":{"sentAt":1774950001000}}`,
		`{"role":"assistant","content":[` + use("t1", "shell_command", map[string]any{"command": "go test ./...", "workdir": "/tmp/proj"}) + `],"usage":{"timestamp":"2026-03-31T09:40:05Z"}}`,
		`{"role":"user","content":[` + res("t1", "done") + `]}`,
		`{"role":"assistant","content":[` + use("t2", "apply_patch", map[string]any{"patchText": patch}) + `,` + use("t3", "apply_patch", map[string]any{"patchText": relPatch}) + `]}`,
		`{"role":"user","content":[` + res("t2", "done") + `,` + res("t3", "done") + `]}`,
		`{"role":"assistant","content":[` + use("t4", "apply_patch", map[string]any{"patchText": refused}) + `]}`,
		`{"role":"user","content":[` + res("t4", "rejected-by-user") + `]}`,
	}
	body := `{"v":7,"id":"T-0f3c","created":1774950000000,"title":"Fix the retry loop","env":{"initial":{"trees":[{"uri":"file:///tmp/proj"}]}},"messages":[` + strings.Join(msgs, ",") + `]}`
	p := vocabWrite(t, filepath.Join(t.TempDir(), "T-0f3c.json"), body)
	vocabCheck(t, vocabParse(t, ParseAmpFile, p), append([]string{
		vocabCmd("$ go test ./..."),
		vocabFiles("/tmp/proj/notes.go"),
		vocabEdit("/tmp/proj/notes.go", "func oldNotes() {}"),
		vocabWrote("/tmp/proj/notes.go", "func notes() error { return errRetry }"),
	}, patchWants...), []string{
		vocabEdit("/tmp/proj/refused.go", "func kept() {}"),
		vocabWrote("/tmp/proj/refused.go", "func refusedChange() error { return nil }"),
	})
}
