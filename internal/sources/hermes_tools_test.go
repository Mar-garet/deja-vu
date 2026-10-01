package sources

import (
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
		RoleToolOutput: {"notes.txt", `{"bytes_written": 26}`, "SyntaxError: bad"},
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
