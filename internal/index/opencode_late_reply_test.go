package index

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vshulcz/deja-vu/internal/search"
)

// opencode creates the assistant message and its text part, part.time.start
// set, before the text streams in. A pass that reads the store in that moment
// stamps a watermark past both, and the text that lands a few milliseconds
// later is never asked for again: the reply was lost until a rebuild (#4207).
// Asking for it again must not hold the user turn twice either.
func TestAnOpencodeReplyWrittenDuringAPassReachesTheIndex(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 CLI not available")
	}
	tmp := t.TempDir()
	setHome(t, tmp)
	t.Setenv("DEJA_CLAUDE_ROOT", filepath.Join(tmp, "claude"))
	t.Setenv("DEJA_CODEX_ROOT", filepath.Join(tmp, "codex"))
	t.Setenv("DEJA_GOOSE_DB", filepath.Join(tmp, "none-goose.db"))
	t.Setenv("DEJA_NOTES_FILE", filepath.Join(tmp, "notes.jsonl"))
	db := filepath.Join(tmp, "opencode.db")
	t.Setenv("DEJA_OPENCODE_DB", db)
	run := func(sql string) {
		t.Helper()
		if out, err := exec.Command("sqlite3", db, sql).CombinedOutput(); err != nil {
			t.Fatalf("sqlite3: %v %s", err, out)
		}
	}
	// The rows of ses_f0887bef0ffezIQpytYxwnxRzE as the half-written pass
	// saw them: the reply's part exists, started, and empty.
	run(`create table session(id text primary key, directory text, title text, time_created integer, time_updated integer);
create table message(id text primary key, session_id text, time_created integer, time_updated integer, data text);
create table part(id text primary key, message_id text, session_id text, time_created integer, time_updated integer, data text);
insert into session values('s1','/w/app','basalt',1790858248400,1790858251964);
insert into message values('m1','s1',1790858248489,1790858248489,'{"role":"user","time":{"created":1790858248489}}');
insert into part values('p1','m1','s1',1790858248489,1790858248489,'{"type":"text","text":"Reply ok. Topic: the basaltfinch rollout."}');
insert into message values('m2','s1',1790858248697,1790858251964,'{"role":"assistant","time":{"created":1790858248697}}');
insert into part values('p2','m2','s1',1790858251964,1790858251964,'{"type":"text","text":"","time":{"start":1790858251964}}');`)

	dir := filepath.Join(tmp, "index.db")
	if err := Ensure(dir, "", true, nil); err != nil {
		t.Fatal(err)
	}
	if m, err := readManifest(dir); err != nil || m.Files[db].LastUpdated == 0 {
		t.Fatalf("the store has no watermark, so the next pass reads it whole and measures nothing: %v", err)
	}

	// The text lands, opencode finishes the message and touches the session.
	run(`update part set data='{"type":"text","text":"ok, quillwort","time":{"start":1790858251964,"end":1790858251965}}', time_updated=1790858251965 where id='p2';
update message set time_updated=1790858252007 where id='m2';
update session set time_updated=1790858252010 where id='s1';`)
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(db, future, future); err != nil {
		t.Fatal(err)
	}
	var said strings.Builder
	if err := Ensure(dir, "", false, &said); err != nil {
		t.Fatal(err)
	}

	hits, err := Search(dir, search.Options{Query: "quillwort", All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Errorf("the reply written during the first pass never reached the index (%q)", said.String())
	}
	s, ok, err := FindByIdentity(dir, "opencode", "s1")
	if err != nil || !ok {
		t.Fatalf("the session is not in the index: %v %v", ok, err)
	}
	var texts []string
	asked := 0
	for _, msg := range s.Messages {
		texts = append(texts, msg.Text)
		if strings.Contains(msg.Text, "basaltfinch") {
			asked++
		}
	}
	if asked != 1 || len(s.Messages) != 2 {
		t.Errorf("want the two turns once each, got %d: %q", len(s.Messages), texts)
	}
}

// A database file can grow with its earlier bytes in place, which is what the
// append path takes as a log that gained lines. A store read by whole sessions
// must not take it: the sessions it hands back are already in the index.
func TestAGrownWholeSessionStoreIsNotAppendedTo(t *testing.T) {
	tmp := t.TempDir()
	db := filepath.Join(tmp, "opencode.db")
	t.Setenv("DEJA_OPENCODE_DB", db)
	body := []byte(strings.Repeat("page\n", 200))
	if err := os.WriteFile(db, body, 0o600); err != nil {
		t.Fatal(err)
	}
	safe := lastCompleteLineOffset(db, int64(len(body)))
	old := FileState{Path: db, Size: int64(len(body)), SafeSize: safe, PrefixSample: filePrefixSample(db, safe), LastUpdated: 1}
	if err := os.WriteFile(db, append(body, []byte(strings.Repeat("more\n", 50))...), 0o600); err != nil {
		t.Fatal(err)
	}
	grown := FileState{Path: db, Size: int64(len(body) + 250)}
	if canAppendIncremental(map[string]FileState{db: grown}, map[string]FileState{db: old}) {
		t.Error("a grown opencode store went down the append path")
	}
}
