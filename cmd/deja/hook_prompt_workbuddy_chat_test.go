package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vshulcz/deja-vu/internal/index"
)

// WorkBuddy AI starts every chat in a fresh ~/WorkBuddy AI/<yyyy-mm-dd-hh-mm-ss>
// folder. Scoped by that folder, each chat was its own project and the second
// chat never recalled the first (#4735).
func TestWorkBuddySecondChatRecallsTheFirst(t *testing.T) {
	withStatsStores(t)
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, "WorkBuddy AI")
	first := filepath.Join(root, "2026-10-05-14-52-46")
	second := filepath.Join(root, "2026-10-05-15-06-37")
	for _, d := range []string{first, second} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	id := "72784911-4fa0-423d-b43f-685d8a6fa2b7"
	ts := time.Now().Add(-72 * time.Hour).UnixMilli()
	line := func(role, kind, text string) string {
		return `{"id":"` + role + `1","timestamp":` + jsonString(ts) + `,"type":"message","role":"` + role +
			`","content":[{"type":"` + kind + `","text":` + jsonString(text) + `}],"sessionId":"` + id + `","cwd":` + jsonString(first) + "}\n"
	}
	path := filepath.Join(home, ".workbuddy-ai", "projects", "WorkBuddy AI-2026-10-05-14-52-46", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := line("user", "input_text", "what backoff should the payments retry use") +
		line("assistant", "output_text", "We settled on jittered exponential backoff for the payments retry, capped at 30 seconds.")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := index.Ensure(index.DefaultDir(), "", true, nil); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	in := strings.NewReader(`{"session_id":"8407","cwd":` + jsonString(second) +
		`,"hook_event_name":"UserPromptSubmit","prompt":"What backoff did we decide on for the payments retry last time?","client":"WorkBuddy"}`)
	if err := runHookPromptMode(index.DefaultDir(), in, &out, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "jittered exponential backoff") {
		t.Fatalf("the second WorkBuddy chat recalled nothing from the first:\n%q", out.String())
	}
}
