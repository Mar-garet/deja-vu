package sources

import (
	"os"
	"path/filepath"
	"strings"
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

// Only copilot_readFile names in its message the file it opened. getErrors
// over the workspace lists every file with a problem, getChangedFiles the
// repository root, createDirectory a directory and memory a file of the
// extension's own — none a file the session touched (#4492).
func TestCopilotChatMessageURIsOnlyForReadFile(t *testing.T) {
	uri := func(paths ...string) map[string]any {
		out := map[string]any{}
		for _, p := range paths {
			out["file://"+p] = map[string]any{"$mid": 1, "path": p, "scheme": "file"}
		}
		return out
	}
	part := func(id, tool, msg string, uris map[string]any) any {
		return map[string]any{"kind": "toolInvocationSerialized", "toolId": tool, "toolCallId": id, "isComplete": true,
			"pastTenseMessage": map[string]any{"value": msg, "uris": uris}}
	}
	p := vocabWrite(t, filepath.Join(t.TempDir(), "chatSessions", "5c0ffee0-0000-4000-8000-000000000001.jsonl"),
		vocabJSON(map[string]any{"kind": 0, "v": map[string]any{"version": 3, "sessionId": "5c0ffee0-0000-4000-8000-000000000001", "creationDate": 1790000000000, "requests": []any{}}}),
		vocabJSON(map[string]any{"kind": 2, "k": []any{"requests"}, "v": []any{map[string]any{
			"requestId": "r1", "timestamp": 1790000001000, "message": map[string]any{"text": "fix the retry loop"},
			"response": []any{
				part("c1", "copilot_getErrors", "Checked workspace, 3 problems found in [](file:///tmp/proj/a.go), [](file:///tmp/proj/b.go)", uri("/tmp/proj/a.go", "/tmp/proj/b.go")),
				part("c2", "copilot_getChangedFiles", "Read changed files in [](file:///tmp/proj)", uri("/tmp/proj")),
				part("c3", "copilot_createDirectory", "Created [](file:///tmp/proj/pkg)", uri("/tmp/proj/pkg")),
				part("c4", "copilot_memory", "Read memory [](file:///tmp/storage/memory.md)", uri("/tmp/storage/memory.md")),
				part("c5", "copilot_readFile", "Read [](file:///tmp/proj/retry.go)", uri("/tmp/proj/retry.go")),
				map[string]any{"value": "The retry loop never stops."},
			}}}}),
	)
	ss := vocabParse(t, ParseCopilotChatFile, p)
	if got := vocabRoles(ss, RoleFiles); len(got) != 1 || got[0] != "/tmp/proj/retry.go" {
		t.Errorf("files records = %q, want only the file readFile opened", got)
	}
}

// A patch names files relative to where it runs: Copilot CLI and Cline resolve
// `*** Update File: retry.go` against the session's directory, and the record
// has to say /tmp/proj/retry.go for blame and restore to find it (#4491,
// #4503).
const relPatch = "*** Begin Patch\n*** Update File: retry.go\n@@\n-" + oldLoop + "\n+" + newLoop + "\n*** End Patch"

var relPatchWants = []string{
	vocabFiles("/tmp/proj/retry.go"),
	vocabEdit("/tmp/proj/retry.go", oldLoop),
	vocabWrote("/tmp/proj/retry.go", newLoop),
}

func TestRelativePatchPathsResolveAgainstTheSession(t *testing.T) {
	for name, parse := range map[string]func(t *testing.T) []model.Session{
		"copilot": vocabCopilot(relPatch),
		"copilot from an offset": func(t *testing.T) []model.Session {
			ss := vocabCopilot(relPatch)(t)
			path := ss[0].Path
			// Past session.start, the way an incremental index reads on.
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			off := int64(strings.Index(string(raw), "\n") + 1)
			out, err := ParseCopilotFileFromOffset(path, off)
			if err != nil {
				t.Fatal(err)
			}
			return out
		},
		"cline-sdk": func(t *testing.T) []model.Session {
			dir := filepath.Join(t.TempDir(), "1790877871095_abcdf")
			vocabWrite(t, filepath.Join(dir, "1790877871095_abcdf.json"), `{"session_id":"1790877871095_abcdf","cwd":"/tmp/proj"}`)
			p := vocabWrite(t, filepath.Join(dir, "1790877871095_abcdf.messages.json"), vocabJSON(map[string]any{"messages": []any{
				map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "fix the retry loop"}}},
				map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "c1", "name": "apply_patch", "input": map[string]any{"input": relPatch}}}},
			}}))
			return vocabParse(t, ParseClineFile, p)
		},
	} {
		t.Run(name, func(t *testing.T) {
			ss := parse(t)
			for _, w := range relPatchWants {
				if !vocabHas(ss, w) {
					t.Errorf("missing %q; files %q, edits %q", w, vocabRoles(ss, RoleFiles), vocabRoles(ss, RoleEdit))
				}
			}
		})
	}
}
