package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/index"
)

// A skill drives deja through the shell, and the shell had no answer an agent
// could read: `deja search --json` ran to megabytes. `deja recall` prints the
// page the MCP tool serves, word for word (#4781).
func TestRecallOnTheCommandLineIsTheMCPAnswer(t *testing.T) {
	hermeticEnv(t)
	proj := filepath.Join(os.Getenv("DEJA_CLAUDE_ROOT"), "-work-app")
	writeClaudeFixture(t, filepath.Join(proj, "s1.jsonl"), "s1", []string{
		`{"type":"user","sessionId":"s1","cwd":"/work/app","timestamp":"2026-09-01T10:00:00Z","message":{"role":"user","content":"why did the zonkomatic export break"}}`,
		`{"type":"assistant","sessionId":"s1","cwd":"/work/app","timestamp":"2026-09-01T10:01:00Z","message":{"role":"assistant","content":"the zonkomatic export needs CRLF line endings for the finance importer"}}`,
	})
	dir := index.DefaultDir()
	if err := index.Ensure(dir, "", true, nil); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runRecall(dir, []string{"zonkomatic", "export"}, &out); err != nil {
		t.Fatal(err)
	}
	mcp, err := callMCPTool(dir, "deja", json.RawMessage(`{"mode":"recall","q":"zonkomatic export"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "CRLF") {
		t.Fatalf("the command served nothing from the fixture:\n%s", out.String())
	}
	// The MCP answer may carry the once-per-session environment block after
	// the frame; the page itself has to be the same.
	if !strings.HasPrefix(mcp, strings.TrimRight(out.String(), "\n")) {
		t.Errorf("the command and the tool disagree.\ncli:\n%s\nmcp:\n%s", out.String(), mcp)
	}
	if len(out.String()) > recallMCPBudget+512 {
		t.Errorf("answered %d bytes, past the tool's budget", out.Len())
	}

	// The same answer whichever way the flag is written, and a query may come
	// after -- even when it starts with a dash.
	for _, args := range [][]string{{"zonkomatic", "--limit=1", "export"}, {"--limit", "1", "--", "zonkomatic", "export"}} {
		var b bytes.Buffer
		if err := runRecall(dir, args, &b); err != nil || !strings.Contains(b.String(), "CRLF") {
			t.Errorf("recall %q: %v\n%s", args, err, b.String())
		}
	}
	if err := runRecall(dir, []string{"--", "--zonkomatic"}, &bytes.Buffer{}); err != nil {
		t.Errorf("a query after -- that starts with a dash was refused: %v", err)
	}

	for _, bad := range [][]string{nil, {"--limit"}, {"--limit", "0", "x"}, {"--nope", "x"}, {"--project", "", "x"}} {
		if err := runRecall(dir, bad, &bytes.Buffer{}); err == nil {
			t.Errorf("recall %q was accepted", bad)
		}
	}
}
