package sources

import (
	"path/filepath"
	"testing"

	"github.com/vshulcz/deja-vu/internal/model"
)

// vocabHas reports whether any session carries the record.
func vocabHas(ss []model.Session, want string) bool {
	for _, s := range ss {
		for _, m := range s.Messages {
			if m.Role+": "+m.Text == want {
				return true
			}
		}
	}
	return false
}

// vocabRoles lists every record of the given role, for a failure message.
func vocabRoles(ss []model.Session, role string) []string {
	var out []string
	for _, s := range ss {
		for _, m := range s.Messages {
			if m.Role == role {
				out = append(out, m.Text)
			}
		}
	}
	return out
}

// A NotebookEdit in delete mode still carries new_source, and Claude Code
// discards it: no line of it reaches the notebook, so it is no wrote record
// (#4489).
func TestClaudeNotebookDeleteWritesNothing(t *testing.T) {
	const gone = "retries = compute_backoff_with_jitter(attempt, base=0.5)\n"
	for name, parse := range map[string]func(string) ([]model.Session, error){
		"claude":    ParseClaudeFile,
		"reference": func(p string) ([]model.Session, error) { return parseClaudeGenericFromOffset(p, 0) },
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("DEJA_CLAUDE_ROOT", root)
			p := vocabWrite(t, filepath.Join(root, "-tmp-proj", "c1.jsonl"),
				`{"type":"user","sessionId":"c1","timestamp":"2026-10-01T10:00:00Z","cwd":"/tmp/proj","message":{"role":"user","content":"drop the old cell"}}`,
				vocabJSON(map[string]any{"type": "assistant", "sessionId": "c1", "timestamp": "2026-10-01T10:00:01Z", "cwd": "/tmp/proj",
					"message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "n1", "name": "NotebookEdit",
						"input": map[string]any{"notebook_path": "/tmp/proj/retry.ipynb", "cell_id": "c1", "new_source": gone, "edit_mode": "delete"}}}}}),
			)
			ss := vocabParse(t, parse, p)
			if !vocabHas(ss, vocabFiles("/tmp/proj/retry.ipynb")) {
				t.Errorf("the deleted cell's notebook is not a files record: %q", vocabRoles(ss, RoleFiles))
			}
			if w := vocabRoles(ss, RoleWrote); len(w) > 0 {
				t.Errorf("a deleted cell left wrote records %q", w)
			}
		})
	}
}
