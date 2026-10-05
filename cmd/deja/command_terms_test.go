package main

import (
	"strings"
	"testing"
)

// Command text is not prose. The clause cut that lets a question lead past an
// instruction (#4751) would lead "cd repo; go test" with the test and drop the
// directory, and cut a quoted commit message at its full stop. The tool hook
// reads a command exactly as it did before that change.
func TestToolHookReadsACommandWithoutCuttingClauses(t *testing.T) {
	cases := []struct{ cmd, want string }{
		{
			`cd quokkabloom-service; go test ./retry/... -run TestDialTimeout -count=1 -v`,
			"quokkabloom-service test retry testdialtimeout",
		},
		{
			`go test ./internal/zebraquux/fetcher -run TestFetchTimeout -count=1 -race; echo done`,
			"test internal/zebraquux/fetcher testfetchtimeout echo done",
		},
		{
			`git commit -m "fix: zebraquux fetcher retries. quokkabloom dial timeout raised" internal/zebraquux/fetch.go`,
			"git commit zebraquux fetcher retries quokkabloom",
		},
	}
	for _, c := range cases {
		if got := strings.Join(toolCommandTerms(c.cmd), " "); got != c.want {
			t.Errorf("toolCommandTerms(%q) = %q, want %q", c.cmd, got, c.want)
		}
	}
}
