package sources

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The rows Hermes 0.17 wrote for a session that ran terminal, write_file and
// patch: the calls ride on assistant rows with no content, the results on
// `tool` rows. None of it reached the index (#4242).
func TestHermesToolCallsBecomeWorkRecords(t *testing.T) {
	db := writeHermesDB(t, filepath.Join(t.TempDir(), "p"), `
		INSERT INTO messages (session_id,role,content,tool_call_id,tool_calls,tool_name,timestamp) VALUES
		 ('s','user','fix the retry loop',NULL,NULL,NULL,1785000000.0),
		 ('s','assistant','',NULL,'[{"id":"c1","type":"function","function":{"name":"terminal","arguments":"{\"command\": \"go test ./...\"}"}}]',NULL,1785000001.0),
		 ('s','tool','{"output": "notes.txt", "exit_code": 0, "error": null}','c1',NULL,'terminal',1785000002.0),
		 ('s','assistant',NULL,NULL,'[{"id":"c2","type":"function","function":{"name":"write_file","arguments":"{\"path\": \"/tmp/proj/retry.py\", \"content\": \"def retry_with_backoff(attempts):\\n    raise NotImplementedError\\n\"}"}}]',NULL,1785000003.0),
		 ('s','tool','{"bytes_written": 26}','c2',NULL,'write_file',1785000004.0),
		 ('s','assistant','',NULL,'[{"id":"c3","type":"function","function":{"name":"patch","arguments":"{\"mode\": \"replace\", \"path\": \"/tmp/proj/retry.py\", \"old_string\": \"raise NotImplementedError\", \"new_string\": \"return attempts * backoff_seconds\"}"}}]',NULL,1785000005.0),
		 ('s','assistant','',NULL,'[{"id":"c4","type":"function","function":{"name":"terminal","arguments":"{\"command\": \"make build\"}"}}]',NULL,1785000006.0),
		 ('s','tool','{"output": "SyntaxError: bad", "exit_code": 1, "error": null}','c4',NULL,'terminal',1785000007.0),
		 ('s','assistant','',NULL,'[{"id":"c5","type":"function","function":{"name":"patch","arguments":"{\"mode\": \"patch\", \"patch\": \"*** Begin Patch\\n*** Update File: /tmp/proj/a.py\\n@@\\n-old_value = compute_the_old_way()\\n+new_value = compute_the_new_way()\\n*** End Patch\"}"}}]',NULL,1785000008.0),
		 ('s','assistant','Done.',NULL,NULL,NULL,1785000009.0);`)
	ss, err := ParseHermesDB(db)
	if err != nil || len(ss) != 1 {
		t.Fatalf("parse: %v, %d sessions", err, len(ss))
	}
	by := map[string][]string{}
	for _, m := range ss[0].Messages {
		by[m.Role] = append(by[m.Role], m.Text)
	}
	want := map[string][]string{
		RoleCommand:    {"$ go test ./...", "$ make build  → exit 1"},
		RoleFiles:      {"/tmp/proj/retry.py", "/tmp/proj/retry.py", "/tmp/proj/a.py"},
		RoleEdit:       {"/tmp/proj/retry.py\nraise NotImplementedError", "/tmp/proj/a.py\nold_value = compute_the_old_way()"},
		RoleToolOutput: {"notes.txt", "SyntaxError: bad"},
	}
	for role, w := range want {
		if got := strings.Join(by[role], " | "); got != strings.Join(w, " | ") {
			t.Errorf("%s = %q, want %q", role, by[role], w)
		}
	}
	// write_file's content, patch's new_string, and the patch's added line.
	if len(by[RoleWrote]) != 3 {
		t.Errorf("wrote records = %d, want 3: %q", len(by[RoleWrote]), by[RoleWrote])
	}
	if got := strings.Join(by["assistant"], "|"); got != "Done." {
		t.Errorf("assistant prose = %q, want the one real line", got)
	}
}

// hermes017Schema is the messages table as Hermes 0.17 creates it
// (hermes_state.py), with the active/compacted pair rewind and compaction set.
const hermes017Schema = `CREATE TABLE messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT NOT NULL,
	role TEXT NOT NULL,
	content TEXT,
	tool_call_id TEXT,
	tool_calls TEXT,
	tool_name TEXT,
	timestamp REAL NOT NULL,
	active INTEGER NOT NULL DEFAULT 1,
	compacted INTEGER NOT NULL DEFAULT 0);`

func writeHermesStore(t *testing.T, schema, rows string) string {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is not installed")
	}
	dir := filepath.Join(t.TempDir(), "p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "state.db")
	cmd := exec.Command("sqlite3", db)
	cmd.Stdin = strings.NewReader(schema + rows)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v\n%s", err, out)
	}
	return db
}

func hermesRoles(t *testing.T, db string) map[string][]string {
	t.Helper()
	ss, err := ParseHermesDB(db)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	by := map[string][]string{}
	for _, s := range ss {
		for _, m := range s.Messages {
			by[m.Role] = append(by[m.Role], m.Text)
		}
	}
	return by
}

// exit_code -1 is what terminal returns for a command it never ran — denied,
// blocked, waiting on approval, invalid, failed to start
// (tools/terminal_tool.py). Recorded as a command, it read as one that ran.
func TestHermesCommandThatNeverRanIsNoCommand(t *testing.T) {
	db := writeHermesStore(t, hermes017Schema, `
		INSERT INTO messages (session_id,role,content,tool_call_id,tool_calls,tool_name,timestamp) VALUES
		 ('s','user','clean the build dir',NULL,NULL,NULL,1785000000.0),
		 ('s','assistant','',NULL,'[{"id":"c1","type":"function","function":{"name":"terminal","arguments":"{\"command\": \"rm -rf build && make build\"}"}}]',NULL,1785000001.0),
		 ('s','tool','{"output": "", "exit_code": -1, "error": "Command denied: recursive delete.", "status": "blocked"}','c1',NULL,'terminal',1785000002.0),
		 ('s','assistant','',NULL,'[{"id":"c2","type":"function","function":{"name":"terminal","arguments":"{\"command\": \"make build\"}"}}]',NULL,1785000003.0),
		 ('s','tool','{"output": "ok", "exit_code": 0, "error": null}','c2',NULL,'terminal',1785000004.0);`)
	by := hermesRoles(t, db)
	if got := strings.Join(by[RoleCommand], "|"); got != "$ make build" {
		t.Errorf("commands = %q, want only the one that ran", by[RoleCommand])
	}
	if !strings.Contains(strings.Join(by[RoleToolOutput], "|"), "Command denied") {
		t.Errorf("tool output lost why it never ran: %q", by[RoleToolOutput])
	}
}

// Multimodal content is stored as "\x00json:" + the parts (hermes_state.py
// _encode_content). The text parts are the message; the image is base64.
func TestHermesMultimodalKeepsTextParts(t *testing.T) {
	parts := `[{"type": "text", "text": "why is this chart flat"}, {"type": "image_url", "image_url": {"url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB"}}]`
	db := writeHermesStore(t, hermes017Schema, `
		INSERT INTO messages (session_id,role,content,tool_call_id,tool_calls,tool_name,timestamp) VALUES
		 ('s','user',char(0)||'json:'||'`+parts+`',NULL,NULL,NULL,1785000000.0),
		 ('s','tool',char(0)||'json:'||'`+strings.ReplaceAll(parts, "why is this chart flat", "screenshot of the dashboard")+`','c1',NULL,'computer_use',1785000001.0);`)
	by := hermesRoles(t, db)
	if got := strings.Join(by["user"], "|"); got != "why is this chart flat" {
		t.Errorf("user = %q", got)
	}
	if got := strings.Join(by[RoleToolOutput], "|"); got != "screenshot of the dashboard" {
		t.Errorf("tool output = %q", got)
	}
}

// A result is JSON. What it says lives in output, content, diff or error;
// the rest is bookkeeping, and keys starting with _ are hints to the model.
// Compressor stubs stand in for output that was cleared and say nothing.
func TestHermesToolResultIsItsText(t *testing.T) {
	db := writeHermesStore(t, hermes017Schema, `
		INSERT INTO messages (session_id,role,content,tool_call_id,tool_calls,tool_name,timestamp) VALUES
		 ('s','user','read the config',NULL,NULL,NULL,1785000000.0),
		 ('s','tool','{"content": "1|retries: 3\n2|Always retry on 503", "total_lines": 2, "_hint": "use offset to read more"}','c1',NULL,'read_file',1785000001.0),
		 ('s','tool','{"success": true, "diff": "-retries: 3\n+retries: 5", "files_modified": ["/x"]}','c2',NULL,'patch',1785000002.0),
		 ('s','tool','{"bytes_written": 50, "dirs_created": true}','c3',NULL,'write_file',1785000003.0),
		 ('s','tool','{"success": false, "error": "file not found: /nope"}','c4',NULL,'read_file',1785000004.0),
		 ('s','tool','[Old tool output cleared to save context space]','c5',NULL,'terminal',1785000005.0),
		 ('s','tool','[terminal] ran `+"`make`"+` -> exit 0, 40 lines output','c6',NULL,'terminal',1785000006.0),
		 ('s','tool','[Duplicate tool output — same content as a more recent call]','c7',NULL,'read_file',1785000007.0);`)
	by := hermesRoles(t, db)
	want := []string{"1|retries: 3\n2|Always retry on 503", "-retries: 3\n+retries: 5", "file not found: /nope"}
	if got := by[RoleToolOutput]; strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Errorf("tool output = %q, want %q", got, want)
	}
}

// Compaction archives the live rows (active=0, compacted=1) and writes the
// kept tail again as new active rows; rewind takes turns back (active=0,
// compacted=0). The first must not count a call twice, the second must not
// count at all (hermes_state.py archive_and_compact, rewind_to_message).
func TestHermesCompactionAndRewind(t *testing.T) {
	call := func(id, path string) string {
		return `'[{"id":"` + id + `","type":"function","function":{"name":"patch","arguments":"{\"mode\": \"replace\", \"path\": \"` + path + `\", \"old_string\": \"retries = compute_old_retry_budget()\", \"new_string\": \"retries = compute_new_retry_budget()\"}"}}]'`
	}
	db := writeHermesStore(t, hermes017Schema, `
		INSERT INTO messages (session_id,role,content,tool_call_id,tool_calls,tool_name,timestamp,active,compacted) VALUES
		 ('s','user','fix the retry budget',NULL,NULL,NULL,1785000000.0,0,1),
		 ('s','assistant','',NULL,`+call("c1", "/w/a.py")+`,NULL,1785000001.0,0,1),
		 ('s','tool','{"success": true, "diff": "-old\n+new"}','c1',NULL,'patch',1785000002.0,0,1),
		 ('s','assistant','',NULL,`+call("c1", "/w/a.py")+`,NULL,1785000010.0,1,0),
		 ('s','tool','[Old tool output cleared to save context space]','c1',NULL,'patch',1785000011.0,1,0),
		 ('s','user','try b.py instead',NULL,NULL,NULL,1785000020.0,0,0),
		 ('s','assistant','',NULL,`+call("c9", "/w/b.py")+`,NULL,1785000021.0,0,0);`)
	by := hermesRoles(t, db)
	if got := by[RoleEdit]; len(got) != 1 || !strings.HasPrefix(got[0], "/w/a.py\n") {
		t.Errorf("edits = %q, want a.py once and no rewound b.py", got)
	}
	if got := strings.Join(by[RoleFiles], "|"); got != "/w/a.py" {
		t.Errorf("files = %q", got)
	}
	if got := strings.Join(by["user"], "|"); got != "fix the retry budget" {
		t.Errorf("user = %q, want the rewound turn left out", got)
	}
	if got := strings.Join(by[RoleToolOutput], "|"); got != "-old\n+new" {
		t.Errorf("tool output = %q", got)
	}
}

// V4A as Hermes' own parser takes it (tools/patch_parser.py): spacing after
// *** is optional, and Move File names two paths.
func TestHermesPatchHeadersAsHermesReadsThem(t *testing.T) {
	patch := `*** Begin Patch\\n***Update File: /w/a.py\\n@@\\n-old_value = compute_the_old_way()\\n+new_value = compute_the_new_way()\\n*** Move File: /w/b.py -> /w/c.py\\n*** End Patch`
	db := writeHermesStore(t, hermes017Schema, `
		INSERT INTO messages (session_id,role,content,tool_call_id,tool_calls,tool_name,timestamp) VALUES
		 ('s','user','move it',NULL,NULL,NULL,1785000000.0),
		 ('s','assistant','',NULL,'[{"id":"c1","type":"function","function":{"name":"patch","arguments":"{\"mode\": \"patch\", \"patch\": \"`+patch+`\"}"}}]',NULL,1785000001.0);`)
	by := hermesRoles(t, db)
	if got := strings.Join(by[RoleFiles], "|"); got != "/w/a.py\n/w/b.py\n/w/c.py" {
		t.Errorf("files = %q", got)
	}
	if got := strings.Join(by[RoleEdit], "|"); got != "/w/a.py\nold_value = compute_the_old_way()" {
		t.Errorf("edits = %q", got)
	}
	if len(by[RoleWrote]) != 1 {
		t.Errorf("wrote = %q", by[RoleWrote])
	}
}

// tool_calls is json.dumps of whatever the caller passed, and a single call
// can arrive as a dict rather than a list (hermes_state.py append_message).
func TestHermesToolCallsAsOneObject(t *testing.T) {
	db := writeHermesStore(t, hermes017Schema, `
		INSERT INTO messages (session_id,role,content,tool_call_id,tool_calls,tool_name,timestamp) VALUES
		 ('s','user','run the tests',NULL,NULL,NULL,1785000000.0),
		 ('s','assistant','',NULL,'{"id":"c1","type":"function","function":{"name":"terminal","arguments":"{\"command\": \"go test ./...\"}"}}',NULL,1785000001.0);`)
	by := hermesRoles(t, db)
	if got := strings.Join(by[RoleCommand], "|"); got != "$ go test ./..." {
		t.Errorf("commands = %q", got)
	}
}

// A store from before the tool columns still gives its prose: naming a
// missing column would fail the whole query.
func TestHermesStoreWithoutToolColumns(t *testing.T) {
	db := writeHermesStore(t, `CREATE TABLE messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL,
		role TEXT NOT NULL, content TEXT, timestamp REAL NOT NULL);`, `
		INSERT INTO messages (session_id,role,content,timestamp) VALUES
		 ('s','user','an old question',1785000000.0),
		 ('s','assistant','an old answer',1785000001.0);`)
	by := hermesRoles(t, db)
	if got := strings.Join(by["user"], "|") + "/" + strings.Join(by["assistant"], "|"); got != "an old question/an old answer" {
		t.Errorf("prose = %q", got)
	}
}

// Hermes fills a missing call id with sha256(name:args:index)
// (agent/chat_completion_helpers.py), so the same command run twice carries
// the same id. Only a call compaction wrote again is a repeat.
func TestHermesRepeatedCallIDKeepsBothRuns(t *testing.T) {
	call := `'[{"id":"call_3f1a2b4c5d6e","type":"function","function":{"name":"terminal","arguments":"{\"command\": \"go test ./...\"}"}}]'`
	db := writeHermesStore(t, hermes017Schema, `
		INSERT INTO messages (session_id,role,content,tool_call_id,tool_calls,tool_name,timestamp) VALUES
		 ('s','user','make the tests pass',NULL,NULL,NULL,1785000000.0),
		 ('s','assistant','',NULL,`+call+`,NULL,1785000001.0),
		 ('s','tool','{"output": "FAIL pool_test.go:12", "exit_code": 1}','call_3f1a2b4c5d6e',NULL,'terminal',1785000002.0),
		 ('s','assistant','',NULL,`+call+`,NULL,1785000010.0),
		 ('s','tool','{"output": "ok  example/pool 0.2s", "exit_code": 0}','call_3f1a2b4c5d6e',NULL,'terminal',1785000011.0);`)
	by := hermesRoles(t, db)
	if got := strings.Join(by[RoleCommand], "|"); got != "$ go test ./...  → exit 1|$ go test ./..." {
		t.Errorf("commands = %q", by[RoleCommand])
	}
	if got := strings.Join(by[RoleToolOutput], "|"); got != "FAIL pool_test.go:12|ok  example/pool 0.2s" {
		t.Errorf("tool output = %q", by[RoleToolOutput])
	}
}

// Results that hold their text somewhere other than the known keys keep it:
// search_files in files mode, web_search, web_extract, delegate_task, and an
// output that is not a string.
func TestHermesStructuredResultsKeepTheirText(t *testing.T) {
	db := writeHermesStore(t, hermes017Schema, `
		INSERT INTO messages (session_id,role,content,tool_call_id,tool_calls,tool_name,timestamp) VALUES
		 ('s','user','where is the retry budget',NULL,NULL,NULL,1785000000.0),
		 ('s','tool','{"total_count": 1, "files": ["/w/retry_budget.go"]}','c1',NULL,'search_files',1785000001.0),
		 ('s','tool','{"total_count": 1, "matches": [{"path": "/w/a.go", "line": 3, "content": "computeRetryBudget()"}]}','c2',NULL,'search_files',1785000002.0),
		 ('s','tool','{"success": true, "data": {"web": [{"title": "Retry budgets in gRPC", "url": "https://example.com"}]}}','c3',NULL,'web_search',1785000003.0),
		 ('s','tool','{"results": [{"url": "https://example.com", "content": "extracted page body"}]}','c4',NULL,'web_extract',1785000004.0),
		 ('s','tool','{"results": [{"task_index": 0, "status": "completed", "summary": "the leak is in pool.go"}]}','c5',NULL,'delegate_task',1785000005.0),
		 ('s','tool','{"output": 42}','c6',NULL,'execute_code',1785000006.0),
		 ('s','tool','{"output": ["line one", "line two"], "exit_code": 0}','c7',NULL,'terminal',1785000007.0),
		 ('s','tool','{"bytes_written": 26, "_warning": "file was overwritten"}','c8',NULL,'write_file',1785000008.0);`)
	got := strings.Join(hermesRoles(t, db)[RoleToolOutput], "|")
	for _, want := range []string{"/w/retry_budget.go", "computeRetryBudget()", "Retry budgets in gRPC",
		"extracted page body", "the leak is in pool.go", "42", "line one\nline two"} {
		if !strings.Contains(got, want) {
			t.Errorf("tool output %q lost %q", got, want)
		}
	}
	if strings.Contains(got, "overwritten") {
		t.Errorf("tool output %q kept a hint meant for the model", got)
	}
}

// -1 is a command that never ran; any other negative code is a process killed
// by a signal after it started (tools/environments/base.py), which ran.
func TestHermesKilledCommandStaysACommand(t *testing.T) {
	db := writeHermesStore(t, hermes017Schema, `
		INSERT INTO messages (session_id,role,content,tool_call_id,tool_calls,tool_name,timestamp) VALUES
		 ('s','user','build it',NULL,NULL,NULL,1785000000.0),
		 ('s','assistant','',NULL,'[{"id":"c1","type":"function","function":{"name":"terminal","arguments":"{\"command\": \"make all\"}"}}]',NULL,1785000001.0),
		 ('s','tool','{"output": "cc1: out of memory", "exit_code": -9}','c1',NULL,'terminal',1785000002.0);`)
	if got := strings.Join(hermesRoles(t, db)[RoleCommand], "|"); got != "$ make all" {
		t.Errorf("commands = %q, want the killed run kept", got)
	}
}

// After Move File and Delete File, Hermes' parser has no current file and
// ignores lines until the next header (tools/patch_parser.py).
func TestHermesPatchLinesAfterMoveBelongToNoFile(t *testing.T) {
	patch := `***Begin Patch\\n*** Move File: /w/b.py -> /w/c.py\\n-ghost_line_hermes_ignores = 1\\n*** Delete File: /w/d.py\\n-another_ignored_line = 2\\n*** Update File: /w/a.py\\n@@\\n-old_value = compute_the_old_way()\\n+new_value = compute_the_new_way()\\n***End Patch`
	db := writeHermesStore(t, hermes017Schema, `
		INSERT INTO messages (session_id,role,content,tool_call_id,tool_calls,tool_name,timestamp) VALUES
		 ('s','user','q',NULL,NULL,NULL,1785000000.0),
		 ('s','assistant','',NULL,'[{"id":"c1","type":"function","function":{"name":"patch","arguments":"{\"mode\": \"patch\", \"patch\": \"`+patch+`\"}"}}]',NULL,1785000001.0);`)
	by := hermesRoles(t, db)
	if got := strings.Join(by[RoleEdit], "|"); got != "/w/a.py\nold_value = compute_the_old_way()" {
		t.Errorf("edits = %q", by[RoleEdit])
	}
}

// A store with active but no compacted still hides a rewound turn.
func TestHermesActiveWithoutCompacted(t *testing.T) {
	db := writeHermesStore(t, `CREATE TABLE messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, role TEXT NOT NULL,
		content TEXT, tool_call_id TEXT, tool_calls TEXT, tool_name TEXT,
		timestamp REAL NOT NULL, active INTEGER NOT NULL DEFAULT 1);`, `
		INSERT INTO messages (session_id,role,content,timestamp,active) VALUES
		 ('s','user','keep this question',1785000000.0,1),
		 ('s','user','a turn taken back',1785000001.0,0);`)
	if got := strings.Join(hermesRoles(t, db)["user"], "|"); got != "keep this question" {
		t.Errorf("user = %q", got)
	}
}

// In-place compaction writes the kept head and tail again as live rows, with
// a summary between them; the archived originals are still there (#4296).
func TestHermesCompactionDoesNotRepeatProse(t *testing.T) {
	db := writeHermesStore(t, hermes017Schema, `
		INSERT INTO messages (session_id,role,content,tool_call_id,tool_calls,tool_name,timestamp,active,compacted) VALUES
		 ('s','user','fix the retry budget',NULL,NULL,NULL,1785000000.0,0,1),
		 ('s','assistant','looking at it now',NULL,NULL,NULL,1785000001.0,0,1),
		 ('s','user','fix the retry budget',NULL,NULL,NULL,1785000010.0,1,0),
		 ('s','assistant','[CONTEXT COMPACTION — REFERENCE ONLY] Earlier turns were compacted into the summary below.',NULL,NULL,NULL,1785000011.0,1,0),
		 ('s','assistant','looking at it now',NULL,NULL,NULL,1785000012.0,1,0),
		 ('s','user','now the pool size',NULL,NULL,NULL,1785000020.0,1,0);`)
	by := hermesRoles(t, db)
	if got := strings.Join(by["user"], "|"); got != "fix the retry budget|now the pool size" {
		t.Errorf("user = %q", by["user"])
	}
	if got := strings.Join(by["assistant"], "|"); got != "looking at it now" {
		t.Errorf("assistant = %q", by["assistant"])
	}
	if len(by[RoleSummary]) != 1 {
		t.Errorf("summary = %q, want the compaction summary under its own role", by[RoleSummary])
	}
}
