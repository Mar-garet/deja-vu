package sources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/model"
)

var failedClaudeTranscript = strings.Join([]string{
	`{"type":"user","sessionId":"c1","timestamp":"2026-10-01T10:00:00Z","cwd":"/tmp/proj","message":{"role":"user","content":"fix the retry loop"}}`,
	`{"type":"assistant","sessionId":"c1","timestamp":"2026-10-01T10:00:01Z","cwd":"/tmp/proj","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./..."}}]}}`,
	`{"type":"user","sessionId":"c1","timestamp":"2026-10-01T10:00:02Z","cwd":"/tmp/proj","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","is_error":true,"content":"Exit code 1\n./retry.go:12:5: undefined: backoffJitter\nFAIL"}]}}`,
}, "\n") + "\n"

var failedPiTranscript = strings.Join([]string{
	`{"type":"session","version":3,"id":"pi-s1","timestamp":"2026-09-20T10:00:00.000Z","cwd":"/tmp/proj"}`,
	`{"type":"message","id":"u1","timestamp":"2026-09-20T10:00:00.000Z","message":{"role":"user","content":[{"type":"text","text":"fix the retry loop"}]}}`,
	`{"type":"message","id":"a1","timestamp":"2026-09-20T10:00:01.000Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"c1","name":"bash","arguments":{"command":"go test ./..."}}]}}`,
	`{"type":"message","id":"r1","timestamp":"2026-09-20T10:00:02.000Z","message":{"role":"toolResult","toolCallId":"c1","toolName":"bash","content":[{"type":"text","text":"./retry.go:12:5: undefined: backoffJitter\n\nCommand exited with code 1"}],"details":{},"isError":true}}`,
}, "\n") + "\n"

// A command that failed carries its exit code in every harness that writes it
// down, each in its own place: a status line before or after the output, a
// field beside it, or a result filed apart from the call. Each row is one call
// to `go test ./...` and the failing result that harness writes for it
// (#4487, #4501, #4496, #4502, #4505, #4507, #4493).
func TestAFailedCommandKeepsItsExitCode(t *testing.T) {
	const want = "$ go test ./...  → exit 1"
	write := func(t *testing.T, p, body string) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	file := func(name, body string) func(*testing.T, string) string {
		return func(t *testing.T, dir string) string { return write(t, filepath.Join(dir, name), body) }
	}

	rows := []struct {
		name, kind string
		fixture    func(*testing.T, string) string
		parse      func(string) ([]model.Session, error)
	}{
		{name: "claude", kind: "claude", fixture: file("c1.jsonl", failedClaudeTranscript)},
		{name: "claude reference parser", fixture: file("c1.jsonl", failedClaudeTranscript), parse: func(p string) ([]model.Session, error) { return parseClaudeGenericFromOffset(p, 0) }},
		{name: "cherrystudio", kind: "cherrystudio", fixture: file("c1.jsonl", failedClaudeTranscript)},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			p := r.fixture(t, t.TempDir())
			var ss []model.Session
			if r.parse != nil {
				var err error
				if ss, err = r.parse(p); err != nil {
					t.Fatal(err)
				}
			} else {
				ss = parseKindForTest(t, r.kind, p)
			}
			assertCommand(t, ss, want)
		})
	}

}

func assertCommand(t *testing.T, ss []model.Session, want string) {
	t.Helper()
	var got []string
	for _, s := range ss {
		for _, m := range s.Messages {
			if m.Role == RoleCommand {
				if m.Text == want {
					return
				}
				got = append(got, m.Text)
			}
		}
	}
	t.Errorf("commands = %q, want %q among them", got, want)
}
