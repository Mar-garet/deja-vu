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

// Antigravity 2.4.2: write_to_file {TargetFile, CodeContent} runs as a
// CODE_ACTION step that says "Created file" and carries no diff. A step that
// failed names no file and writes nothing (#4528).
func TestAntigravityWriteToFileWrote(t *testing.T) {
	q := func(s string) string { return vocabJSON(s) } // args are JSON-in-JSON on disk
	planner := func(file, content string) string {
		return vocabJSON(map[string]any{"step_index": 1, "source": "MODEL", "type": "PLANNER_RESPONSE", "status": "DONE", "created_at": "2026-09-30T10:00:05Z", "content": "",
			"tool_calls": []any{map[string]any{"name": "write_to_file", "args": map[string]any{"TargetFile": q(file), "CodeContent": q(content), "Overwrite": false}}}})
	}
	step := func(status, content string) string {
		return vocabJSON(map[string]any{"step_index": 2, "source": "MODEL", "type": "CODE_ACTION", "status": status, "created_at": "2026-09-30T10:00:09Z", "content": content})
	}
	failed := "func neverWritten() error { return nil }"
	p := vocabWrite(t, filepath.Join(t.TempDir(), "brain", "b0c1d2e3", ".system_generated", "logs", "transcript.jsonl"),
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-09-30T10:00:00Z","content":"<USER_REQUEST>\nfix the retry loop\n</USER_REQUEST>"}`,
		planner("/tmp/proj/jitter.go", jitter),
		step("DONE", "Created At: now\nCompleted At: now\n\nCreated file file:///tmp/proj/jitter.go"),
		planner("/tmp/proj/never.go", failed),
		step("ERROR", "Created At: now\nCompleted At: now\n\nEncountered error in step execution: error executing cascade"),
	)
	vocabCheck(t, vocabParse(t, ParseAntigravityFile, p), []string{
		vocabFiles("/tmp/proj/jitter.go"),
		vocabWrote("/tmp/proj/jitter.go", jitter),
	}, []string{
		vocabWrote("/tmp/proj/never.go", failed),
	})
}

// Continue's IDE agent edits with edit_existing_file {filepath, changes}: the
// new code with the untouched stretches elided. The elision lines are not
// written lines, and a canceled call wrote nothing (#4529).
func TestContinueEditExistingFileWrote(t *testing.T) {
	st := func(id, status string, args map[string]any) string {
		return vocabJSON(map[string]any{"toolCall": map[string]any{"id": id, "type": "function", "function": map[string]any{"name": "edit_existing_file", "arguments": vocabJSON(args)}},
			"status": status, "parsedArgs": args, "output": []any{}})
	}
	changes := "// ... existing code ...\n" + newLoop + "\n\t# ... rest of code ...\n<!-- … unchanged markup … -->"
	canceled := "func canceledChange() error { return nil }"
	body := `{"sessionId":"s1","title":"fix the retry loop","workspaceDirectory":"/tmp/proj","history":[` +
		`{"message":{"role":"user","content":"fix the retry loop"},"contextItems":[]},` +
		`{"message":{"role":"assistant","content":"","toolCalls":[]},"contextItems":[],"toolCallStates":[` +
		st("c1", "done", map[string]any{"filepath": "retry.go", "changes": changes}) + "," +
		st("c2", "canceled", map[string]any{"filepath": "other.go", "changes": canceled}) + `]}]}`
	p := vocabWrite(t, filepath.Join(t.TempDir(), "sessions", "s1.json"), body)
	vocabCheck(t, vocabParse(t, ParseContinueFile, p), []string{
		vocabFiles("retry.go\nother.go"),
		vocabWrote("retry.go", newLoop),
	}, []string{
		vocabWrote("retry.go", changes),
		vocabWrote("other.go", canceled),
	})
}

func TestWithoutElisions(t *testing.T) {
	for _, keep := range []string{"...", "\tpass  # ...", "x = ...", "foo(...args)", "return a..b"} {
		if got := withoutElisions(keep); got != keep {
			t.Errorf("withoutElisions(%q) = %q, want it kept", keep, got)
		}
	}
	for _, drop := range []string{"// ... existing code ...", "  # ... rest of code ...", "/* ... */ ", "<!-- … unchanged … -->", "{/* ... existing JSX ... */}", "... existing code ..."} {
		if got := withoutElisions(drop); got != "" {
			t.Errorf("withoutElisions(%q) = %q, want it dropped", drop, got)
		}
	}
}

// rooTask parses one Roo task under workspace /tmp/proj whose assistant turn
// makes the given calls.
func rooTask(t *testing.T, parse func(string) ([]model.Session, error), calls ...string) []model.Session {
	t.Helper()
	task := filepath.Join(t.TempDir(), "tasks", "1788845325718")
	vocabWrite(t, filepath.Join(task, "history_item.json"), `{"id":"1788845325718","ts":1788845325718,"task":"fix the retry loop","workspace":"/tmp/proj"}`)
	p := vocabWrite(t, filepath.Join(task, "api_conversation_history.json"),
		`[{"role":"user","content":[{"type":"text","text":"<task>\nfix the retry loop\n</task>"}]},{"role":"assistant","content":[`+strings.Join(calls, ",")+`]}]`)
	return vocabParse(t, parse, p)
}

func rooUse(name string, in any) string {
	return vocabJSON(map[string]any{"type": "tool_use", "id": "t-" + name, "name": name, "input": in})
}

// roo-cli 0.1.17 offers MiniMax models search_and_replace, an alias of edit,
// and keeps the alias in history with edit's arguments (#4531).
func TestRooSearchAndReplaceAlias(t *testing.T) {
	ss := rooTask(t, ParseRooTask, rooUse("search_and_replace", map[string]any{"file_path": "retry.go", "old_string": oldLoop, "new_string": newLoop}))
	vocabCheck(t, ss, []string{
		vocabFiles("/tmp/proj/retry.go"),
		vocabEdit("/tmp/proj/retry.go", oldLoop),
		vocabWrote("/tmp/proj/retry.go", newLoop),
	}, nil)
}
