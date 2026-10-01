package sources

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vshulcz/deja-vu/internal/model"
)

// piDialect is the tool vocabulary of pi and the harnesses built on it — omp,
// OpenClaw, gjc, prime, senpi, Kimchi — read off sessions each of them wrote:
// `read`, `edit` and `write` name their file under `path`, an edit carries
// `edits[].oldText/newText` (an older pi put one pair at the top level), and
// the shell is `bash`, or `exec` in OpenClaw, with the line under `command`.
// The reader kept the text and skipped these calls, so files, commands and
// edits were empty for all six (#4113).
var piDialect = toolDialect{
	pathKey:    "path",
	pathTools:  map[string]bool{"read": true, "edit": true, "write": true},
	shellTools: map[string]bool{"bash": true, "exec": true},
	editTools:  map[string]bool{"edit": true, "write": true},
	oldKey:     "oldText",
	newKey:     "newText",
}

// piReader folds pi-shaped lines into one session. It holds what a line needs
// from the lines before it: the header's cwd, which the relative paths in the
// calls resolve against, and the calls still waiting for their result.
type piReader struct {
	s            *model.Session
	useHeaderCwd bool
	cwd          string
	// commandAt is where each shell call's rows sit, by call id, so its
	// result can say how the command ended.
	commandAt map[string][]int
	// hashlineFile is the one file a gjc hashline edit named, by call id: the
	// call carries no replaced text, and the result's diff does.
	hashlineFile map[string]string
	// pending holds what an edit or a write would have changed until its
	// result says it did: a refused edit changed nothing, and restore handed
	// its text back as something that stopped existing. A call whose result
	// never arrives is recorded by finish, as the call alone.
	pending map[string][]model.Message
	order   []string
}

func newPiReader(s *model.Session, useHeaderCwd bool) *piReader {
	return &piReader{s: s, useHeaderCwd: useHeaderCwd, commandAt: map[string][]int{},
		hashlineFile: map[string]string{}, pending: map[string][]model.Message{}}
}

func (r *piReader) add(role, text string, t time.Time) {
	r.s.Messages = append(r.s.Messages, model.Message{Role: role, Text: text, Time: t})
}

// change records an edit span or a written record once the call's result is
// in, or now when the call has no id to be answered under.
func (r *piReader) change(id, role, text string, t time.Time) {
	if id == "" {
		r.add(role, text, t)
		return
	}
	if _, ok := r.pending[id]; !ok {
		r.order = append(r.order, id)
	}
	r.pending[id] = append(r.pending[id], model.Message{Role: role, Text: text, Time: t})
}

// finish records the changes whose result the transcript does not hold yet.
func (r *piReader) finish() {
	for _, id := range r.order {
		r.s.Messages = append(r.s.Messages, r.pending[id]...)
	}
	r.pending, r.order = map[string][]model.Message{}, nil
}

// abs resolves a path the agent gave relative to the session's directory, the
// way the files of every other harness are recorded.
func (r *piReader) abs(p string) string {
	// A leading slash is absolute however the machine reading it spells
	// paths: the session may have been written on another one.
	if p == "" || r.cwd == "" || filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return p
	}
	return filepath.Join(r.cwd, p)
}

// toolCalls records the calls in one assistant message through the shared
// extractors, each call rewritten into the tool_use shape they read.
func (r *piReader) toolCalls(content any, t time.Time) {
	items, _ := content.([]any)
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if typ, _ := m["type"].(string); typ != "toolCall" {
			continue
		}
		name, _ := m["name"].(string)
		args, _ := m["arguments"].(map[string]any)
		id, _ := m["id"].(string)
		if name == "" || args == nil {
			continue
		}
		if in, _ := args["input"].(string); name == "edit" && in != "" && args["path"] == nil {
			r.hashline(id, in, t)
			continue
		}
		in := args
		if p, _ := args["path"].(string); p != "" && r.abs(p) != p {
			in = make(map[string]any, len(args))
			for k, v := range args {
				in[k] = v
			}
			in["path"] = r.abs(p)
		}
		call := []any{map[string]any{"type": "tool_use", "name": name, "input": in}}
		if IndexToolPaths() {
			if p := toolPathsIn(call, piDialect); p != "" {
				r.add(RoleFiles, p, t)
			}
		}
		if IndexEdits() {
			for _, span := range editSpansIn(call, piDialect) {
				r.change(id, RoleEdit, span, t)
			}
		}
		if IndexWrites() {
			for _, w := range wroteRecordsIn(call, piDialect) {
				r.change(id, RoleWrote, w, t)
			}
		}
		if IndexCommands() {
			for _, cmd := range commandsIn(call, piDialect) {
				if id != "" {
					r.commandAt[id] = append(r.commandAt[id], len(r.s.Messages))
				}
				r.add(RoleCommand, cmd, t)
			}
		}
	}
}

// hashline reads gjc's edit, which takes one string rather than a path and a
// span: `§path` starts a file, `≔A..B` replaces the anchored lines, `«A` and
// `»A` insert before and after, and the lines under an op are what it writes.
// The replaced text is not in the call; the result's diff carries it.
func (r *piReader) hashline(id, input string, t time.Time) {
	var files []string
	written := map[string][]string{}
	cur, inOp := "", false
	for _, l := range strings.Split(input, "\n") {
		l = strings.TrimRight(l, "\r")
		switch {
		case strings.HasPrefix(l, "§"):
			cur, inOp = r.abs(strings.TrimSpace(strings.TrimPrefix(l, "§"))), false
			if cur != "" && !strings.ContainsAny(cur, "\n\r") {
				files = append(files, cur)
			}
		case cur == "":
		case strings.HasPrefix(l, "≔"), strings.HasPrefix(l, "«"), strings.HasPrefix(l, "»"):
			inOp = true
		case inOp && strings.TrimSpace(l) != "":
			written[cur] = append(written[cur], l)
		}
	}
	if len(files) == 0 {
		return
	}
	if IndexToolPaths() {
		r.add(RoleFiles, strings.Join(files, "\n"), t)
	}
	if IndexWrites() {
		for _, f := range files {
			if rec := WroteRecord(f, strings.Join(written[f], "\n")); rec != "" {
				r.change(id, RoleWrote, rec, t)
			}
		}
	}
	// With more than one file the diff does not say which lines were whose.
	if len(files) == 1 && id != "" {
		r.hashlineFile[id] = files[0]
	}
}

// toolResult closes a call: a command gets the way it ended, in the marker
// every other harness writes, and a gjc hashline edit gets the lines it
// replaced, read off the result's diff.
func (r *piReader) toolResult(msg map[string]any, t time.Time) {
	id, _ := msg["toolCallId"].(string)
	if id == "" {
		return
	}
	failed, _ := msg["isError"].(bool)
	details, _ := msg["details"].(map[string]any)
	if recs, ok := r.pending[id]; ok {
		delete(r.pending, id)
		if !failed {
			r.s.Messages = append(r.s.Messages, recs...)
		}
	}
	if at, ok := r.commandAt[id]; ok {
		delete(r.commandAt, id)
		mark := ""
		if code, ok := piExitCode(details["exitCode"]); ok {
			mark = "  → exit " + strconv.Itoa(code)
		} else if !failed {
			// pi records no exit code, so only the clean case is stated,
			// as the Claude decoder does; nothing is made up for a failure.
			mark = "  → exit 0"
		}
		for _, i := range at {
			if mark != "" && i < len(r.s.Messages) && !strings.Contains(r.s.Messages[i].Text, "  → exit ") {
				r.s.Messages[i].Text += mark
			}
		}
	}
	if path, ok := r.hashlineFile[id]; ok {
		delete(r.hashlineFile, id)
		diff, _ := details["diff"].(string)
		if failed || diff == "" || !IndexEdits() {
			return
		}
		var removed []string
		for _, l := range strings.Split(diff, "\n") {
			if !strings.HasPrefix(l, "-") || strings.HasPrefix(l, "---") {
				continue
			}
			// "-12|text": the line number and gjc's separator go.
			l = strings.TrimLeft(l[1:], "0123456789")
			l = strings.TrimPrefix(l, "|")
			removed = append(removed, l)
		}
		span := strings.Join(removed, "\n")
		if strings.TrimSpace(span) == "" {
			return
		}
		if len(span) > editSpanMax {
			span = span[:editSpanMax]
		}
		r.add(RoleEdit, path+"\n"+span, t)
	}
}

func piExitCode(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	}
	return 0, false
}
