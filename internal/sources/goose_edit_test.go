package sources

import (
	"strings"
	"testing"
)

// goose stores both sides of every file change: the developer extension's
// `edit` takes path, before and after, and `write` takes path and content
// (goose 1.46.0, read back from a store goose had just written). Only the path
// was kept, so restore and line-level blame had nothing from goose (#4265).
// The text_editor tool goose shipped before it spells the same things
// old_str, new_str and file_text.
func TestGooseEditAndWriteCallsLeaveEditAndWroteRecords(t *testing.T) {
	for _, tc := range []struct {
		name, raw  string
		edit       string
		wroteLines []string
	}{
		{
			name: "edit",
			raw: `[{"type":"toolRequest","id":"call_1","toolCall":{"status":"success","value":{"name":"edit","arguments":` +
				`{"path":"/tmp/proj/main.go","before":"func retry() {}","after":"func retry() {\n\tfor i := 0; i < 3; i++ {\n\t}\n}"}}},` +
				`"_meta":{"goose_extension":"developer"}}]`,
			edit:       "/tmp/proj/main.go\nfunc retry() {}",
			wroteLines: []string{"\tfor i := 0; i < 3; i++ {"},
		},
		{
			name: "write",
			raw: `[{"type":"toolRequest","id":"call_2","toolCall":{"status":"success","value":{"name":"write","arguments":` +
				`{"path":"/tmp/proj/NOTES.md","content":"retry loop: cap at 3 attempts\n"}}},"_meta":{"goose_extension":"developer"}}]`,
			wroteLines: []string{"retry loop: cap at 3 attempts"},
		},
		{
			name: "text_editor str_replace",
			raw: `[{"type":"toolRequest","id":"t2","toolCall":{"status":"success","value":{"name":"developer__text_editor","arguments":` +
				`{"command":"str_replace","path":"/w/app/queue.go","old_str":"size := 10","new_str":"pool := make(chan conn, maxPoolSize)"}}}}]`,
			edit:       "/w/app/queue.go\nsize := 10",
			wroteLines: []string{"pool := make(chan conn, maxPoolSize)"},
		},
		{
			// goose 1.10–1.25 takes a unified diff on str_replace, and its
			// schema calls that the preferred way to edit (#4287).
			name: "text_editor str_replace diff",
			raw: `[{"type":"toolRequest","id":"t5","toolCall":{"status":"success","value":{"name":"developer__text_editor","arguments":` +
				`{"command":"str_replace","path":"/tmp/proj/retry.go","diff":"--- a/retry.go\n+++ b/retry.go\n@@ -1 +1,3 @@\n-func retry() {}\n+func retry() {\n+\tfor i := 0; i < 3; i++ {}\n+}\n"}}}}]`,
			edit:       "/tmp/proj/retry.go\nfunc retry() {}",
			wroteLines: []string{"\tfor i := 0; i < 3; i++ {}"},
		},
		{
			// The diff wins over old_str and new_str when a call has both,
			// as it does in goose's text_editor.
			name: "text_editor str_replace diff over old_str",
			raw: `[{"type":"toolRequest","id":"t6","toolCall":{"status":"success","value":{"name":"developer__text_editor","arguments":` +
				`{"command":"str_replace","path":"/tmp/proj/retry.go","old_str":"zzz","new_str":"yyy","diff":"--- a/retry.go\n+++ b/retry.go\n@@ -1 +1 @@\n-func retry() {}\n+func retry() { backoff() }\n"}}}}]`,
			edit:       "/tmp/proj/retry.go\nfunc retry() {}",
			wroteLines: []string{"func retry() { backoff() }"},
		},
		{
			name: "text_editor write",
			raw: `[{"type":"toolRequest","id":"t3","toolCall":{"status":"success","value":{"name":"developer__text_editor","arguments":` +
				`{"command":"write","path":"/w/app/pool.go","file_text":"package app\n\nconst maxPoolSize = 40 // raised after the load test\n"}}}}]`,
			wroteLines: []string{"const maxPoolSize = 40 // raised after the load test"},
		},
	} {
		s := gooseSessionFrom(t, "assistant", tc.raw)
		var edits, wrote []string
		for _, m := range s.Messages {
			switch m.Role {
			case RoleEdit:
				edits = append(edits, m.Text)
			case RoleWrote:
				wrote = append(wrote, m.Text)
			}
		}
		if tc.edit == "" && len(edits) != 0 {
			t.Errorf("%s: edit records %q, want none", tc.name, edits)
		}
		if tc.edit != "" && (len(edits) != 1 || edits[0] != tc.edit) {
			t.Errorf("%s: edit records %q, want %q", tc.name, edits, tc.edit)
		}
		if len(wrote) != 1 {
			t.Fatalf("%s: wrote records %q, want one", tc.name, wrote)
		}
		for _, line := range tc.wroteLines {
			h, _ := HashWrittenLine(line)
			if _, has := WroteRecordHas(wrote[0], h); !has {
				t.Errorf("%s: wrote record %q lacks %q", tc.name, wrote[0], line)
			}
		}
		if !strings.HasPrefix(wrote[0], "/") {
			t.Errorf("%s: wrote record %q does not start with the path", tc.name, wrote[0])
		}
	}
}

// text_editor carries every argument any of its commands takes, and goose
// reads only the ones the command names: a view or an undo with stray old_str
// or new_str changed nothing, so it leaves no edit and no wrote record.
func TestGooseTextEditorReadsOnlyWhatTheCommandTakes(t *testing.T) {
	for _, command := range []string{"view", "undo_edit"} {
		raw := `[{"type":"toolRequest","id":"t4","toolCall":{"status":"success","value":{"name":"developer__text_editor","arguments":` +
			`{"command":"` + command + `","path":"/w/app/queue.go","old_str":"size := 10","new_str":"pool := make(chan conn, maxPoolSize)","file_text":"package app\n\nconst maxPoolSize = 40\n","edits":[{"old_str":"zzz","new_str":"pool := make(chan conn, maxPoolSize)"}]}}}}]`
		s := gooseSessionFrom(t, "assistant", raw)
		for _, m := range s.Messages {
			if m.Role == RoleEdit || m.Role == RoleWrote {
				t.Errorf("%s: %s record %q, want none", command, m.Role, m.Text)
			}
		}
	}
}

// A diff naming several files is applied under the call's path, which is then
// a directory: each file gets its own edit and wrote record (#4287).
func TestGooseTextEditorMultiFileDiff(t *testing.T) {
	raw := `[{"type":"toolRequest","id":"t7","toolCall":{"status":"success","value":{"name":"developer__text_editor","arguments":` +
		`{"command":"str_replace","path":"/tmp/proj","diff":"--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old a\n+retryBudget := 3 * time.Second\n--- a/b.go\n+++ b/b.go\n@@ -1 +1 @@\n-old b\n+poolSize := runtime.NumCPU() * 4\n"}}}}]`
	s := gooseSessionFrom(t, "assistant", raw)
	var edits, wrote []string
	for _, m := range s.Messages {
		switch m.Role {
		case RoleEdit:
			edits = append(edits, m.Text)
		case RoleWrote:
			wrote = append(wrote, m.Text)
		}
	}
	if strings.Join(edits, "|") != "/tmp/proj/a.go\nold a|/tmp/proj/b.go\nold b" {
		t.Errorf("edit records %q", edits)
	}
	if len(wrote) != 2 || !strings.HasPrefix(wrote[0], "/tmp/proj/a.go\n") || !strings.HasPrefix(wrote[1], "/tmp/proj/b.go\n") {
		t.Errorf("wrote records %q", wrote)
	}
}

// The diff's headers go under the base goose uses, in the path's own
// convention, without repeating the directories the two share.
func TestGooseTextEditorDiffPaths(t *testing.T) {
	for _, tc := range []struct{ call, header, want string }{
		{"/w/proj/retry.go", "b/retry.go", "/w/proj/retry.go"},
		{"/w/proj/src/retry.go", "b/src/retry.go", "/w/proj/src/retry.go"},
		{"/w/proj/src", "b/src/a.go", "/w/proj/src/a.go"},
		{"/w/proj", "b/pkg/a.go", "/w/proj/pkg/a.go"},
		{`C:\w\proj\src\retry.go`, "b/src/retry.go", `C:\w\proj\src\retry.go`},
		{"/w/proj/retry.go", "/abs/other.go", "/abs/other.go"},
		// A relative path is under goose's working directory, and a file's
		// base is that directory, not the file.
		{"main.go", "b/util.go", "util.go"},
	} {
		diff := "--- " + tc.header + "\n+++ " + tc.header + "\n@@ -1 +1 @@\n-x := 1\n+x := 2\n"
		if files, _, _ := gooseTextEditorDiff(tc.call, diff); len(files) != 1 || files[0] != tc.want {
			t.Errorf("%s with %s: files %q, want %q", tc.call, tc.header, files, tc.want)
		}
	}
}
