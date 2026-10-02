package main

import (
	"encoding/json"
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
