package sources

import (
	"strings"
	"testing"
)

// piSession is a pi-shaped transcript in /tmp/proj holding lines after the
// header and the first prompt.
func piSession(lines ...string) string {
	return `{"type":"session","version":3,"id":"s1","timestamp":"2026-10-01T10:00:00.000Z","cwd":"/tmp/proj"}` + "\n" +
		`{"type":"message","id":"u1","timestamp":"2026-10-01T10:00:00.000Z","message":{"role":"user","content":[{"type":"text","text":"fix the retry loop"}]}}` + "\n" +
		strings.Join(lines, "\n") + "\n"
}

func piCall(id, name, args string) string {
	return `{"type":"message","id":"a-` + id + `","timestamp":"2026-10-01T10:00:01.000Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"` + id + `","name":"` + name + `","arguments":` + args + `}]}}`
}

func piResult(id, name, text, details string, failed bool) string {
	isErr := "false"
	if failed {
		isErr = "true"
	}
	return `{"type":"message","id":"r-` + id + `","timestamp":"2026-10-01T10:00:02.000Z","message":{"role":"toolResult","toolCallId":"` + id + `","toolName":"` + name + `","content":[{"type":"text","text":` + vocabJSON(text) + `}],"details":` + details + `,"isError":` + isErr + `}}`
}

// powershell is a built-in tool of the pi-coding-agent Kimchi and Senpi ship,
// beside bash and with the same {command, timeout}; the reader took only bash
// and exec for the shell, so a PowerShell run kept its output and lost the
// command, in a Senpi eval cell too (#4523).
func TestPowerShellRunIsACommand(t *testing.T) {
	for _, kind := range []string{"kimchi", "senpi", "pi"} {
		p := writePiFixture(t, piSession(
			piCall("c0", "powershell", `{"command":"go build ./..."}`),
			piResult("c0", "powershell", "package retry", `{}`, false),
			piCall("c1", "powershell", `{"command":"go vet ./retry"}`),
			piResult("c1", "powershell", "vet: unreachable\n\nCommand exited with code 1", `{}`, true),
		))
		got := commandsOf(parseKindForTest(t, kind, p))
		want := []string{"$ go build ./...  → exit 0", "$ go vet ./retry  → exit 1"}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s: commands = %q, want %q", kind, got, want)
		}
	}
	// Senpi's codemode runs powershell only inside an eval cell.
	p := writePiFixture(t, piSession(
		piCall("e0", "eval", `{"code":"await tool.powershell({command: \"go test ./retry\"})"}`),
		piResult("e0", "eval", `{"text":"ok"}`, `{"toolCalls":[{"name":"powershell","ok":true,"args":{"command":"go test ./retry"}}]}`, false),
	))
	if got := commandsOf(parseKindForTest(t, "senpi", p)); len(got) != 1 || got[0] != "$ go test ./retry" {
		t.Errorf("senpi eval: commands = %q, want the cell's powershell run", got)
	}
}
