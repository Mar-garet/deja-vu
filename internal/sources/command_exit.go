package sources

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/vshulcz/deja-vu/internal/model"
)

// commandExits joins a command record to the result that closes its call. The
// call and its result are separate parts, often in separate messages, and
// only the result says how the command ended; a reader that emitted the two
// without joining them stored every failure as a bare `$ cmd`, which reads the
// same as a run whose result never arrived (#4496, #4502, #4505, #4507).
type commandExits map[string][]int

// note files the command records appended to msgs from index from on under the
// calls they were read from, in order. calls is what commandCallsIn read off
// the same parts, so the two line up one to one; when they do not, nothing is
// noted rather than a status pinned to the wrong command.
func (c commandExits) note(msgs []model.Message, from int, calls []claudeCommand) {
	var at []int
	for i := from; i < len(msgs); i++ {
		if msgs[i].Role == RoleCommand {
			at = append(at, i)
		}
	}
	if len(at) != len(calls) {
		return
	}
	for k, i := range at {
		if calls[k].ID != "" {
			c[calls[k].ID] = append(c[calls[k].ID], i)
		}
	}
}

// stamp marks the commands of call id with the code their result reported, in
// the marker every other harness writes. cmd, when set, picks the one command
// of a batch the code belongs to: the first of them not stamped yet, since a
// batch that runs a command twice reports each run in order.
func (c commandExits) stamp(msgs []model.Message, id, cmd string, code int) {
	for _, i := range c[id] {
		if i >= len(msgs) || strings.Contains(msgs[i].Text, "  → exit ") {
			continue
		}
		if cmd != "" && msgs[i].Text != "$ "+cmd {
			continue
		}
		msgs[i].Text += fmt.Sprintf("  → exit %d", code)
		if cmd != "" {
			return
		}
	}
}

// commandCallsIn is commandsIn with the id of the call each command came from,
// so a reader can stamp the outcome when the result arrives.
func commandCallsIn(v any, d toolDialect) []claudeCommand {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []claudeCommand
	for _, it := range items {
		name, in, ok := toolPart(it, d)
		if !ok || !d.isShellTool(name) {
			continue
		}
		id := ""
		if m, ok := it.(map[string]any); ok {
			id, _ = m["id"].(string)
		}
		for _, cmd := range commandStrings(in, d) {
			if !worthIndexing(cmd) {
				continue
			}
			out = append(out, claudeCommand{ID: id, Text: "$ " + cmd})
		}
	}
	return out
}

// statusCode reads N off a status line a harness writes around a command's
// output, "<prefix>N<suffix>": Claude's "Exit code 1", pi's and goose's
// "Command exited with code 1", Cline's "Command failed with exit code 1.".
// The line must be the whole status, so output that merely mentions an exit
// code is not taken for one.
func statusCode(line, prefix, suffix string) (int, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), prefix)
	if !ok {
		return 0, false
	}
	rest, ok = strings.CutSuffix(rest, suffix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	return n, err == nil && rest != "" && rest[0] != '+' && rest[0] != '-'
}

// lastLine is firstLine from the other end, where pi and goose put the status
// they add after a command's output.
func lastLine(s string) string {
	s = strings.TrimRight(s, " \t\r\n")
	return strings.TrimSpace(s[strings.LastIndexByte(s, '\n')+1:])
}
