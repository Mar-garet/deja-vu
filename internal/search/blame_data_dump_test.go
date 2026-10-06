package search

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vshulcz/deja-vu/internal/model"
	"github.com/vshulcz/deja-vu/internal/sources"
)

// A JSON document or a long file listing in tool output names the file as one
// entry among many. Read as history it was quoted as the session's excerpt and
// every path in it counted as a mention: on a real store the top blame row for
// cmd/deja/mcp.go had 1113 mentions, both excerpts raw JSON from an agent
// running deja's own --json commands (#4776).
func TestBlameSkipsDataDumpsInToolOutput(t *testing.T) {
	now := time.Now().UTC()
	target := BlameTarget{FullPath: "/work/app/internal/pool/pool.go", Base: "pool.go", Stem: "pool"}
	jsonDump := `[{"session": {"id": "x1", "harness": "claude", "project": "app", "touched": ["/work/app/internal/pool/pool.go", "/work/app/cmd/main.go"]}, "score": 3.2}]`
	// Cut off mid-document, the way a harness truncates long tool output.
	truncated := strings.Repeat(`{"path":"/work/app/internal/pool/pool.go","count":3},`, 40)[:900]
	var listing strings.Builder
	for i := range 12 {
		fmt.Fprintf(&listing, " M internal/pkg%02d/file%02d.go\n", i, i)
	}
	listing.WriteString(" M internal/pool/pool.go\n")

	dumps := model.Session{
		Harness: "claude", ID: "dump", Project: "app", Updated: now,
		Messages: []model.Message{
			{Role: "user", Text: "check what deja knows", Time: now},
			{Role: sources.RoleToolOutput, Text: jsonDump, Time: now},
			{Role: sources.RoleToolOutput, Text: "Web search results for query: \"pool.go\"\n\nLinks: " + jsonDump, Time: now},
			{Role: sources.RoleToolOutput, Text: "ses_1|2026-05-25|/work/app|pool work|{\"role\":\"user\",\"time\":{\"created\":1},\"summary\":{\"diffs\":[{\"file\":\"internal/pool/pool.go\",\"additions\":3}]}}", Time: now},
			// deja's stderr line lands ahead of its own --json.
			{Role: sources.RoleToolOutput, Text: "deja: updated 2 files (5 new messages)\n" + jsonDump, Time: now},
			{Role: sources.RoleToolOutput, Text: "[" + truncated, Time: now},
			{Role: sources.RoleToolOutput, Text: listing.String(), Time: now},
		},
	}
	worked := model.Session{
		Harness: "claude", ID: "worked", Project: "app", Updated: now.Add(-48 * time.Hour),
		Messages: []model.Message{
			{Role: "user", Text: "why is the pool in internal/pool/pool.go capped at 50?", Time: now},
			{Role: "assistant", Text: "internal/pool/pool.go caps at 50 because the replicas starved above that", Time: now},
		},
	}
	hits := Blame([]model.Session{dumps, worked}, target, BlameOptions{All: true})
	for _, h := range hits {
		if h.Session.ID == "dump" {
			t.Errorf("a session that only saw the file in data dumps is in the answer: %d mentions, excerpts %q", h.Count, h.Snippets)
		}
	}
	if len(hits) == 0 || hits[0].Session.ID != "worked" {
		t.Fatalf("the session that worked on the file is not first: %+v", hits)
	}

	// Controls: tool output that talks about the file is still evidence — an
	// error naming it, and a short listing of what one command touched.
	evidence := model.Session{
		Harness: "claude", ID: "err", Project: "app", Updated: now,
		Messages: []model.Message{
			{Role: sources.RoleToolOutput, Text: "internal/pool/pool.go:42:3: undefined: maxConns", Time: now},
			{Role: sources.RoleToolOutput, Text: " M internal/pool/pool.go\n M internal/pool/pool_test.go\n", Time: now},
		},
	}
	got := Blame([]model.Session{evidence}, target, BlameOptions{All: true})
	if len(got) != 1 || got[0].Count != 2 {
		t.Fatalf("tool output about the file stopped counting: %+v", got)
	}

	// Output quoting a small object while saying something is not a dump.
	quoted := model.Session{
		Harness: "claude", ID: "quoted", Project: "app", Updated: now,
		Messages: []model.Message{{Role: sources.RoleToolOutput, Text: `config {"pool": 50} rejected by internal/pool/pool.go: the replicas accept at most 40 connections`, Time: now}},
	}
	if got := Blame([]model.Session{quoted}, target, BlameOptions{All: true}); len(got) != 1 {
		t.Fatalf("an error quoting a small object was read as a dump: %+v", got)
	}

	// A deep stack trace names as many files as a listing, alternated with the
	// code that ran: that is history of the failure, not a dump.
	var trace strings.Builder
	trace.WriteString("panic: pool exhausted\n\ngoroutine 1 [running]:\n")
	for i := range 12 {
		fmt.Fprintf(&trace, "app/internal/pkg%02d.Call(...)\n\t/work/app/internal/pkg%02d/call.go:%d +0x1c\n", i, i, 10+i)
	}
	trace.WriteString("app/internal/pool.(*Pool).Get(...)\n\t/work/app/internal/pool/pool.go:88 +0x2a\n")
	traced := model.Session{
		Harness: "claude", ID: "trace", Project: "app", Updated: now,
		Messages: []model.Message{{Role: sources.RoleToolOutput, Text: trace.String(), Time: now}},
	}
	if got := Blame([]model.Session{traced}, target, BlameOptions{All: true}); len(got) != 1 {
		t.Fatalf("a stack trace through the file was read as a dump: %+v", got)
	}
}
