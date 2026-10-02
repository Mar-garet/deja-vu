package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Claude Code's PowerShell runs a command and NotebookEdit changes a file, but
// the hook knew neither: the notebook is named under notebook_path, the shell
// is not Bash, and the installed matcher left PowerShell out (#4489).
func TestHookToolKnowsClaudesPowerShellAndNotebookEdit(t *testing.T) {
	var in toolHookInput
	if err := json.Unmarshal([]byte(`{"tool_name":"NotebookEdit","tool_input":{"notebook_path":"/tmp/proj/retry.ipynb","new_source":"x"}}`), &in); err != nil {
		t.Fatal(err)
	}
	in.adopt()
	if in.ToolInput.FilePath != "/tmp/proj/retry.ipynb" {
		t.Errorf("NotebookEdit file = %q, want the notebook_path", in.ToolInput.FilePath)
	}
	if !isCommandTool("PowerShell") {
		t.Error("PowerShell is not read as a command tool")
	}
	for _, w := range claudeHookWiring {
		if w.Event == "PreToolUse" && !strings.Contains("|"+w.Matcher+"|", "|PowerShell|") {
			t.Errorf("PreToolUse matcher %q leaves PowerShell out", w.Matcher)
		}
	}
}

// Grok fires PostToolUse for a run_terminal_command that exited non-zero, and
// carries the output as toolResult.output_for_prompt (snake alias
// tool_response); `output` there is bytes, not text. grok-auto wired no
// PostToolUse and hook-tool-after read none of grok's keys (#4499).
func TestGrokFailedCommandGetsTheFixPair(t *testing.T) {
	seedFixPair(t, "panic: sql: database is closed", "make clean && make CGO_ENABLED=0")
	for i, payload := range []string{
		`{"hookEventName":"PostToolUse","sessionId":"g-camel","cwd":"/work/app","toolName":"run_terminal_command","toolInput":{"command":"make"},` +
			`"toolResult":{"type":"Bash","exit_code":1,"output_for_prompt":"exit: 1\npanic: sql: database is closed\n","output":[112,97,110]}}`,
		`{"hook_event_name":"PostToolUse","session_id":"g-snake","cwd":"/work/app","tool_name":"run_terminal_command","tool_input":{"command":"make"},` +
			`"tool_response":{"type":"Bash","exit_code":1,"output_for_prompt":"exit: 1\npanic: sql: database is closed\n","output":[112,97,110]}}`,
	} {
		var out bytes.Buffer
		if err := runHookToolAfter(os.Getenv("DEJA_INDEX_DIR"), strings.NewReader(payload), &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "CGO_ENABLED=0") {
			t.Errorf("payload %d: no fix pair for grok's failed command:\n%q", i, out.String())
		}
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GROK_HOME", filepath.Join(home, ".grok"))
	if _, err := installGrokAuto("/usr/local/bin/deja", false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(grokHooksPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "PostToolUse") || !strings.Contains(string(b), "hook-tool-after") {
		t.Errorf("grok-auto wires no PostToolUse hook-tool-after:\n%s", b)
	}
}
