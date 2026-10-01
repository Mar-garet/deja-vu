package sources

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vshulcz/deja-vu/internal/model"
)

// hermesDialect is Hermes' tool vocabulary, read off its own schemas in
// tools/file_tools.py and tools/terminal_tool.py: the shell is `terminal`, the
// file tools take `path`, and patch in its default replace mode takes
// old_string/new_string like Claude's Edit. search_files' path is a directory
// to search, not a file the session touched, so it is left out.
var hermesDialect = toolDialect{
	pathKey:   "path",
	pathTools: map[string]bool{"read_file": true, "write_file": true, "patch": true},
	shellTool: "terminal",
	editTools: map[string]bool{"patch": true, "write_file": true},
}

// hermesSession is one session being read, with what its rows refer back to.
type hermesSession struct {
	s model.Session
	// commandAt is which messages hold the commands a call ran, by call id, so
	// the exit code on the `tool` row lands on its command.
	commandAt map[string][]int
	// In-place compaction archives the session's rows (compacted=1) and
	// writes the kept head and tail again as live rows, under the same call
	// ids and with results replaced by stubs. A live row that repeats an
	// archived one is that copy. Within one side an id can repeat honestly:
	// with no provider id Hermes derives one from the call's name and
	// arguments, so the same command run twice carries the same id.
	archivedCall, archivedResult, archivedProse map[string]bool
	dropped                                     bool
}

func newHermesSession(s model.Session) *hermesSession {
	return &hermesSession{s: s, commandAt: map[string][]int{},
		archivedCall: map[string]bool{}, archivedResult: map[string]bool{}, archivedProse: map[string]bool{}}
}

// hermesCompactionSummary opens the message compaction writes between the
// head and tail it keeps (agent/context_compressor.py SUMMARY_PREFIX).
const hermesCompactionSummary = "[CONTEXT COMPACTION"

func (h *hermesSession) row(r map[string]any) {
	t := hermesTime(r["timestamp"])
	role := str(r["role"])
	txt := hermesText(str(r["content"]))
	archived := fmt.Sprint(r["compacted"]) == "1"
	if role == "tool" {
		h.result(txt, str(r["tool_name"]), str(r["tool_call_id"]), archived, t)
		return
	}
	if txt != "" {
		key := role + "\x00" + txt
		switch {
		case strings.HasPrefix(txt, hermesCompactionSummary):
			// A restatement of turns the store still holds, kept where
			// `--role summary` reaches it and ordinary search does not.
			role = RoleSummary
		case archived:
			h.archivedProse[key] = true
		case h.archivedProse[key]:
			txt = ""
		}
	}
	if txt != "" {
		h.s.Touch(t)
		h.s.Messages = append(h.s.Messages, model.Message{Role: role, Text: capParsedMessage(txt), Time: t})
	}
	if role == "assistant" {
		h.calls(str(r["tool_calls"]), archived, t)
	}
}

// repeat reports whether a call or result id is compaction's copy of an
// archived one, and notes the archived ones.
func repeat(seen map[string]bool, id string, archived bool) bool {
	if id == "" {
		return false
	}
	if archived {
		seen[id] = true
		return false
	}
	return seen[id]
}

// done hands the session back without the command records of runs that never
// happened.
func (h *hermesSession) done() model.Session {
	if h.dropped {
		kept := h.s.Messages[:0]
		for _, m := range h.s.Messages {
			if m.Role != "" {
				kept = append(kept, m)
			}
		}
		h.s.Messages = kept
	}
	return h.s
}

// calls turns an assistant row's OpenAI-style tool_calls into work records.
// The column is json.dumps of what the caller passed, so a single call can be
// an object rather than a list (hermes_state.py append_message).
func (h *hermesSession) calls(raw string, archived bool, t time.Time) {
	if raw == "" {
		return
	}
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return
	}
	calls, _ := v.([]any)
	if m, ok := v.(map[string]any); ok {
		calls = []any{m}
	}
	for _, c := range calls {
		id := ""
		if m, ok := c.(map[string]any); ok {
			id, _ = m["id"].(string)
		}
		if repeat(h.archivedCall, id, archived) {
			continue
		}
		blocks := reasonixToolUses([]any{c})
		if len(blocks) == 0 {
			continue
		}
		var records []model.Message
		if IndexToolPaths() {
			if p := toolPathsIn(blocks, hermesDialect); p != "" {
				records = append(records, model.Message{Role: RoleFiles, Text: p, Time: t})
			}
		}
		if IndexWrites() {
			for _, w := range wroteRecordsIn(blocks, hermesDialect) {
				records = append(records, model.Message{Role: RoleWrote, Text: w, Time: t})
			}
		}
		if IndexEdits() {
			for _, span := range editSpansIn(blocks, hermesDialect) {
				records = append(records, model.Message{Role: RoleEdit, Text: span, Time: t})
			}
		}
		records = append(records, hermesPatchRecords(blocks[0], t)...)
		var at []int
		if IndexCommands() {
			for _, cmd := range commandsIn(blocks, hermesDialect) {
				at = append(at, len(h.s.Messages)+len(records))
				records = append(records, model.Message{Role: RoleCommand, Text: cmd, Time: t})
			}
		}
		if len(records) == 0 {
			continue
		}
		if id != "" && len(at) > 0 {
			h.commandAt[id] = at
		}
		h.s.Touch(t)
		h.s.Messages = append(h.s.Messages, records...)
	}
}

// result records a `tool` row. Every Hermes tool answers in JSON. What it
// says is usually in output (terminal), content (read_file), diff (patch),
// matches_text (search_files) or error, and then the rest is bookkeeping;
// other tools nest it — search_files' files, web_search's data, the results
// of web_extract and delegate_task — and then every string in it is kept.
// Keys starting with _ are hints to the model and never kept.
//
// terminal's exit_code rides on the command it answers. -1 is a command that
// never ran — denied, blocked, waiting on approval, invalid, failed to start
// (tools/terminal_tool.py) — so its command record is dropped: kept, it reads
// as a run that happened, and `→ exit -1` is not a status the index reads.
// Why it did not run stays in the tool output.
func (h *hermesSession) result(txt, tool, callID string, archived bool, t time.Time) {
	if repeat(h.archivedResult, callID, archived) {
		return
	}
	if txt == "" || hermesCompressorStub(txt, tool) {
		return
	}
	if strings.HasPrefix(txt, "{") {
		var res map[string]any
		if json.Unmarshal([]byte(txt), &res) == nil {
			if code, ok := res["exit_code"]; ok {
				h.exit(callID, exitCode(code))
			}
			txt = hermesResultText(res)
		}
	}
	if txt == "" || !IndexToolOutput() {
		return
	}
	h.s.Touch(t)
	h.s.Messages = append(h.s.Messages, model.Message{Role: RoleToolOutput, Text: capParsedMessage(txt), Time: t})
}

func (h *hermesSession) exit(callID string, code int) {
	// Other negative codes are a process killed by a signal after it started
	// (tools/environments/base.py); it ran, and the marker reads digits only.
	if code == 0 || callID == "" || (code < 0 && code != -1) {
		return
	}
	for _, i := range h.commandAt[callID] {
		if i >= len(h.s.Messages) || h.s.Messages[i].Role != RoleCommand {
			continue
		}
		if code == -1 {
			h.s.Messages[i].Role = ""
			h.dropped = true
			continue
		}
		h.s.Messages[i].Text += fmt.Sprintf("  → exit %d", code)
	}
	delete(h.commandAt, callID)
}

var hermesResultKeys = []string{"output", "content", "diff", "matches_text", "error"}

func hermesResultText(res map[string]any) string {
	var parts []string
	for _, k := range hermesResultKeys {
		parts = hermesLeaves(res[k], true, parts)
	}
	if len(parts) == 0 {
		parts = hermesLeaves(res, false, nil)
	}
	return strings.Join(parts, "\n")
}

// hermesLeaves appends the strings in v, in key order so a record reads the
// same on every parse. Numbers count only where the key says the value is
// the result — {"output": 42} — and not as the counts and indexes beside it.
func hermesLeaves(v any, numbers bool, out []string) []string {
	switch e := v.(type) {
	case string:
		if s := strings.TrimSpace(e); s != "" {
			out = append(out, s)
		}
	case float64:
		if numbers {
			out = append(out, strconv.FormatFloat(e, 'f', -1, 64))
		}
	case []any:
		for _, it := range e {
			out = hermesLeaves(it, numbers, out)
		}
	case map[string]any:
		keys := make([]string, 0, len(e))
		for k := range e {
			if !strings.HasPrefix(k, "_") {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = hermesLeaves(e[k], numbers, out)
		}
	}
	return out
}

// hermesCompressorStub reports text Hermes' context compressor wrote in place
// of a result it cleared (agent/context_compressor.py): a fixed placeholder,
// or a one-line summary that opens with the tool's name in brackets.
func hermesCompressorStub(txt, tool string) bool {
	for _, p := range []string{
		"[Old tool output cleared to save context space]",
		"[Duplicate tool output",
		"[Result from earlier conversation",
		"[screenshot removed",
	} {
		if strings.HasPrefix(txt, p) {
			return true
		}
	}
	return tool != "" && strings.HasPrefix(txt, "["+tool+"]")
}

// hermesContentJSON is the prefix Hermes stores structured content under —
// a multimodal message's list of parts (hermes_state.py _encode_content).
const hermesContentJSON = "\x00json:"

// hermesText is a row's content as text: the text parts of a multimodal
// message, never the base64 of its images.
func hermesText(content string) string {
	if !strings.HasPrefix(content, hermesContentJSON) {
		return strings.TrimSpace(content)
	}
	var v any
	if json.Unmarshal([]byte(content[len(hermesContentJSON):]), &v) != nil {
		return ""
	}
	parts, _ := v.([]any)
	if m, ok := v.(map[string]any); ok {
		parts = []any{m}
	}
	var out []string
	for _, p := range parts {
		switch e := p.(type) {
		case string:
			out = append(out, e)
		case map[string]any:
			if e["type"] != nil && e["type"] != "text" {
				continue
			}
			if s, _ := e["text"].(string); s != "" {
				out = append(out, s)
			} else if s, _ := e["text_summary"].(string); s != "" {
				out = append(out, s)
			}
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// hermesPatchHeader and hermesPatchMove are the file headers as Hermes' own
// V4A parser matches them (tools/patch_parser.py): the space after *** is
// optional, and Move File names two paths.
var (
	hermesPatchHeader = regexp.MustCompile(`^\*\*\*\s*(Update|Add|Delete)\s+File:\s*(.+)$`)
	hermesPatchMove   = regexp.MustCompile(`^\*\*\*\s*Move\s+File:\s*(.+?)\s*->\s*(.+)$`)
	hermesPatchEnd    = regexp.MustCompile(`^\*\*\*\s*End Patch`)
)

// hermesPatchRecords reads patch in its V4A mode, where the call carries a
// multi-file patch instead of a path and a span. The headers are rewritten to
// the spelling the apply_patch helpers read, so the spans come out the same.
func hermesPatchRecords(block any, t time.Time) []model.Message {
	b, _ := block.(map[string]any)
	if name, _ := b["name"].(string); name != "patch" {
		return nil
	}
	in, _ := b["input"].(map[string]any)
	patch, _ := in["patch"].(string)
	if patch == "" {
		return nil
	}
	var files []string
	seen := map[string]bool{}
	file := func(f string) {
		if f = strings.TrimSpace(f); f != "" && !seen[f] {
			seen[f] = true
			files = append(files, f)
		}
	}
	lines := strings.Split(patch, "\n")
	// After Delete File and Move File the parser has no current file and
	// passes over every line up to the next header, so those lines belong
	// to no file here either.
	orphan := false
	for i, line := range lines {
		switch {
		case hermesPatchHeader.MatchString(line):
			m := hermesPatchHeader.FindStringSubmatch(line)
			file(m[2])
			lines[i] = "*** " + m[1] + " File: " + strings.TrimSpace(m[2])
			orphan = m[1] == "Delete"
		case hermesPatchMove.MatchString(line):
			m := hermesPatchMove.FindStringSubmatch(line)
			file(m[1])
			file(m[2])
			// A header of its own ends the file before it.
			lines[i] = "*** Delete File: " + strings.TrimSpace(m[1])
			orphan = true
		case hermesPatchEnd.MatchString(line):
			lines[i] = "*** End Patch"
		case orphan:
			lines[i] = ""
		}
	}
	patch = strings.Join(lines, "\n")
	var out []model.Message
	if IndexToolPaths() && len(files) > 0 {
		out = append(out, model.Message{Role: RoleFiles, Text: strings.Join(files, "\n"), Time: t})
	}
	if IndexWrites() {
		for _, rec := range addedLinesOfPatch(patch) {
			out = append(out, model.Message{Role: RoleWrote, Text: rec, Time: t})
		}
	}
	if IndexEdits() {
		for _, span := range patchSpans(patch) {
			out = append(out, model.Message{Role: RoleEdit, Text: span, Time: t})
		}
	}
	return out
}
