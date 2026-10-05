package sources

import (
	"path/filepath"
	"strings"
	"testing"
)

// CodeBuddy 2.161.2 writes how a command ended only into the result's text,
// as an `Exit Code: N` line, and keeps status "completed" either way. Read
// without it, a failed make was stored as a bare `$ make …` (#4703).
func TestCodeBuddyCommandCarriesExitCode(t *testing.T) {
	home := codeBuddyEnv(t)
	path := filepath.Join(home, ".codebuddy", "projects", "-work-app", "s1.jsonl")
	writeCodeBuddyFile(t, path, strings.Join([]string{
		`{"id":"u1","timestamp":1791187200000,"type":"message","role":"user","content":[{"type":"input_text","text":"build it"}],"sessionId":"s1","cwd":"/work/app"}`,
		`{"id":"a1","timestamp":1791187201000,"type":"function_call","name":"Bash","callId":"call_1","arguments":"{\"command\":\"make build-everything\"}","sessionId":"s1","cwd":"/work/app"}`,
		`{"id":"r1","timestamp":1791187202000,"type":"function_call_result","name":"Bash","callId":"call_1","status":"completed","output":{"type":"text","text":"Command: make build-everything\nStdout: (empty)\nStderr: make: *** No rule to make target\n\nExit Code: 2\nSignal: (none)"},"sessionId":"s1","cwd":"/work/app"}`,
		`{"id":"a2","timestamp":1791187203000,"type":"function_call","name":"Bash","callId":"call_2","arguments":"{\"command\":\"make all\"}","sessionId":"s1","cwd":"/work/app"}`,
		`{"id":"r2","timestamp":1791187204000,"type":"function_call_result","name":"Bash","callId":"call_2","status":"completed","output":{"type":"text","text":"Command: make all\nStdout: ok\nStderr: (empty)\nExit Code: 0\nSignal: (none)"},"sessionId":"s1","cwd":"/work/app"}`,
		// The PowerShell builder leaves the line out on success.
		`{"id":"a3","timestamp":1791187205000,"type":"function_call","name":"PowerShell","callId":"call_3","arguments":"{\"command\":\"go test ./...\"}","sessionId":"s1","cwd":"/work/app"}`,
		`{"id":"r3","timestamp":1791187206000,"type":"function_call_result","name":"PowerShell","callId":"call_3","status":"completed","output":{"type":"text","text":"Command: go test ./...\nStdout: ok"},"sessionId":"s1","cwd":"/work/app"}`,
		// Killed by a signal: no code to state.
		`{"id":"a4","timestamp":1791187207000,"type":"function_call","name":"Bash","callId":"call_4","arguments":"{\"command\":\"make serve\"}","sessionId":"s1","cwd":"/work/app"}`,
		`{"id":"r4","timestamp":1791187208000,"type":"function_call_result","name":"Bash","callId":"call_4","status":"completed","output":{"type":"text","text":"Command: make serve\nStdout: (empty)\nStderr: (empty)\nExit Code: (none)\nSignal: SIGTERM"},"sessionId":"s1","cwd":"/work/app"}`,
		// Output that prints a status-shaped line of its own.
		`{"id":"a5","timestamp":1791187209000,"type":"function_call","name":"Bash","callId":"call_5","arguments":"{\"command\":\"go test ./parser/...\"}","sessionId":"s1","cwd":"/work/app"}`,
		`{"id":"r5","timestamp":1791187210000,"type":"function_call_result","name":"Bash","callId":"call_5","status":"completed","output":{"type":"text","text":"Command: go test ./parser/...\nStdout: Exit Code: 0\nFAIL\nStderr: (empty)\nExit Code: 1\nSignal: (none)"},"sessionId":"s1","cwd":"/work/app"}`,
		// Backgrounded, and never launched: no code either way.
		`{"id":"a6","timestamp":1791187211000,"type":"function_call","name":"Bash","callId":"call_6","arguments":"{\"command\":\"go test ./slow/...\"}","sessionId":"s1","cwd":"/work/app"}`,
		`{"id":"r6","timestamp":1791187212000,"type":"function_call_result","name":"Bash","callId":"call_6","status":"completed","output":{"type":"text","text":"Command: go test ./slow/...\nStatus: Running in background with task_id: t1\nReason: Auto-backgrounded after hitting foreground timeout"},"sessionId":"s1","cwd":"/work/app"}`,
		`{"id":"a7","timestamp":1791187213000,"type":"function_call","name":"PowerShell","callId":"call_7","arguments":"{\"command\":\"go vet ./...\"}","sessionId":"s1","cwd":"/work/app"}`,
		`{"id":"r7","timestamp":1791187214000,"type":"function_call_result","name":"PowerShell","callId":"call_7","status":"completed","output":{"type":"text","text":"Command: go vet ./...\nError: [SandboxError] could not be launched"},"sessionId":"s1","cwd":"/work/app"}`,
	}, "\n")+"\n")
	ss, err := ParseCodeBuddyFile(path)
	if err != nil || len(ss) != 1 {
		t.Fatalf("parse: %v %d", err, len(ss))
	}
	var cmds []string
	for _, m := range ss[0].Messages {
		if m.Role == RoleCommand {
			cmds = append(cmds, m.Text)
		}
	}
	want := []string{"$ make build-everything  → exit 2", "$ make all  → exit 0", "$ go test ./...  → exit 0", "$ make serve", "$ go test ./parser/...  → exit 1", "$ go test ./slow/...", "$ go vet ./..."}
	if strings.Join(cmds, "|") != strings.Join(want, "|") {
		t.Fatalf("commands:\n got %q\nwant %q", cmds, want)
	}
}

// /compact leaves its instruction prompt in the transcript as a user message
// marked only agent:"compact" — CodeBuddy's own isInjectedUserItem skips it,
// and it is nobody's words (#4704).
func TestCodeBuddySkipsCompactionPrompt(t *testing.T) {
	home := codeBuddyEnv(t)
	path := filepath.Join(home, ".codebuddy", "projects", "-work-app", "s2.jsonl")
	writeCodeBuddyFile(t, path, strings.Join([]string{
		`{"id":"u1","timestamp":1791187200000,"type":"message","role":"user","content":[{"type":"input_text","text":"fix the parser"}],"providerData":{"agent":"cli"},"sessionId":"s2","cwd":"/work/app"}`,
		`{"id":"u2","timestamp":1791187260000,"type":"message","role":"user","content":[{"type":"input_text","text":"**IMPORTANT CONSTRAINTS:**\n- Do NOT use any tools"}],"providerData":{"agent":"compact"},"sessionId":"s2","cwd":"/work/app"}`,
		`{"id":"a2","timestamp":1791187261000,"type":"message","role":"assistant","content":[{"type":"output_text","text":"<conversation_history_summary>x</conversation_history_summary>"}],"providerData":{"agent":"compact","isCompactInternal":true,"isSummary":true},"sessionId":"s2","cwd":"/work/app"}`,
	}, "\n")+"\n")
	ss, err := ParseCodeBuddyFile(path)
	if err != nil || len(ss) != 1 {
		t.Fatalf("parse: %v %d", err, len(ss))
	}
	for _, m := range ss[0].Messages {
		if strings.Contains(m.Text, "IMPORTANT CONSTRAINTS") {
			t.Fatalf("compaction prompt indexed as %s: %q", m.Role, m.Text)
		}
	}
	if len(ss[0].Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(ss[0].Messages))
	}
}

// A compaction hook names the live CodeBuddy transcript. Its records carry
// sessionId the way Claude's do, so it was taken for one and came back with
// no session: nothing was captured and no packet followed (#4705).
func TestCodeBuddyTranscriptIsReadForCompaction(t *testing.T) {
	home := codeBuddyEnv(t)
	path := filepath.Join(home, ".codebuddy", "projects", "-work-app", "cb-1.jsonl")
	writeCodeBuddyFile(t, path, strings.Join([]string{
		`{"id":"u1","timestamp":1791187200000,"type":"message","role":"user","content":[{"type":"input_text","text":"fix the parser test"}],"providerData":{"agent":"cli"},"sessionId":"cb-1","cwd":"/work/app"}`,
		`{"timestamp":1791187200100,"type":"file-history-snapshot","messageId":"u1","snapshot":{},"cwd":"/work/app"}`,
		`{"id":"a1","timestamp":1791187201000,"type":"function_call","name":"Bash","callId":"call_1","arguments":"{\"command\":\"go test ./parser/...\"}","sessionId":"cb-1","cwd":"/work/app"}`,
		`{"id":"r1","timestamp":1791187202000,"type":"function_call_result","name":"Bash","callId":"call_1","status":"completed","output":{"type":"text","text":"Command: go test ./parser/...\nStdout: --- FAIL: TestParseSeed\nwant 3, got 4\nFAIL\nStderr: (empty)\nExit Code: 1\nSignal: (none)"},"sessionId":"cb-1","cwd":"/work/app"}`,
		`{"id":"a2","timestamp":1791187203000,"type":"function_call","name":"Edit","callId":"call_2","arguments":"{\"file_path\":\"/work/app/parse.go\",\"old_string\":\"a\",\"new_string\":\"b\"}","sessionId":"cb-1","cwd":"/work/app"}`,
		`{"id":"m2","timestamp":1791187204000,"type":"message","role":"assistant","content":[{"type":"output_text","text":"The parser test fails: want 3, got 4. I decided to fix parse.go next."}],"sessionId":"cb-1","cwd":"/work/app"}`,
	}, "\n")+"\n")
	tr, err := ReadCompactionTranscript(path, "cb-1")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Harness != "codebuddy" || tr.Workspace != "/work/app" {
		t.Fatalf("harness %q workspace %q", tr.Harness, tr.Workspace)
	}
	if len(tr.ToolCalls) != 2 || tr.ToolCalls[0].ID != "call_1" || tr.ToolCalls[1].Name != "Edit" || tr.LastToolID != "call_2" {
		t.Fatalf("tool calls %+v", tr.ToolCalls)
	}
	var cmd bool
	for _, m := range tr.Session.Messages {
		cmd = cmd || m.Role == RoleCommand && m.Text == "$ go test ./parser/...  → exit 1"
	}
	if !cmd || tr.Session.ID != "cb-1" {
		t.Fatalf("session %q messages %+v", tr.Session.ID, tr.Session.Messages)
	}
	if _, err := ReadCompactionTranscript(path, "cb-2"); err == nil {
		t.Fatal("another session id was accepted for this transcript")
	}
}

// The SessionStart guard for #4705 has to know a CodeBuddy transcript even
// outside the configured roots, or the packet is spent where it is dropped.
func TestCodeBuddyTranscriptKnownOutsideItsRoots(t *testing.T) {
	codeBuddyEnv(t)
	elsewhere := filepath.Join(t.TempDir(), "s9.jsonl")
	writeCodeBuddyFile(t, elsewhere, `{"timestamp":1,"type":"file-history-snapshot","snapshot":{}}`+"\n"+
		`{"id":"u1","timestamp":1791187200000,"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}],"sessionId":"s9","cwd":"/work/app"}`+"\n")
	if !IsCodeBuddyTranscript(elsewhere) {
		t.Fatal("a CodeBuddy transcript outside the roots was not recognised")
	}
	claude := filepath.Join(t.TempDir(), "c.jsonl")
	writeCodeBuddyFile(t, claude, `{"type":"user","sessionId":"c","message":{"role":"user","content":"hi"}}`+"\n")
	if IsCodeBuddyTranscript(claude) || IsCodeBuddyTranscript("") {
		t.Fatal("a Claude transcript was taken for CodeBuddy's")
	}
}
