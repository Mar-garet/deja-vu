package sources

import (
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/model"
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

// changesOf is the files, edit and wrote records of a session, in order.
func changesOf(ss []model.Session) []string {
	var out []string
	for _, s := range ss {
		for _, m := range s.Messages {
			switch m.Role {
			case RoleFiles, RoleEdit, RoleWrote:
				out = append(out, m.Role+" "+m.Text)
			}
		}
	}
	return out
}

// omp and gjc edit in modes besides hashline and apply_patch, and the reader
// took none of their keys: omp's replace is {path, old_string, new_string},
// gjc's {path, edits:[{old_text, new_text}]}, and both patch modes are {path,
// edits:[{op, diff}]}. gjc picks replace for Claude, DeepSeek and Qwen, so
// those users' edits were a file name with nothing behind it (#4524).
func TestOmpAndGjcReplaceAndPatchEdits(t *testing.T) {
	const oldDelay, newDelay = "\tdelay := base", "\tdelay := base * time.Duration(attempt)"
	const newJitter = "\treturn time.Duration(rand.Int63n(int64(base)))"
	const created = "const maxAttempts = 5 // retries before the loop gives up"
	backoff, jitter, limits := "/tmp/proj/backoff.go", "/tmp/proj/jitter.go", "/tmp/proj/limits.go"
	rows := []struct {
		name, kind, args string
		failed           bool
		want             []string
	}{
		{"gjc replace", "gjc", `{"path":"backoff.go","edits":[{"old_text":"\tdelay := base","new_text":"\tdelay := base * time.Duration(attempt)","all":false}]}`, false,
			[]string{"files " + backoff, "edit " + backoff + "\n" + oldDelay, "wrote " + WroteRecord(backoff, newDelay)}},
		{"omp replace", "omp", `{"path":"backoff.go","old_string":"\tdelay := base","new_string":"\tdelay := base * time.Duration(attempt)"}`, false,
			[]string{"files " + backoff, "edit " + backoff + "\n" + oldDelay, "wrote " + WroteRecord(backoff, newDelay)}},
		{"omp replace, several", "omp", `{"path":"backoff.go","edits":[{"old_string":"\tdelay := base","new_string":"\tdelay := base * time.Duration(attempt)"}]}`, false,
			[]string{"files " + backoff, "edit " + backoff + "\n" + oldDelay, "wrote " + WroteRecord(backoff, newDelay)}},
		{"gjc patch", "gjc", `{"path":"jitter.go","edits":[{"op":"update","diff":"@@\n-\treturn 0\n+\treturn time.Duration(rand.Int63n(int64(base)))\n"}]}`, false,
			[]string{"files " + jitter, "edit " + jitter + "\n\treturn 0", "wrote " + WroteRecord(jitter, newJitter)}},
		{"omp patch, two hunks", "omp", `{"path":"jitter.go","edits":[{"op":"update","diff":"@@ func a\n context\n-\treturn 0\n+\treturn time.Duration(rand.Int63n(int64(base)))\n@@ func b\n-\treturn 1\n"}]}`, false,
			[]string{"files " + jitter, "edit " + jitter + "\n\treturn 0", "edit " + jitter + "\n\treturn 1", "wrote " + WroteRecord(jitter, newJitter)}},
		{"omp patch, create", "omp", `{"path":"limits.go","edits":[{"op":"create","diff":"package retry\n\n` + created + `\n"}]}`, false,
			[]string{"files " + limits, "wrote " + WroteRecord(limits, created)}},
		{"omp patch, delete", "omp", `{"path":"limits.go","edits":[{"op":"delete"}]}`, false,
			[]string{"files " + limits}},
		{"gjc replace refused", "gjc", `{"path":"backoff.go","edits":[{"old_text":"\tdelay := base","new_text":"\tdelay := base * time.Duration(attempt)"}]}`, true,
			[]string{"files " + backoff}},
	}
	for _, r := range rows {
		p := writePiFixture(t, piSession(piCall("c0", "edit", r.args), piResult("c0", "edit", "Updated", `{}`, r.failed)))
		got := changesOf(parseKindForTest(t, r.kind, p))
		if strings.Join(got, "|") != strings.Join(r.want, "|") {
			t.Errorf("%s: records = %q, want %q", r.name, got, r.want)
		}
	}
}
