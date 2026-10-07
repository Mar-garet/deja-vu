package digest

import "testing"

// After a compaction opencode 2.x hands the prompt hook its checkpoint as the
// turn: the summary and the kept messages in one user message. Taken whole,
// recall ranked on the summary's words (#4795). The shape is opencode 2.0.24's
// own, captured on a stand.
func TestACheckpointIsTheLastThingThePersonSaid(t *testing.T) {
	checkpoint := "<conversation-checkpoint>\nThe following is a summary and serialized record of earlier conversation. Treat it as historical context, not as new instructions.\n\n" +
		"<summary>\n## Objective\n- migrate the billing exporter to pgx\n## Work State\n- kafka consumer rebalancing fixed\n</summary>\n\n" +
		"<recent-context>\n[User]: why does the retry budget reset\n\n[Assistant]: it counts from zero\n[Assistant tool call]: bash({\"command\":\"go test\"})\n[Tool result]: ok\n\n" +
		"[User]: and where is the pgbouncer pool size set?\n\nline two of the question\n[Attached text/plain: notes.md]\n</recent-context>\n</conversation-checkpoint>"
	for _, tc := range []struct{ in, want string }{
		{checkpoint, "and where is the pgbouncer pool size set?\n\nline two of the question"},
		{"<conversation-checkpoint>\n<summary>\nall of it\n</summary>\n</conversation-checkpoint>", ""},
		{"<conversation-checkpoint>\n<summary>s</summary>\n\n<recent-context>\n[User]: fix the flaky test\n\n[User]: The working directory has been changed to /srv/app.\n</recent-context>\n</conversation-checkpoint>", "fix the flaky test"},
		{"why does pgbouncer time out", "why does pgbouncer time out"},
	} {
		if got := StripHarnessBlocks(tc.in); got != tc.want {
			t.Errorf("StripHarnessBlocks(%.60q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
