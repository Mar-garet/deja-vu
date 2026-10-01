package sources

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// An Amp thread records its tool calls as tool_use blocks and their results as
// tool_result blocks with a run, and stamps user turns with meta.sentAt and
// assistant turns with usage.timestamp. Reading only text blocks under the
// thread's created time lost every command and edit (#4356).
func TestParseAmpKeepsToolCallsAndMessageTimes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "T-0f3c.json")
	data := `{"v":7,"id":"T-0f3c","created":1774950000000,"title":"Fix the retry loop",
"env":{"initial":{"trees":[{"uri":"file:///tmp/proj"}]}},
"messages":[
{"role":"user","content":[{"type":"text","text":"fix the retry loop in client.go"}],"meta":{"sentAt":1774950001000}},
{"role":"assistant","content":[{"type":"text","text":"Running the tests first."},{"type":"tool_use","id":"toolu_01","name":"Bash","complete":true,"input":{"cmd":"go test ./...","cwd":"/tmp/proj"}}],"usage":{"timestamp":"2026-03-31T09:40:05Z"}},
{"role":"user","content":[{"type":"tool_result","toolUseID":"toolu_01","run":{"status":"done","result":{"output":"--- FAIL: TestRetry (0.00s)","exitCode":1}}}]},
{"role":"assistant","content":[{"type":"tool_use","id":"toolu_02","name":"edit_file","complete":true,"input":{"path":"/tmp/proj/client.go","old_str":"for i := 0; i <= max; i++","new_str":"for i := 0; i < max; i++"}}]},
{"role":"user","content":[{"type":"tool_result","toolUseID":"toolu_02","run":{"status":"done","result":{"diff":"@@ -1 +1 @@"}}}]},
{"role":"assistant","content":[{"type":"tool_use","id":"toolu_03","name":"create_file","complete":true,"input":{"path":"/tmp/proj/retry_test.go","content":"package proj\n\nfunc TestRetryStops() {}\n"}}]},
{"role":"assistant","content":[{"type":"text","text":"Fixed the off-by-one in the retry loop."}]},
{"role":"user","content":[{"type":"text","text":"thanks"}],"meta":{"sentAt":1774950300000}}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	ss, err := ParseAmpFile(path)
	if err != nil || len(ss) != 1 {
		t.Fatalf("ParseAmpFile = %v, %v", ss, err)
	}
	s := ss[0]
	byRole := map[string][]string{}
	for _, m := range s.Messages {
		byRole[m.Role] = append(byRole[m.Role], m.Text)
	}
	if got := byRole[RoleCommand]; len(got) != 1 || got[0] != "$ go test ./..." {
		t.Errorf("commands = %q", got)
	}
	if got := byRole[RoleToolOutput]; len(got) != 1 || got[0] != "--- FAIL: TestRetry (0.00s)" {
		t.Errorf("tool output = %q", got)
	}
	if got := byRole[RoleFiles]; len(got) != 2 || got[0] != "/tmp/proj/client.go" || got[1] != "/tmp/proj/retry_test.go" {
		t.Errorf("files = %q", got)
	}
	if got := byRole[RoleEdit]; len(got) != 1 || got[0] != "/tmp/proj/client.go\nfor i := 0; i <= max; i++" {
		t.Errorf("edits = %q", got)
	}
	if got := byRole[RoleWrote]; len(got) == 0 {
		t.Errorf("no wrote record for the edit or the new file")
	}

	first, last := s.Messages[0], s.Messages[len(s.Messages)-1]
	if first.Role != "user" || !first.Time.Equal(time.UnixMilli(1774950001000)) {
		t.Errorf("first message = %s at %v, want user at sentAt", first.Role, first.Time)
	}
	if last.Text != "thanks" || !last.Time.Equal(time.UnixMilli(1774950300000)) {
		t.Errorf("last message = %q at %v, want thanks at its sentAt", last.Text, last.Time)
	}
	want := time.Date(2026, 3, 31, 9, 40, 5, 0, time.UTC)
	if m := s.Messages[1]; m.Role != "assistant" || !m.Time.Equal(want) {
		t.Errorf("assistant turn at %v, want its usage.timestamp %v", m.Time, want)
	}
	if !s.Updated.Equal(last.Time) {
		t.Errorf("session updated %v, want the last turn %v", s.Updated, last.Time)
	}
}
