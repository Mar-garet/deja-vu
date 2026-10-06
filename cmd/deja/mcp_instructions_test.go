package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Hosts that honour the field (copilot, behind --allow-all-mcp-server-instructions)
// read it straight out of the initialize result, so keep it wired to the handler.
func TestMCPInitializeCarriesInstructions(t *testing.T) {
	res, code, msg := handleMCP(t.TempDir(), rpcRequest{Method: "initialize"})
	if code != 0 {
		t.Fatalf("initialize failed: %d %s", code, msg)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result is %T, want map", res)
	}
	got, _ := m["instructions"].(string)
	if !strings.Contains(got, "recall") {
		t.Fatalf("instructions do not mention the recall tool: %q", got)
	}
}

func TestMCPInstructionsReportBuildInProgress(t *testing.T) {
	dir := t.TempDir()
	st := warmupStatus{Phase: "indexing", Total: 4, Done: 1, Updated: time.Now().UnixNano()}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(warmupStatusPath(dir)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(warmupStatusPath(dir), b, 0o644); err != nil {
		t.Fatal(err)
	}
	got := mcpInstructions(dir, "")
	if !strings.Contains(got, "indexing 25%") {
		t.Fatalf("instructions omit build progress: %q", got)
	}
	_ = os.Remove(warmupStatusPath(dir))
	if got := mcpInstructions(dir, ""); strings.Contains(got, "still building") {
		t.Fatalf("instructions claim a build with no status file: %q", got)
	}
}

// Claude Code defers the tool behind ToolSearch, and only Claude Code is told
// how to load it — the hint names a tool other hosts do not have (#4760). Both
// spellings it has sent are covered, and the select names the plugin's tool too.
func TestMCPInitializeTellsClaudeCodeToLoadTheDeferredTool(t *testing.T) {
	for _, client := range []string{"claude-code", "Claude Code"} {
		res, _, _ := handleMCP(t.TempDir(), rpcRequest{Method: "initialize", Params: json.RawMessage(`{"clientInfo":{"name":"` + client + `","version":"2.1.290"}}`)})
		got, _ := res.(map[string]any)["instructions"].(string)
		for _, want := range []string{"ToolSearch", "mcp__deja__deja", "mcp__plugin_deja-vu_deja__deja"} {
			if !strings.Contains(got, want) {
				t.Errorf("client %q: instructions miss %q: %q", client, want, got)
			}
		}
	}
	for _, params := range []string{`{"clientInfo":{"name":"opencode"}}`, `{}`, `{"clientInfo":7}`} {
		res, code, _ := handleMCP(t.TempDir(), rpcRequest{Method: "initialize", Params: json.RawMessage(params)})
		if code != 0 {
			t.Fatalf("%s: initialize failed with %d", params, code)
		}
		got, _ := res.(map[string]any)["instructions"].(string)
		if strings.Contains(got, "ToolSearch") {
			t.Errorf("%s: a host without ToolSearch was told to use it: %q", params, got)
		}
		if !strings.Contains(got, "Before you change code") {
			t.Errorf("%s: instructions lost the change trigger: %q", params, got)
		}
	}
}
