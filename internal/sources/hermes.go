package sources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vshulcz/deja-vu/internal/model"
)

// HermesHome is the Hermes root: profiles, plugins and config.yaml live here.
// HERMES_HOME is Hermes's own switch and moves install, parse and doctor
// together; DEJA_HERMES_HOME overrides it for deja alone (#3203).
func HermesHome() string {
	return EnvPath("DEJA_HERMES_HOME", EnvPath("HERMES_HOME", filepath.Join(Home(), ".hermes")))
}

// Hermes keeps one SQLite store per profile under ~/.hermes/profiles/<name>,
// so a user running several profiles has several stores and all of them count.
func HermesProfilesRoot() string {
	if p := os.Getenv("DEJA_HERMES_PROFILES_ROOT"); p != "" {
		return p
	}
	return filepath.Join(HermesHome(), "profiles")
}

// HermesDBs lists every profile store that exists and has content. A single
// store can be forced with DEJA_HERMES_DB, which is what the tests use.
func HermesDBs() []string {
	if p := os.Getenv("DEJA_HERMES_DB"); p != "" {
		if fi, err := os.Stat(p); err == nil && fi.Size() > 0 {
			return []string{p}
		}
		return nil
	}
	var out []string
	// 0.17 keeps one store at the root; older builds shard it per profile.
	// Both shapes exist in the wild, so both are looked for.
	if db := filepath.Join(HermesHome(), "state.db"); nonEmptyFile(db) {
		out = append(out, db)
	}
	entries, err := os.ReadDir(HermesProfilesRoot())
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		db := filepath.Join(HermesProfilesRoot(), e.Name(), "state.db")
		if nonEmptyFile(db) {
			out = append(out, db)
		}
	}
	return out
}

// HermesSessionFiles is the store list the indexer stats for changes. The
// Postgres store, when opted in, rides along as a token the index fingerprints
// instead of stats.
func HermesSessionFiles() []string {
	files := HermesDBs()
	if dsn := HermesPGDSN(); dsn != "" {
		files = append(files, HermesPGStorePath(dsn))
	}
	return files
}

func LoadHermes() []model.Session {
	var out []model.Session
	for _, db := range HermesDBs() {
		ss, _ := ParseHermesDB(db)
		out = append(out, ss...)
	}
	if dsn := HermesPGDSN(); dsn != "" {
		ss, _ := ParseHermesPG(dsn, 0)
		out = append(out, ss...)
	}
	return out
}

func ParseHermesDB(db string) ([]model.Session, error) {
	return parseHermesDBWhere(db, "")
}

// ParseHermesDBSince reads only what changed, so a re-index of an unchanged
// profile costs one query instead of the whole history. timestamp is REAL
// seconds since the epoch in Hermes' schema.
//
// A second back from the watermark, which is the guard grok's reader spells in
// milliseconds (#2150). The column is REAL but a store may write whole seconds
// into it, and then every message sharing the watermark's second compares equal
// to it and a strict `>` leaves it out for good — measured on such a store, 0
// of 2 came back. The cost is re-reading one second of history, and those turns
// are already held (#2075).
func ParseHermesDBSince(db string, t time.Time) ([]model.Session, error) {
	if t.IsZero() {
		return parseHermesDBWhere(db, "")
	}
	// By session, so the session comes back whole: what a store parsed from its
	// watermark hands back replaces what the index holds for that key, and a
	// return of the newest turn alone takes the earlier ones with it (#2075).
	// The whole-second floor is then harmless — it can only re-offer a message
	// the pass was going to replace anyway.
	return parseHermesDBWhere(db, fmt.Sprintf(
		" and session_id in (select session_id from messages where timestamp > %d)",
		t.Add(-time.Second).Unix()))
}

func parseHermesDBWhere(db, where string) ([]model.Session, error) {
	if fi, err := os.Stat(db); err != nil || fi.Size() == 0 {
		return nil, nil
	}
	// json_object rather than the shell's -json mode, which is quadratic in
	// what it escapes — see sqliteRows.
	q := `select json_object('session_id',cast(session_id as text),'role',cast(role as text),` +
		`'content',cast(content as text),'timestamp',timestamp) from messages ` +
		`where role in ('user','assistant') and content is not null and content <> ''` + where +
		` order by session_id,timestamp,id`
	if hermesHasToolColumns(db) {
		// A tool-call row has no content, only tool_calls, and the result lands
		// on a `tool` row; both carry the session's work (#4242).
		q = `select json_object('session_id',cast(session_id as text),'role',cast(role as text),` +
			`'content',cast(content as text),'timestamp',timestamp,` +
			`'tool_calls',cast(tool_calls as text),'tool_call_id',cast(tool_call_id as text)) from messages ` +
			`where role in ('user','assistant','tool') and ((content is not null and content <> '')` +
			` or (tool_calls is not null and tool_calls <> ''))` + where +
			` order by session_id,timestamp,id`
	}
	cmd, stopRead := sqliteReadCmd(db, q)
	defer stopRead()
	dec, err := sqliteRows(cmd)
	if err != nil {
		return nil, err
	}
	out, err := decodeHermesArray(dec, hermesProfile(db), db)
	if err != nil {
		_ = cmd.Wait()
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		if len(out) == 0 {
			// No stdout means two very different things: a query that matched
			// nothing, or one sqlite3 refused to run because the harness
			// changed its schema. Reporting the second as "no sessions" makes
			// a whole harness disappear from recall while doctor still calls
			// the store healthy.
			return nil, fmt.Errorf("hermes: query failed, the store schema may have changed: %w", err)
		}
		return nil, err
	}
	cwds := hermesSessionCwds(db)
	for i := range out {
		if cwd := cwds[out[i].ID]; cwd != "" {
			out[i].Project = claudeProjectName(pathToProjectKey(cwd))
		}
	}
	return out, nil
}

// decodeHermesArray reads {session_id,role,content,timestamp} rows into
// sessions — with tool_calls and tool_call_id when the query asked for them —
// either as a json array — Postgres json_agg, with dec positioned
// just past the opening '[' and left just past the closing ']' — or as the
// bare stream of objects the sqlite3 reader produces. project and path stamp
// every session.
func decodeHermesArray(dec *json.Decoder, project, path string) ([]model.Session, error) {
	by := map[string]*model.Session{}
	var order []string
	// Which messages hold the commands a call ran, by session and call id, so
	// the exit code on the `tool` row lands on its command.
	commandAt := map[[2]string][]int{}
	for dec.More() {
		var r map[string]any
		if err := dec.Decode(&r); err != nil {
			return nil, err
		}
		id := str(r["session_id"])
		if id == "" {
			continue
		}
		s := by[id]
		if s == nil {
			s = &model.Session{Harness: "hermes", ID: id, Project: project, Path: path}
			by[id] = s
			order = append(order, id)
		}
		t := hermesTime(r["timestamp"])
		role := str(r["role"])
		txt := strings.TrimSpace(str(r["content"]))
		if role == "tool" {
			hermesToolResult(s, txt, commandAt[[2]string{id, str(r["tool_call_id"])}], t)
			continue
		}
		if txt != "" {
			s.Touch(t)
			s.Messages = append(s.Messages, model.Message{Role: role, Text: capParsedMessage(txt), Time: t})
		}
		if role == "assistant" {
			hermesToolCalls(s, str(r["tool_calls"]), commandAt, t)
		}
	}
	if _, err := dec.Token(); err != nil && err != io.EOF {
		return nil, err
	}
	out := make([]model.Session, 0, len(order))
	for _, id := range order {
		s := by[id]
		if len(s.Messages) == 0 {
			continue
		}
		// No title here: the index derives one (the person's first line,
		// a greeting giving way to the next turn, the agent's line when
		// nobody typed). Titling in the parser skipped the greeting rule, so
		// a session opened with "hi" listed as "hi" (#3241, #3251).
		out = append(out, *s)
	}
	return out, nil
}

// hermesHasToolColumns probes for the two columns the tool rows are read
// from. Every Hermes schema seen has them, but naming a missing column fails
// the whole query, and a store without them still has its prose to give.
func hermesHasToolColumns(db string) bool {
	out, err := sqliteOutput(db, "pragma table_info(messages)")
	return err == nil && bytes.Contains(out, []byte("|tool_calls|")) && bytes.Contains(out, []byte("|tool_call_id|"))
}

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

// hermesToolCalls turns an assistant row's OpenAI-style tool_calls into work
// records, noting where each call's commands landed.
func hermesToolCalls(s *model.Session, raw string, commandAt map[[2]string][]int, t time.Time) {
	if raw == "" {
		return
	}
	var calls []any
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return
	}
	for _, c := range calls {
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
				at = append(at, len(s.Messages)+len(records))
				records = append(records, model.Message{Role: RoleCommand, Text: cmd, Time: t})
			}
		}
		if len(records) == 0 {
			continue
		}
		if m, _ := c.(map[string]any); m != nil && len(at) > 0 {
			if id, _ := m["id"].(string); id != "" {
				commandAt[[2]string{s.ID, id}] = at
			}
		}
		s.Touch(t)
		s.Messages = append(s.Messages, records...)
	}
}

// hermesPatchRecords reads patch in its V4A mode, where the call carries a
// multi-file patch instead of a path and a span.
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
	var out []model.Message
	if IndexToolPaths() {
		var files []string
		seen := map[string]bool{}
		for _, m := range codexPatchFile.FindAllStringSubmatch(patch, -1) {
			if f := strings.TrimSpace(m[1]); f != "" && !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
		if len(files) > 0 {
			out = append(out, model.Message{Role: RoleFiles, Text: strings.Join(files, "\n"), Time: t})
		}
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

// hermesToolResult records a `tool` row. terminal answers with
// {output, exit_code, error}: the output is the record and a non-zero code
// rides on the command it answers, as it does for opencode and Copilot. Every
// other tool's JSON is kept as written.
func hermesToolResult(s *model.Session, txt string, cmdAt []int, t time.Time) {
	if txt == "" {
		return
	}
	var res map[string]any
	if json.Unmarshal([]byte(txt), &res) == nil {
		if out, ok := res["output"].(string); ok {
			txt = strings.TrimSpace(out)
			if e, _ := res["error"].(string); strings.TrimSpace(e) != "" {
				txt = strings.TrimSpace(txt + "\n" + e)
			}
			if code := exitCode(res["exit_code"]); code > 0 {
				for _, i := range cmdAt {
					if i < len(s.Messages) && s.Messages[i].Role == RoleCommand {
						s.Messages[i].Text += fmt.Sprintf("  → exit %d", code)
					}
				}
			}
		}
	}
	if txt == "" || !IndexToolOutput() {
		return
	}
	s.Touch(t)
	s.Messages = append(s.Messages, model.Message{Role: RoleToolOutput, Text: capParsedMessage(txt), Time: t})
}

// hermesTime reads Hermes' REAL epoch seconds. The shared parser handles
// integer epochs and RFC 3339, but a fractional second arrives as 1785000000.5
// and would otherwise land as the zero time — which sorts as ancient and never
// surfaces in recall.
func hermesTime(v any) time.Time {
	var secs float64
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return parseTimeAny(v)
		}
		secs = f
	case float64:
		secs = n
	default:
		return parseTimeAny(v)
	}
	if secs <= 0 {
		return time.Time{}
	}
	sec := int64(secs)
	return time.Unix(sec, int64((secs-float64(sec))*1e9)).UTC()
}

func nonEmptyFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Size() > 0
}

// hermesSessionCwds reads the directory each session was recorded in, from the
// sessions table Hermes keeps beside messages. Stamped with the profile alone,
// every session fell into one project, and the prompt hook — which ranks the
// payload's project only — never served a Hermes session in the directory it
// was about (#3257). Best effort: a store from before the table, or a row with
// no cwd, keeps the profile.
func hermesSessionCwds(db string) map[string]string {
	q := `select json_object('id',id,'cwd',cwd) from sessions where cwd is not null and cwd <> ''`
	out, err := sqliteOutput(db, q)
	if err != nil {
		return nil
	}
	rows, err := sqliteObjects[struct {
		ID  string `json:"id"`
		Cwd string `json:"cwd"`
	}](out)
	if err != nil {
		return nil
	}
	cwds := make(map[string]string, len(rows))
	for _, r := range rows {
		if r.ID != "" && strings.TrimSpace(r.Cwd) != "" {
			cwds[r.ID] = strings.TrimSpace(r.Cwd)
		}
	}
	return cwds
}

// hermesProfile names the session's project after the profile directory when
// the store says nothing about where the session was recorded.
func hermesProfile(db string) string {
	name := filepath.Base(filepath.Dir(db))
	// The root store has no profile directory to be named after.
	if name == filepath.Base(HermesHome()) {
		return "hermes"
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "hermes"
	}
	return name
}
