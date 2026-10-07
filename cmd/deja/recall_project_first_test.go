package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/index"
	"github.com/vshulcz/deja-vu/internal/sources"
)

// The prompt hook ranks inside the project the agent works in and recall
// ranked the whole machine, so an agent that asked recall itself got another
// project's sessions above its own project's answer. Here sixty sessions from
// elsewhere say the query's words over and over, and the one session of this
// project that answers says them once.
func TestRecallPutsThisProjectsAnswerFirst(t *testing.T) {
	hermeticEnv(t)
	root := os.Getenv("DEJA_CLAUDE_ROOT")
	work := t.TempDir()
	ledgerd := filepath.Join(work, "src", "ledgerd")
	if err := os.MkdirAll(ledgerd, 0o755); err != nil {
		t.Fatal(err)
	}
	line := func(sid, cwd, role, text string) string {
		b, _ := json.Marshal(map[string]any{"type": role, "sessionId": sid, "cwd": cwd, "timestamp": "2026-09-10T10:00:00Z",
			"message": map[string]any{"role": role, "content": text}})
		return string(b)
	}
	writeClaudeFixture(t, filepath.Join(root, sources.ClaudeProjectName(ledgerd), "dana.jsonl"), "dana", []string{
		line("dana", ledgerd, "user", "who owns the bulk import now that Theo is gone"),
		line("dana", ledgerd, "assistant", "pgx and bulk-import reviews go to Dana Whitfield, not Theo Brandt."),
	})
	other := filepath.Join(work, "src", "other")
	for i := range 60 {
		sid := fmt.Sprintf("o%02d", i)
		writeClaudeFixture(t, filepath.Join(root, sources.ClaudeProjectName(other), sid+".jsonl"), sid, []string{
			line(sid, other, "user", fmt.Sprintf("service %d: open the pgx upgrade PR %d and find a reviewer; the owner wants review today", i, 100+i)),
			line(sid, other, "assistant", fmt.Sprintf("PR %d pgx upgrade: reviewer assigned by the owner of service %d, review requested, pgx upgrade review pending", 100+i, i)),
		})
	}
	dir := index.DefaultDir()
	if err := index.Ensure(dir, "", true, nil); err != nil {
		t.Fatal(err)
	}
	firstRow := regexp.MustCompile(`\n1\. \[[^\]]*\] (\S+)`)
	ask := func(args string) string {
		t.Helper()
		text, err := callMCPTool(dir, "deja", json.RawMessage(args))
		if err != nil {
			t.Fatal(err)
		}
		return text
	}
	const q = `{"mode":"recall","q":"pgx upgrade PR reviewer owner review"}`

	t.Chdir(ledgerd)
	text := ask(q)
	m := firstRow.FindStringSubmatch(text)
	if m == nil || !strings.HasSuffix(m[1], "ledgerd") || !strings.Contains(text, "Dana") {
		t.Fatalf("from inside ledgerd, its own answer is not first:\n%s", firstLines(text, 12))
	}

	// Control: asked from a directory that is no project, the sixty win, so
	// the order above is the project's doing and not the fixture's.
	t.Chdir(t.TempDir())
	if m := firstRow.FindStringSubmatch(ask(q)); m == nil || strings.HasSuffix(m[1], "ledgerd") {
		t.Fatalf("outside any project ledgerd still led, so the test above proves nothing: %v", m)
	}

	// A project the caller names is a filter, as the tool describes it.
	t.Chdir(ledgerd)
	if named := ask(`{"mode":"recall","q":"pgx upgrade PR reviewer owner review","project":"src/other"}`); strings.Contains(named, "Dana") {
		t.Fatalf("project src/other still served ledgerd's session:\n%s", firstLines(named, 12))
	}
}
