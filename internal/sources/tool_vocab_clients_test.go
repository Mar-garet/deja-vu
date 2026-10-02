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

// Roo's read_file still accepts files[{path, line_ranges}] and stores it as
// {files: [{path, lineRanges}], _legacyFormat: true} (#4531).
func TestRooLegacyReadFileList(t *testing.T) {
	ss := rooTask(t, ParseRooTask, rooUse("read_file", map[string]any{
		"files": []any{map[string]any{"path": "retry.go", "lineRanges": []any{map[string]any{"start": 1, "end": 20}}}, map[string]any{"path": "jitter.go"}}, "_legacyFormat": true}))
	vocabCheck(t, ss, []string{vocabFiles("/tmp/proj/retry.go\n/tmp/proj/jitter.go")}, nil)
}

// Crush v0.97.1: lsp_replace_symbol {symbol, file_path, replacement, action}
// writes replacement in place of the symbol, or before or after it; a delete
// writes nothing (#4533).
func TestCrushReplaceSymbolWrote(t *testing.T) {
	replaced := "func backoffJitter(attempt int) time.Duration { return time.Duration(rand.Int63n(int64(attempt+1))) }"
	deleted := "func legacyRetry() error { return errGiveUp }"
	call := func(id, input string) string {
		return crushInsert(t, "a_"+id, "s1", "assistant", 1784282401, []any{
			map[string]any{"type": "tool_call", "data": map[string]any{"id": "c_" + id, "name": "lsp_replace_symbol", "input": input, "finished": true}}})
	}
	sql := "insert into sessions values ('s1',null,'retry',2,0,0,0.0,1784282405,1784282400,null,null);\n" +
		call("1", vocabJSON(map[string]any{"symbol": "backoffJitter", "file_path": "/tmp/proj/jitter.go", "replacement": replaced, "action": "replace"})) +
		call("2", vocabJSON(map[string]any{"symbol": "legacyRetry", "file_path": "/tmp/proj/retry.go", "replacement": deleted, "action": "delete"}))
	ss, err := ParseCrushDB(crushStore(t, "proj", sql))
	if err != nil {
		t.Fatal(err)
	}
	vocabCheck(t, ss, []string{
		vocabFiles("/tmp/proj/jitter.go"),
		vocabWrote("/tmp/proj/jitter.go", replaced),
		vocabFiles("/tmp/proj/retry.go"),
	}, []string{
		vocabWrote("/tmp/proj/retry.go", deleted),
	})
}

// Kilo CLI 7.8.3 in kilo.db: background_process {action, command, workdir},
// notebook_edit {path, action, kind, source} and notebook_read {path}. Only
// start and monitor run a command, and only insert and replace write a cell;
// a notebook path may be relative to the session directory (#4534).
func TestKiloDBBackgroundProcessAndNotebooks(t *testing.T) {
	part := func(id, tool, status string, input any) string {
		data := fmt.Sprintf(`{"type":"tool","tool":%q,"callID":%q,"state":{"status":%q,"input":%s,"output":"ok","time":{"start":1790000002000}}}`, tool, id, status, vocabJSON(input))
		return fmt.Sprintf("insert into part values('%s','m1',%s);\n", id, sqlQuote(data))
	}
	cell := "retries = compute_backoff_with_jitter(attempt, base=0.5)"
	refused := "retries = refused_change(attempt)"
	db := vocabSQL(t, `create table session(id text primary key, parent_id text, directory text, title text, time_created integer, time_updated integer);
create table message(id text, session_id text, time_created integer, data text);
create table part(id text, message_id text, data text);
insert into session values('ses_1',null,'/tmp/proj','fix the retry loop',1790000000000,1790000100000);
insert into message values('m1','ses_1',1790000001000,'{"role":"assistant","time":{"created":1790000001000}}');
`+part("p1", "background_process", "completed", map[string]any{"action": "start", "command": "go run ./cmd/retryd", "workdir": "/tmp/proj"})+
		part("p2", "background_process", "completed", map[string]any{"action": "stop", "id": "bgp_01", "command": "go run ./cmd/stopped"})+
		part("p3", "notebook_edit", "completed", map[string]any{"path": "/tmp/proj/retry.ipynb", "action": "replace", "kind": "code", "index": 0, "source": cell})+
		part("p4", "notebook_read", "completed", map[string]any{"path": "notes/read.ipynb"})+
		part("p5", "notebook_edit", "error", map[string]any{"path": "/tmp/proj/retry.ipynb", "action": "insert", "kind": "code", "index": 1, "source": refused}))
	ss, err := ParseKiloDB(db)
	if err != nil {
		t.Fatal(err)
	}
	vocabCheck(t, ss, []string{
		vocabCmd("$ go run ./cmd/retryd"),
		vocabFiles("/tmp/proj/retry.ipynb"),
		vocabWrote("/tmp/proj/retry.ipynb", cell),
		vocabFiles("/tmp/proj/notes/read.ipynb"),
	}, []string{
		vocabCmd("$ go run ./cmd/stopped"),
		vocabWrote("/tmp/proj/retry.ipynb", refused),
	})
}

// Kilo Code (kilocode-legacy v5.16.2) task files: search_and_replace
// {path, operations[{search, replace}]}, fast_edit_file {target_file,
// instructions, code_edit}, write_file (the alias it keeps in history),
// delete_file {path} and generate_image {prompt, path} (#4535).
func TestKiloTaskToolVocabulary(t *testing.T) {
	second := "func backoffJitter(attempt int) time.Duration { return time.Duration(attempt) }"
	ss := rooTask(t, ParseKiloTask,
		rooUse("search_and_replace", map[string]any{"path": "retry.go", "operations": []any{
			map[string]any{"search": oldLoop, "replace": newLoop},
			map[string]any{"search": jitter, "replace": second}}}),
		rooUse("fast_edit_file", map[string]any{"target_file": "backoff.go", "instructions": "cap the retry loop",
			"code_edit": "// ... existing code ...\n" + newLoop + "\n// ... existing code ..."}),
		rooUse("write_file", map[string]any{"path": "jitter.go", "content": jitter}),
		rooUse("delete_file", map[string]any{"path": "old_retry.go"}),
		rooUse("generate_image", map[string]any{"prompt": "a retry diagram", "path": "retry.png"}),
	)
	vocabCheck(t, ss, []string{
		vocabFiles("/tmp/proj/retry.go\n/tmp/proj/backoff.go\n/tmp/proj/jitter.go\n/tmp/proj/old_retry.go\n/tmp/proj/retry.png"),
		vocabEdit("/tmp/proj/retry.go", oldLoop),
		vocabWrote("/tmp/proj/retry.go", newLoop),
		vocabEdit("/tmp/proj/retry.go", jitter),
		vocabWrote("/tmp/proj/retry.go", second),
		vocabWrote("/tmp/proj/backoff.go", newLoop),
		vocabWrote("/tmp/proj/jitter.go", jitter),
	}, nil)
}

// cwSaved parses one saved CodeWhale session, workspace /tmp/proj,
// whose assistant turn makes each call and gets result back (#4538).
func cwSaved(t *testing.T, result string, isErr bool, calls ...map[string]any) []model.Session {
	t.Helper()
	var uses, results []any
	for i, c := range calls {
		id := fmt.Sprintf("call_%d", i)
		uses = append(uses, map[string]any{"type": "tool_use", "id": id, "name": c["name"], "input": c["input"]})
		results = append(results, map[string]any{"type": "tool_result", "tool_use_id": id, "content": result, "is_error": isErr})
	}
	doc := map[string]any{
		"schema_version": 1,
		"metadata":       map[string]any{"id": "cw-1", "title": "fix the retry loop", "created_at": "2026-10-01T10:00:00Z", "updated_at": "2026-10-01T10:05:00Z", "workspace": "/tmp/proj"},
		"messages": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "fix the retry loop"}}},
			map[string]any{"role": "assistant", "content": uses},
			map[string]any{"role": "user", "content": results},
		},
	}
	return vocabParse(t, ParseCodeWhaleFile, vocabWrite(t, filepath.Join(t.TempDir(), "cw-1.json"), vocabJSON(doc)))
}

// CodeWhale 0.10.1 runs commands through terminal/run {command, session} and
// task_shell_start {command, cwd} beside bash (#4538).
func TestCodeWhaleTerminalAndTaskShellCommands(t *testing.T) {
	ss := cwSaved(t, "ok", false,
		map[string]any{"name": "terminal/run", "input": map[string]any{"command": "go test ./...", "session": "term-1"}},
		map[string]any{"name": "task_shell_start", "input": map[string]any{"command": "go test -race ./..."}},
	)
	vocabCheck(t, ss, []string{vocabCmd("$ go test ./..."), vocabCmd("$ go test -race ./...")}, nil)
}

// CodeWhale 0.10.1's apply_patch takes a unified diff under patch, retargeted
// by path when set, or whole files under replace[] {path, content}. Paths are
// relative to the workspace, and a call that came back as an error changed
// nothing (#4538).
func TestCodeWhaleApplyPatch(t *testing.T) {
	diff := "--- a/retry.go\n+++ b/retry.go\n@@ -1,3 +1,3 @@\n func retry() {\n-" + oldLoop + "\n+" + newLoop + "\n }\n"
	ss := cwSaved(t, "applied", false,
		map[string]any{"name": "apply_patch", "input": map[string]any{"path": "backoff.go", "patch": diff}},
		map[string]any{"name": "apply_patch", "input": map[string]any{"patch": diff}},
		map[string]any{"name": "apply_patch", "input": map[string]any{"replace": []any{map[string]any{"path": "/tmp/proj/jitter.go", "content": jitter}}}},
	)
	vocabCheck(t, ss, []string{
		vocabFiles("/tmp/proj/backoff.go"),
		vocabEdit("/tmp/proj/backoff.go", oldLoop),
		vocabWrote("/tmp/proj/backoff.go", newLoop),
		vocabFiles("/tmp/proj/retry.go"),
		vocabEdit("/tmp/proj/retry.go", oldLoop),
		vocabWrote("/tmp/proj/retry.go", newLoop),
		vocabFiles("/tmp/proj/jitter.go"),
		vocabWrote("/tmp/proj/jitter.go", jitter),
	}, nil)

	refused := cwSaved(t, "Error: patch did not apply", true,
		map[string]any{"name": "apply_patch", "input": map[string]any{"patch": diff}})
	vocabCheck(t, refused, nil, []string{
		vocabEdit("/tmp/proj/retry.go", oldLoop),
		vocabWrote("/tmp/proj/retry.go", newLoop),
	})
}

func TestUnifiedPatchReadsHunksByCount(t *testing.T) {
	// The removed SQL comment reads "--- old" with its diff marker, and is not
	// the next file's header; the second file has a bare @@.
	patch := "diff --git a/q.sql b/q.sql\n--- a/q.sql\t2026-06-26 10:00:00 +0000\n+++ b/q.sql\n@@ -1,2 +1,2 @@\n--- old\n+select retries from attempts;\n context\n" +
		"--- /dev/null\n+++ b/new.go\n@@\n+" + jitter + "\n"
	files, spans, wrote := unifiedPatch(patch, "", func(p string) string { return "/w/" + p })
	if strings.Join(files, ",") != "/w/q.sql,/w/new.go" {
		t.Errorf("files = %q", files)
	}
	if len(spans) != 1 || spans[0] != "/w/q.sql\n-- old" {
		t.Errorf("spans = %q", spans)
	}
	want := []string{WroteRecord("/w/q.sql", "select retries from attempts;"), WroteRecord("/w/new.go", jitter)}
	if strings.Join(wrote, "|") != strings.Join(want, "|") {
		t.Errorf("wrote = %q, want %q", wrote, want)
	}
}

// command-code 1.74.0: shell_command and monitor_command take {command,
// args[]}, powershell {command}, and read_file's paths may hold globs, which
// name no file the session touched (#4540).
func TestCommandCodeShellArgsAndReadGlobs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DEJA_COMMANDCODE_ROOT", root)
	n := 0
	line := func(role string, content []any) string {
		n++
		return vocabJSON(map[string]any{"type": "message", "id": fmt.Sprintf("m%d", n), "timestamp": fmt.Sprintf("2026-10-01T10:00:%02d.000Z", n), "message": map[string]any{"role": role, "content": content}})
	}
	use := func(name string, in any) string {
		return line("assistant", []any{map[string]any{"type": "tool_use", "id": "call_" + name, "name": name, "input": in}})
	}
	p := vocabWrite(t, filepath.Join(root, "tmp-proj", "cc-1.jsonl"),
		vocabJSON(map[string]any{"type": "session", "version": 3, "id": "cc-1", "timestamp": "2026-10-01T10:00:00.000Z", "cwd": "/tmp/proj"}),
		line("user", []any{map[string]any{"type": "text", "text": "fix the retry loop"}}),
		use("shell_command", map[string]any{"command": "go", "args": []any{"test", "./retry/..."}}),
		use("powershell", map[string]any{"command": "go vet ./..."}),
		use("monitor_command", map[string]any{"command": "go", "args": []any{"run", "./cmd/retryd", "--watch"}}),
		use("read_file", map[string]any{"paths": []any{"/tmp/proj/retry.go", "/tmp/proj/**/*_test.go"}}),
	)
	vocabCheck(t, vocabParse(t, ParseCommandCodeFile, p), []string{
		vocabCmd("$ go test ./retry/..."),
		vocabCmd("$ go vet ./..."),
		vocabCmd("$ go run ./cmd/retryd --watch"),
		vocabFiles("/tmp/proj/retry.go"),
	}, []string{
		vocabFiles("/tmp/proj/retry.go\n/tmp/proj/**/*_test.go"),
	})
}
