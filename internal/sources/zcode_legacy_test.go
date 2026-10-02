package sources

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Before the current runtime ZCode kept each conversation as
// ~/.zcode/v2/sessions/<workspaceHash>/<taskId>.json, {meta, messages}, the
// shape the runtime's own restore-legacy-sessions skill scans. Those stay on
// disk until restored by hand, and deja read neither directory (#4432).
func TestZCodeReadsLegacySessionSnapshots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("DEJA_ZCODE_ROOT", filepath.Join(home, "absent"))
	t.Setenv("DEJA_ZCODE_DB", filepath.Join(home, "absent.sqlite"))
	dir := filepath.Join(home, ".zcode", "v2", "sessions", "5f0c1e9a")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	snap := `{"meta":{"taskId":"task-1","acpSessionId":"acp-9","workspacePath":"/private/tmp/proj","provider":"glm","title":"fix the retry loop","createdAt":1790000000000,"updatedAt":1790000060000},` +
		`"messages":[{"role":"user","content":"fix the retry loop, it never stops","timestamp":1790000000000},` +
		`{"role":"assistant","content":"capped it at five attempts with a 120 second backoff","timestamp":1790000060000}]}`
	live := filepath.Join(dir, "task-1.json")
	gone := filepath.Join(dir, "task-2.deleted.json")
	for p, body := range map[string]string{live: snap, gone: snap} {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files := ZCodeSessionFiles()
	if len(files) != 1 || files[0] != live {
		t.Fatalf("ZCodeSessionFiles = %v, want only %s: ZCode skips .deleted.json", files, live)
	}
	ss := LoadZCode()
	if len(ss) != 1 {
		t.Fatalf("sessions = %d, want the one snapshot", len(ss))
	}
	s := ss[0]
	// acpSessionId || taskId, the id the restore skill gives the session.
	if s.ID != "acp-9" || s.Harness != "zcode" || s.Project != "tmp/proj" || s.Title != "fix the retry loop" {
		t.Errorf("session = %s %s %q %q, want zcode acp-9 in tmp/proj titled from meta", s.Harness, s.ID, s.Project, s.Title)
	}
	if len(s.Messages) != 2 || s.Messages[1].Role != "assistant" || s.Messages[1].Time.IsZero() {
		t.Errorf("messages = %+v", s.Messages)
	}

	// Restored into the CLI database, the same conversation is read from
	// there, and the snapshot is not a second copy of it.
	if !SQLite3Available() {
		return
	}
	db := filepath.Join(home, "db.sqlite")
	t.Setenv("DEJA_ZCODE_DB", db)
	seed := `create table session (id text primary key, directory text, time_created integer, time_updated integer);
create table message (id text primary key, session_id text, time_created integer, data text);
create table part (id text primary key, message_id text, data text);
insert into session values ('acp-9', '/private/tmp/proj', 1790000000000, 1790000060000);
insert into message values ('m1', 'acp-9', 1790000000000, '{"role":"user","time":{"created":1790000000000}}');
insert into part values ('p1', 'm1', '{"type":"text","text":"fix the retry loop, it never stops"}');`
	if out, err := exec.Command("sqlite3", db, seed).CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 seed: %v %s", err, out)
	}
	n := 0
	for _, s := range LoadZCode() {
		if s.ID == "acp-9" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("acp-9 read %d times once restored, want once", n)
	}
}
