package sources

import (
	"strings"
	"testing"
)

// dsh writes every tool call twice: as a tool-call block inside the
// assistant/message and as a tool/call event. Shapes from dsh 0.1.1-rc.2: bash,
// edit and write from the fs tools, and str_replace_editor, which names its
// file `path` and its spans old_str/new_str/file_text (#4291).
const deepSeekToolLog = `{"type":"session","version":0,"id":"session-11111111-2222-3333-4444-555555555555","createdAt":1790874400000,"cwd":"/tmp/proj","delegationDepth":0}
{"type":"user/message","seq":7,"time":1790874400100,"data":{"content":[{"type":"text","text":"fix the retry loop"}],"source":{"kind":"user"},"role":"user","id":"u1"}}
{"type":"assistant/message","seq":18,"time":1790874400200,"data":{"turn":1,"step":1,"message":{"role":"assistant","content":[{"type":"tool-call","id":"call_1","name":"bash","arguments":"{\"command\": \"go test ./retry/...\", \"description\": \"run the tests\"}"}],"source":{"kind":"model"},"id":"a1"}}}
{"type":"tool/call","seq":19,"time":1790874400200,"data":{"turn":1,"step":1,"callId":"call_1","name":"bash","arguments":"{\"command\": \"go test ./retry/...\", \"description\": \"run the tests\"}"}}
{"type":"tool/result","seq":20,"time":1790874400300,"data":{"turn":1,"step":1,"message":{"source":{"kind":"tool","callId":"call_1"},"content":[{"type":"tool-result","toolCallId":"call_1","content":[{"type":"text","text":"ok  \texample.com/proj/retry\t0.01s"}],"isError":false}],"role":"user","id":"t1"}}}
{"type":"assistant/message","seq":21,"time":1790874400400,"data":{"turn":1,"step":2,"message":{"role":"assistant","content":[{"type":"tool-call","id":"call_2","name":"edit","arguments":"{\"file_path\": \"/tmp/proj/retry.go\", \"old_string\": \"for {\", \"new_string\": \"for attempt := 0; attempt < maxAttempts; attempt++ {\"}"}],"source":{"kind":"model"},"id":"a2"}}}
{"type":"tool/call","seq":22,"time":1790874400400,"data":{"turn":1,"step":2,"callId":"call_2","name":"edit","arguments":"{\"file_path\": \"/tmp/proj/retry.go\", \"old_string\": \"for {\", \"new_string\": \"for attempt := 0; attempt < maxAttempts; attempt++ {\"}"}}
{"type":"tool/result","seq":23,"time":1790874400500,"data":{"turn":1,"step":2,"message":{"source":{"kind":"tool","callId":"call_2"},"content":[{"type":"tool-result","toolCallId":"call_2","content":[{"type":"text","text":"The file /tmp/proj/retry.go has been updated successfully."}],"isError":false}],"role":"user","id":"t2"}}}
{"type":"assistant/message","seq":24,"time":1790874400600,"data":{"turn":1,"step":3,"message":{"role":"assistant","content":[{"type":"tool-call","id":"call_3","name":"write","arguments":"{\"file_path\": \"/tmp/proj/PLAN.md\", \"content\": \"Back off twice as long on every retry, five tries at most.\\n\"}"},{"type":"tool-call","id":"call_4","name":"str_replace_editor","arguments":"{\"command\": \"str_replace\", \"path\": \"/tmp/proj/backoff.go\", \"old_str\": \"delay *= 2\", \"new_str\": \"delay = min(delay*2, maxDelay) // capped backoff\"}"}],"source":{"kind":"model"},"id":"a3"}}}
{"type":"tool/call","seq":25,"time":1790874400600,"data":{"turn":1,"step":3,"callId":"call_3","name":"write","arguments":"{\"file_path\": \"/tmp/proj/PLAN.md\", \"content\": \"Back off twice as long on every retry, five tries at most.\\n\"}"}}
{"type":"tool/call","seq":26,"time":1790874400600,"data":{"turn":1,"step":3,"callId":"call_4","name":"str_replace_editor","arguments":"{\"command\": \"str_replace\", \"path\": \"/tmp/proj/backoff.go\", \"old_str\": \"delay *= 2\", \"new_str\": \"delay = min(delay*2, maxDelay) // capped backoff\"}"}}
{"type":"assistant/message","seq":27,"time":1790874400700,"data":{"turn":1,"step":4,"message":{"role":"assistant","content":[{"type":"text","text":"Bounded the retry loop."}],"source":{"kind":"model"},"id":"a4"}}}
`

func TestParseDeepSeekFileKeepsToolCalls(t *testing.T) {
	path := writeDeepSeekSession(t, t.TempDir(), "tools", deepSeekToolLog, false)
	ss, err := ParseDeepSeekFile(path)
	if err != nil || len(ss) != 1 {
		t.Fatalf("parse: %v, %d sessions", err, len(ss))
	}
	byRole := map[string][]string{}
	for _, m := range ss[0].Messages {
		byRole[m.Role] = append(byRole[m.Role], m.Text)
		if m.Time.IsZero() {
			t.Errorf("%s record has no time: %q", m.Role, m.Text)
		}
	}
	// Each call is written twice; it must be counted once.
	if got := byRole[RoleCommand]; strings.Join(got, "|") != "$ go test ./retry/..." {
		t.Errorf("commands = %q, want the one bash call", got)
	}
	files := strings.Join(byRole[RoleFiles], "\n")
	for _, p := range []string{"/tmp/proj/retry.go", "/tmp/proj/PLAN.md", "/tmp/proj/backoff.go"} {
		if strings.Count(files, p) != 1 {
			t.Errorf("files = %q, want %s once", files, p)
		}
	}
	edits := byRole[RoleEdit]
	want := []string{"/tmp/proj/retry.go\nfor {", "/tmp/proj/backoff.go\ndelay *= 2"}
	if strings.Join(edits, "|") != strings.Join(want, "|") {
		t.Errorf("edits = %q, want %q", edits, want)
	}
	wrote := strings.Join(byRole[RoleWrote], "\n")
	for _, p := range []string{"/tmp/proj/retry.go", "/tmp/proj/PLAN.md", "/tmp/proj/backoff.go"} {
		if strings.Count(wrote, p+"\n") != 1 {
			t.Errorf("wrote = %q, want one record for %s", wrote, p)
		}
	}
	if len(byRole["assistant"]) != 1 || len(byRole["tool-output"]) != 2 {
		t.Errorf("prose changed: %+v", byRole)
	}
}
