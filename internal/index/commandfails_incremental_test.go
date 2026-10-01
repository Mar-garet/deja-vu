package index

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// cfStore is a claude store of sessions that run commands, some of which fail
// the same way in more than one session, so the failure table has rows.
type cfStore struct {
	t    *testing.T
	root string
}

func newCFStore(t *testing.T) (*cfStore, string) {
	t.Helper()
	tmp := t.TempDir()
	setHome(t, tmp)
	t.Setenv("DEJA_CLAUDE_ROOT", filepath.Join(tmp, "claude"))
	t.Setenv("DEJA_CODEX_ROOT", filepath.Join(tmp, "codex"))
	t.Setenv("DEJA_OPENCODE_DB", filepath.Join(tmp, "none.db"))
	t.Setenv("DEJA_NOTES_FILE", filepath.Join(tmp, "notes.jsonl"))
	return &cfStore{t: t, root: filepath.Join(tmp, "claude")}, tmp
}

func (c *cfStore) path(project string, i int) string {
	return filepath.Join(c.root, "-tmp-"+project, fmt.Sprintf("%08d-0000-4000-8000-000000000000.jsonl", i))
}

func (c *cfStore) sid(i int) string { return fmt.Sprintf("%08d-0000-4000-8000-000000000000", i) }

// turn is one Bash call and what it printed.
func (c *cfStore) turn(project string, i, k int, cmd, out string, failed bool) string {
	sid, id := c.sid(i), fmt.Sprintf("call_%d_%d", i, k)
	ts := fmt.Sprintf("2026-09-%02dT10:%02d:00Z", 1+i%28, k%60)
	cwd := "/tmp/" + project
	a := map[string]any{"type": "assistant", "sessionId": sid, "timestamp": ts, "cwd": cwd, "message": map[string]any{
		"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": id, "name": "Bash", "input": map[string]any{"command": cmd}}}}}
	r := map[string]any{"type": "user", "sessionId": sid, "timestamp": ts, "cwd": cwd, "message": map[string]any{
		"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "is_error": failed, "content": out}}}}
	ab, _ := json.Marshal(a)
	rb, _ := json.Marshal(r)
	return string(ab) + "\n" + string(rb) + "\n"
}

func (c *cfStore) prompt(project string, i int, text string) string {
	b, _ := json.Marshal(map[string]any{"type": "user", "sessionId": c.sid(i), "timestamp": fmt.Sprintf("2026-09-%02dT09:59:00Z", 1+i%28),
		"cwd": "/tmp/" + project, "message": map[string]any{"role": "user", "content": text}})
	return string(b) + "\n"
}

// write gives session i a prompt and three runs: a failing test, a passing
// build and, every third session, a failing lint.
func (c *cfStore) write(project string, i int, title string) {
	c.t.Helper()
	body := c.prompt(project, i, title) +
		c.turn(project, i, 0, "go test ./...", "--- FAIL: TestStoreRejectsStaleWrites (0.01s)\nFAIL", true) +
		c.turn(project, i, 1, "go build ./...", "ok", false)
	if i%3 == 0 {
		body += c.turn(project, i, 2, "npm run lint", "npm ERR! missing script: lint", true)
	}
	if err := os.MkdirAll(filepath.Dir(c.path(project, i)), 0o755); err != nil {
		c.t.Fatal(err)
	}
	if err := os.WriteFile(c.path(project, i), []byte(body), 0o644); err != nil {
		c.t.Fatal(err)
	}
}

func (c *cfStore) appendTurn(project string, i, k int, cmd, out string) {
	c.t.Helper()
	f, err := os.OpenFile(c.path(project, i), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(c.turn(project, i, k, cmd, out, true)); err != nil {
		c.t.Fatal(err)
	}
}

func countOutputsRead(t *testing.T) *int {
	n := new(int)
	commandFailOutputRead = func() { *n++ }
	t.Cleanup(func() { commandFailOutputRead = nil })
	return n
}

// An update that touched one session used to search every tool output in the
// index for its failure line: 1.26 s of a 1.85 s update at 10,000 sessions
// (#4288). What it reads has to follow what changed, not the corpus.
func TestACommandFailureUpdateReadsOnlyWhatChanged(t *testing.T) {
	c, tmp := newCFStore(t)
	const n = 40
	for i := 0; i < n; i++ {
		c.write("app", i, fmt.Sprintf("fix the store test %d", i))
	}
	dir := filepath.Join(tmp, "index.db")
	if err := Ensure(dir, "", true, nil); err != nil {
		t.Fatal(err)
	}
	read := countOutputsRead(t)

	// The first update after a full build has nothing to start from and walks
	// everything once. That is the control: the counter sees the whole index.
	c.appendTurn("app", 0, 10, "go test ./...", "--- FAIL: TestStoreRejectsStaleWrites (0.01s)\nFAIL")
	if err := Ensure(dir, "", false, nil); err != nil {
		t.Fatal(err)
	}
	if *read < n {
		t.Fatalf("the first update read %d outputs of more than %d, so the counter does not see the walk", *read, n)
	}

	*read = 0
	c.appendTurn("app", 1, 11, "go vet ./...", "./store.go:12:2: undefined: Flock")
	if err := Ensure(dir, "", false, nil); err != nil {
		t.Fatal(err)
	}
	if *read > 1 {
		t.Errorf("one appended run read %d tool outputs, want 1", *read)
	}

	// A transcript rewritten in place goes through the replacement path, which
	// reads that session again from the start: its two or three runs.
	*read = 0
	c.write("app", 2, "fix the store test 2, take two")
	if err := Ensure(dir, "", false, nil); err != nil {
		t.Fatal(err)
	}
	if *read == 0 || *read > 3 {
		t.Errorf("replacing one session read %d tool outputs, want that session's, at most 3", *read)
	}

	// Without the state, the walk is whole again.
	if err := os.Remove(commandFailStatePath(dir)); err != nil {
		t.Fatal(err)
	}
	*read = 0
	c.appendTurn("app", 3, 12, "go test ./...", "--- FAIL: TestStoreRejectsStaleWrites (0.01s)\nFAIL")
	if err := Ensure(dir, "", false, nil); err != nil {
		t.Fatal(err)
	}
	if *read < n {
		t.Errorf("with no state on file the update read %d outputs, want the whole index", *read)
	}
}

// The table an update leaves has to be the one a full rebuild of the same
// transcripts writes — rows, order, counts and the project each names — after
// appends, rewrites, removals and new sessions (#4288).
func TestACommandFailureUpdateEndsWhereAFullBuildDoes(t *testing.T) {
	c, tmp := newCFStore(t)
	projects := []string{"app", "web"}
	for i := 0; i < 24; i++ {
		c.write(projects[i%2], i, fmt.Sprintf("fix the store test %d", i))
	}
	// Four sessions in a project of their own, the only ones whose builds
	// fail, so removing the project takes that row away.
	for i := 40; i < 44; i++ {
		c.write("old", i, fmt.Sprintf("an old project %d", i))
		c.appendTurn("old", i, 10, "make", "make: *** [all] Error 2")
	}
	dir := filepath.Join(tmp, "index.db")
	if err := Ensure(dir, "", true, nil); err != nil {
		t.Fatal(err)
	}
	check := func(step string) {
		t.Helper()
		got := ReadCommandFails(dir)
		if len(got) == 0 {
			t.Fatalf("%s: the table is empty, so there is nothing to compare", step)
		}
		if want := commandFailsByFullWalk(t, dir); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the update left\n%+v\na full walk of the same index gives\n%+v", step, got, want)
		}
		fresh := filepath.Join(t.TempDir(), "index.db")
		if err := Ensure(fresh, "", true, nil); err != nil {
			t.Fatal(err)
		}
		if want := ReadCommandFails(fresh); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the update left\n%+v\na full build of the same transcripts gives\n%+v", step, got, want)
		}
	}

	c.appendTurn("app", 0, 10, "go vet ./...", "./store.go:12:2: undefined: Flock")
	if err := Ensure(dir, "", false, nil); err != nil {
		t.Fatal(err)
	}
	check("first append")
	hasMake := func() bool {
		for _, f := range ReadCommandFails(dir) {
			if f.Head == "make" {
				return true
			}
		}
		return false
	}
	if !hasMake() {
		t.Fatal("the old project's make failure is not on file, so removing it proves nothing")
	}

	c.appendTurn("web", 1, 10, "go vet ./...", "./store.go:12:2: undefined: Flock")
	c.appendTurn("app", 2, 10, "go vet ./...", "./store.go:12:2: undefined: Flock")
	if err := Ensure(dir, "", false, nil); err != nil {
		t.Fatal(err)
	}
	check("appends")

	// Session 0 is rewritten without the lint failure it held, and session 3
	// with the vet failure, so the rows move both ways.
	if err := os.WriteFile(c.path("app", 0), []byte(c.prompt("app", 0, "rewritten")+
		c.turn("app", 0, 1, "go build ./...", "ok", false)), 0o644); err != nil {
		t.Fatal(err)
	}
	c.write("web", 3, "fix the store test 3 again")
	c.appendTurn("web", 3, 10, "go vet ./...", "./store.go:12:2: undefined: Flock")
	if err := Ensure(dir, "", false, nil); err != nil {
		t.Fatal(err)
	}
	check("rewrites")

	// A transcript deleted on its own stays in the index (#2970); a project
	// directory gone whole leaves it, and takes its sessions' runs along.
	if err := os.RemoveAll(filepath.Dir(c.path("old", 0))); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(dir, "", false, nil); err != nil {
		t.Fatal(err)
	}
	check("removals")
	if hasMake() {
		t.Error("the removed project's make failure is still on file")
	}

	c.write("web", 31, "a new session")
	c.write("web", 33, "another new session")
	if err := Ensure(dir, "", false, nil); err != nil {
		t.Fatal(err)
	}
	check("new sessions")

	c.appendTurn("web", 31, 10, "go vet ./...", "./store.go:12:2: undefined: Flock")
	if err := Ensure(dir, "", false, nil); err != nil {
		t.Fatal(err)
	}
	check("append after the rest")
}

// commandFailsByFullWalk is the table a walk of every record in the index
// gives, with no state carried in.
func commandFailsByFullWalk(t *testing.T, dir string) []CommandFailure {
	t.Helper()
	out, _, err := commandFailsFromIndex(dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
