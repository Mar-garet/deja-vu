package sources

import (
	"encoding/json"
	"fmt"
	"regexp"
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
	// Compaction writes the kept tail again under the same call ids, its
	// results replaced by stubs; the first call and result under an id are
	// the real ones.
	seenCall, seenResult map[string]bool
	dropped              bool
}

func newHermesSession(s model.Session) *hermesSession {
	return &hermesSession{s: s, commandAt: map[string][]int{}, seenCall: map[string]bool{}, seenResult: map[string]bool{}}
}

func (h *hermesSession) row(r map[string]any) {
	t := hermesTime(r["timestamp"])
	role := str(r["role"])
	txt := hermesText(str(r["content"]))
	if role == "tool" {
		h.result(txt, str(r["tool_name"]), str(r["tool_call_id"]), t)
		return
	}
	if txt != "" {
		h.s.Touch(t)
		h.s.Messages = append(h.s.Messages, model.Message{Role: role, Text: capParsedMessage(txt), Time: t})
	}
	if role == "assistant" {
		h.calls(str(r["tool_calls"]), t)
	}
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
func (h *hermesSession) calls(raw string, t time.Time) {
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
		if id != "" {
			if h.seenCall[id] {
				continue
			}
			h.seenCall[id] = true
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

// result records a `tool` row. Every Hermes tool answers in JSON, and what it
// says is in output (terminal), content (read_file), diff (patch),
// matches_text (search_files) or error; the rest is bookkeeping, and keys
// starting with _ are hints to the model. A result with none of those — a
// write_file's byte count — is not kept.
//
// terminal's exit_code rides on the command it answers. -1 is a command that
// never ran — denied, blocked, waiting on approval, invalid, failed to start
// (tools/terminal_tool.py) — so its command record is dropped: kept, it reads
// as a run that happened, and `→ exit -1` is not a status the index reads.
// Why it did not run stays in the tool output.
func (h *hermesSession) result(txt, tool, callID string, t time.Time) {
	if callID != "" {
		if h.seenResult[callID] {
			return
		}
		h.seenResult[callID] = true
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
	if code == 0 || callID == "" {
		return
	}
	for _, i := range h.commandAt[callID] {
		if i >= len(h.s.Messages) || h.s.Messages[i].Role != RoleCommand {
			continue
		}
		if code < 0 {
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
		if v, _ := res[k].(string); strings.TrimSpace(v) != "" {
			parts = append(parts, strings.TrimSpace(v))
		}
	}
	return strings.Join(parts, "\n")
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
	for i, line := range lines {
		switch {
		case hermesPatchHeader.MatchString(line):
			m := hermesPatchHeader.FindStringSubmatch(line)
			file(m[2])
			lines[i] = "*** " + m[1] + " File: " + strings.TrimSpace(m[2])
		case hermesPatchMove.MatchString(line):
			m := hermesPatchMove.FindStringSubmatch(line)
			file(m[1])
			file(m[2])
			// A move carries no hunks; a header of its own ends the file
			// before it, so its lines are not read as the move's.
			lines[i] = "*** Delete File: " + strings.TrimSpace(m[1])
		case hermesPatchEnd.MatchString(line):
			lines[i] = "*** End Patch"
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
