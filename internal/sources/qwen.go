package sources

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/vshulcz/deja-vu/internal/model"
)

// QwenConfigDir is the native Qwen Code configuration directory. DEJA_QWEN_ROOT
// intentionally does not affect it because that variable only relocates reads.
func QwenConfigDir() string { return filepath.Join(Home(), ".qwen") }

func QwenRoot() string { return EnvPath("DEJA_QWEN_ROOT", QwenConfigDir()) }

func QwenSessionFiles() []string {
	return walkFiles(filepath.Join(QwenRoot(), "projects"), func(p string) bool {
		return strings.HasSuffix(p, ".jsonl") && filepath.Base(filepath.Dir(p)) == "chats"
	})
}

// QwenSidecarFiles are the files qwen keeps beside its transcripts that are not
// conversations: `<id>.runtime.json` per session, and `meta.json` and
// `extract-cursor.json` per project. Counted as unread transcripts they made
// doctor say "11 not recognised here" about a store it reads correctly — the
// same wrong claim Continue's `sessions.json` and Kimi's `state.json` used to
// produce (#3676).
func QwenSidecarFiles() []string {
	return walkFiles(filepath.Join(QwenRoot(), "projects"), func(p string) bool {
		base := filepath.Base(p)
		switch {
		case strings.HasSuffix(base, ".runtime.json"):
			return true
		case base == "meta.json" || base == "extract-cursor.json":
			return true
		}
		return false
	})
}

func LoadQwen() []model.Session { return parseFiles(QwenSessionFiles(), ParseQwenFile) }

// QwenProjectDirBase returns the encoded project dir name for a transcript
// path, e.g. "-Users-x-projects-app" for
// .../projects/-Users-x-projects-app/chats/s.jsonl. qwen resumes a session
// only from the directory it belongs to.
func QwenProjectDirBase(path string) string {
	dir := projectDir(filepath.Join(QwenRoot(), "projects"), path)
	base := filepath.Base(dir)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}

func ParseQwenFile(path string) ([]model.Session, error) {
	return parseQwenFileFromOffset(path, 0)
}

func ParseQwenFileFromOffset(path string, offset int64) ([]model.Session, error) {
	return parseQwenFileFromOffset(path, offset)
}

func parseQwenFileFromOffset(path string, offset int64) ([]model.Session, error) {
	project := projectDir(filepath.Join(QwenRoot(), "projects"), path)
	s := model.Session{
		Harness: "qwen",
		ID:      strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		Project: claudeProjectName(project),
		Path:    path,
	}
	err := scanJSONLFromOffset(path, offset, func(m map[string]any) {
		typ, _ := m["type"].(string)
		if typ == "tool_result" {
			// Every tool result is its own record, role user, parts holding
			// the functionResponse. Skipped with the other types, a failing
			// command's error never reached search or the fix pairs (#3281).
			t := parseTimeAny(m["timestamp"])
			// Touched like a user or assistant record, output or not: the
			// session's clock moves with every record it holds.
			s.Touch(t)
			if msg, ok := m["message"].(map[string]any); ok {
				s.Messages = append(s.Messages, qwenWorkRecords(msg["parts"], t)...)
			}
			return
		}
		if typ != "user" && typ != "assistant" {
			return
		}
		if id, _ := m["sessionId"].(string); id != "" {
			s.ID = id
		}
		t := parseTimeAny(m["timestamp"])
		s.Touch(t)
		role := typ
		text := ""
		if msg, ok := m["message"].(map[string]any); ok {
			if r, _ := msg["role"].(string); r != "" {
				switch r {
				case "model":
					role = "assistant"
				case "user":
					role = "user"
				default:
					role = typ
				}
			}
			text = qwenText(msg["parts"])
		}
		if text != "" {
			s.Messages = append(s.Messages, model.Message{Role: role, Text: text, Time: t})
		}
		// The work sits in the same parts list, as functionCall and
		// functionResponse rather than text, so qwenText walked past it and
		// the commands a session ran were reachable from nothing.
		if msg, ok := m["message"].(map[string]any); ok {
			s.Messages = append(s.Messages, qwenWorkRecords(msg["parts"], t)...)
		}
	})
	if len(s.Messages) == 0 {
		return nil, err
	}
	return []model.Session{s}, err
}

// qwenDialect is Qwen Code's tool vocabulary. The names follow Gemini's — Qwen
// Code is built in that shape — with `file_path` for the file tools and
// `command` for the shell.
var qwenDialect = toolDialect{
	pathKey:   "file_path",
	pathTools: map[string]bool{"read_file": true, "write_file": true, "replace": true, "read_many_files": true},
	shellTool: "run_shell_command",
	editTools: map[string]bool{"replace": true},
	oldKey:    "old_string",
}

// qwenWorkRecords turns the functionCall and functionResponse parts of one
// message into work records. The parts are rewritten into the tool_use shape
// the shared extractors read, so Qwen does not need its own copy of the
// extraction.
func qwenWorkRecords(v any, t time.Time) []model.Message {
	parts, ok := v.([]any)
	if !ok {
		return nil
	}
	var calls []any
	var results []string
	for _, part := range parts {
		m, ok := part.(map[string]any)
		if !ok {
			continue
		}
		if call, ok := m["functionCall"].(map[string]any); ok {
			name, _ := call["name"].(string)
			args, _ := call["args"].(map[string]any)
			if name != "" && args != nil {
				calls = append(calls, map[string]any{
					"type": "tool_use", "name": name, "input": args,
				})
			}
		}
		if resp, ok := m["functionResponse"].(map[string]any); ok {
			r, _ := resp["response"].(map[string]any)
			out, _ := r["output"].(string)
			if out = strings.TrimSpace(out); out == "" {
				// A failed call answers in `error`, the field the fix pairs
				// are mined from (#3281).
				out, _ = r["error"].(string)
				out = strings.TrimSpace(out)
			}
			// The shell's result is a report, not the output: indexed as is,
			// qwen's `Error: (none)` made every command read as failed and the
			// error was stored as `Output: <error>`, which no lookup asks for
			// (#4256).
			if name, _ := resp["name"].(string); qwenDialect.isShellTool(name) {
				out = strings.TrimSpace(UnwrapShellReport(out))
			}
			if out != "" {
				results = append(results, capParsedMessage(out))
			}
		}
	}
	var recs []model.Message
	if len(calls) > 0 {
		if IndexToolPaths() {
			if p := toolPathsIn(calls, qwenDialect); p != "" {
				recs = append(recs, model.Message{Role: RoleFiles, Text: p, Time: t})
			}
		}
		if IndexWrites() {
			for _, w := range wroteRecordsIn(calls, qwenDialect) {
				recs = append(recs, model.Message{Role: RoleWrote, Text: w, Time: t})
			}
		}
		if IndexEdits() {
			for _, span := range editSpansIn(calls, qwenDialect) {
				recs = append(recs, model.Message{Role: RoleEdit, Text: span, Time: t})
			}
		}
		if IndexCommands() {
			for _, cmd := range commandsIn(calls, qwenDialect) {
				recs = append(recs, model.Message{Role: RoleCommand, Text: cmd, Time: t})
			}
		}
	}
	if IndexToolOutput() {
		for _, out := range results {
			recs = append(recs, model.Message{Role: RoleToolOutput, Text: out, Time: t})
		}
	}
	return recs
}

func qwenText(v any) string {
	parts, ok := v.([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		m, ok := part.(map[string]any)
		if !ok {
			continue
		}
		if thought, _ := m["thought"].(bool); thought {
			continue
		}
		text, _ := m["text"].(string)
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(text)
	}
	return b.String()
}

// UnwrapShellReport strips the frame gemini and qwen put around a command's
// output before handing it to the model. Gemini fences it in
// <untrusted_context> with an "Output:" marker; qwen writes a labelled report —
// Command, Directory, Output, Error, Exit Code, Signal, PGID. The marker is
// what matters: with it in front, the first line of a build failure stops
// looking like an error, and the fix pair went silent on a failure it answers
// the moment the marker is gone (gemini-cli 0.55.1, qwen-code 0.20.0). The
// index and the failure hook both read it through here, so the error a pair is
// stored under is the one the hook looks up (#4256).
func UnwrapShellReport(s string) string {
	if !strings.Contains(s, "Output:") {
		return s
	}
	var kept []string
	labelled := false
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		// `Error: (none)` is qwen's report saying there was no error. A real
		// one keeps its label: it is the failure, and a line the command
		// printed itself may start the same way.
		if t == "<untrusted_context>" || t == "</untrusted_context>" || t == "Error: (none)" || shellReportLabel(t) {
			continue
		}
		// The label introduces the payload on its first line only; what follows
		// is the command's own output, untouched.
		if !labelled && strings.HasPrefix(line, "Output: ") {
			line, labelled = line[len("Output: "):], true
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// shellReportLabel reports whether a line is part of the shell report rather
// than the command's output.
func shellReportLabel(t string) bool {
	for _, label := range []string{"Command: ", "Directory: ", "Exit Code: ", "Signal: ", "Background PIDs: ", "Process Group PGID:"} {
		if strings.HasPrefix(t, label) {
			return true
		}
	}
	return false
}
