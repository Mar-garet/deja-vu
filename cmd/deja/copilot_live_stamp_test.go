package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// MCP recall leaves out the sessions a hook stamped in the last twenty minutes,
// and Copilot's MCP server is told nothing about who is asking. A sessionStart
// stamp alone runs out twenty minutes into a session, and from then on recall
// answered with the transcript being written (#4551). Copilot runs
// preMcpToolCall before every MCP request with the session in the payload, so
// the stamp is fresh at the moment it is read; sessionEnd takes it back.
func TestCopilotAutoStampsTheSessionAtEachMCPCallAndClearsItAtTheEnd(t *testing.T) {
	copilotTestHome(t)
	if _, err := captureRun(t, "install", "copilot-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, copilotHooksPath())
	var cfg copilotHookFile
	if err := json.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatalf("the hook file is not JSON: %v\n%s", err, got)
	}
	for event, sub := range map[string]string{"preMcpToolCall": " hook-mcp-call", "sessionEnd": " hook-session-end"} {
		entries := cfg.Hooks[event]
		if len(entries) != 1 || entries[0].Type != "command" || !strings.HasSuffix(entries[0].Bash, sub) {
			t.Errorf("%s does not run deja%s:\n%s", event, sub, got)
		}
	}

	// The payloads Copilot CLI 1.0.91 sends, as a probe hook recorded them.
	dir := filepath.Join(t.TempDir(), "index.db")
	const id = "4636debe-086a-415f-bbb9-f8690f1baea0"
	var out bytes.Buffer
	runHookMCPCall(dir, strings.NewReader(`{"sessionId":"`+id+`","timestamp":1790931200291,"cwd":"/tmp/work","toolCallId":"call_07712fdb10eb","serverName":"deja","toolName":"deja","arguments":{"mode":"recall","query":"retry budget"}}`), &out)
	if !liveSessionIDs(dir)[id] {
		t.Errorf("the session making the MCP call is not stamped live")
	}
	// Anything printed is read as metaToUse; nothing keeps the request's _meta.
	if out.Len() != 0 {
		t.Errorf("hook-mcp-call printed %q, which Copilot would read as the request's _meta", out.String())
	}
	runHookSessionEnd(dir, strings.NewReader(`{"sessionId":"`+id+`","timestamp":1790931203994,"cwd":"/tmp/work","reason":"complete"}`))
	if liveSessionIDs(dir)[id] {
		t.Errorf("sessionEnd left the stamp in place")
	}
}
