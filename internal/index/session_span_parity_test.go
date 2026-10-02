package index

import (
	"path/filepath"
	"testing"
	"time"
)

// spanOf is a session's Started and Updated as the index holds them.
func spanOf(t *testing.T, dir, key string) (started, updated time.Time) {
	t.Helper()
	metas, err := AllMeta(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range metas {
		if m.Harness+":"+m.ID == key {
			return m.Started.UTC(), m.Updated.UTC()
		}
	}
	t.Fatalf("%s is not in the index", key)
	return
}

// sameSpanAsRebuild fails where the row's Started or Updated differs from a
// fresh build of the same stores.
func sameSpanAsRebuild(t *testing.T, inc, key string) {
	t.Helper()
	fresh := inc + "-span"
	parityPass(t, fresh, true)
	s1, u1 := spanOf(t, inc, key)
	s2, u2 := spanOf(t, fresh, key)
	if !s1.Equal(s2) || !u1.Equal(u2) {
		t.Errorf("%s: incremental span %s..%s, rebuild %s..%s", key, s1, u1, s2, u2)
	}
}

// Codex appends records with a time and no message after a conversation's
// last turn, thread_settings_applied among them. A rebuild moved Updated to
// them; the append read no message, returned no session and left Updated
// where it was, so one file gave two rows (#4166).
func TestCodexTailWithNoMessageMovesUpdatedAsARebuildDoes(t *testing.T) {
	root := parityEnv(t, map[string]string{"DEJA_CODEX_ROOT": "codex"})
	const id = "01900000-0000-7000-8000-0000000000d1"
	p := filepath.Join(root, "codex", "sessions", "x", "rollout-2026-06-01T01-00-00-"+id+".jsonl")
	parityWrite(t, p, `{"timestamp":"2026-06-01T01:00:00.000Z","type":"session_meta","payload":{"id":"`+id+`","timestamp":"2026-06-01T01:00:00.000Z","cwd":"/tmp/x","originator":"codex_exec","cli_version":"0.1"}}
{"timestamp":"2026-06-01T01:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"first question"}]}}
{"timestamp":"2026-06-01T01:00:09.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"first answer"}]}}
`, false)
	dir := filepath.Join(root, "idx")
	parityPass(t, dir, true)
	parityWrite(t, p, `{"timestamp":"2026-06-01T05:00:00.000Z","type":"event_msg","payload":{"type":"thread_settings_applied"}}`+"\n", true)
	parityPass(t, dir, false)
	sameSpanAsRebuild(t, dir, "codex:"+id)
	sameAsRebuild(t, dir)
}
