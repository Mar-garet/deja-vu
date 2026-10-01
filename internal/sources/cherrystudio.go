package sources

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vshulcz/deja-vu/internal/model"
)

// Cherry Studio (github.com/CherryHQ/cherry-studio) runs Claude Code sessions
// from a desktop app and writes them as ordinary Claude Code transcripts under
// its own app data:
//
//	<app data>/CherryStudio/Data/Agents/.claude/<projects>/<workspace>/<session>.jsonl
//
// and, before the upgrade that added `Data/Agents`, directly under
// `<app data>/CherryStudio/.claude/<projects>/…`. Both are read so a store that
// predates it is not lost.
//
// One difference from a stock store, and it matters for text rather than for
// tokens: Cherry Studio appends the same API call three or four times as the
// stream progresses — a new uuid each time, the same requestId and message id,
// the text growing. Read plainly that makes one reply into three messages, each
// a prefix of the next, so a recall can quote half a sentence and `deja show`
// prints the answer twice before finishing it. The reader collapses a run by
// its request id and keeps the longest (#3644).
//
// Sessions are their own harness rather than extra Claude roots: a reader
// should see which app the work happened in, and `deja sources` should say
// cherrystudio when Cherry Studio is what is on the machine.

// The fixture for this harness cannot carry the real path: the repository
// excludes `.claude/`, so a fixture under `…/Data/Agents/.claude/projects` is
// never committed and the suite passes only where it was written — which is
// how it reached CI red once (#3644).

// CherryStudioRoots are the transcript roots of a Cherry Studio install.
// DEJA_CHERRYSTUDIO_ROOTS replaces the list.
func CherryStudioRoots() []string {
	if list := os.Getenv("DEJA_CHERRYSTUDIO_ROOTS"); list != "" {
		var out []string
		for _, p := range filepath.SplitList(list) {
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	projects := claudeProjectsDirName()
	var out []string
	for _, base := range cherryStudioAppDirs() {
		out = append(out,
			filepath.Join(base, "Data", "Agents", ".claude", projects),
			filepath.Join(base, ".claude", projects),
		)
	}
	var live []string
	for _, p := range out {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			live = append(live, p)
		}
	}
	return live
}

// cherryStudioAppDirs is where the app keeps its data on this platform.
func cherryStudioAppDirs() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{filepath.Join(Home(), "Library", "Application Support", "CherryStudio")}
	case "windows":
		app := os.Getenv("APPDATA")
		if app == "" {
			app = filepath.Join(Home(), "AppData", "Roaming")
		}
		return []string{filepath.Join(app, "CherryStudio")}
	default:
		cfg := os.Getenv("XDG_CONFIG_HOME")
		if cfg == "" {
			cfg = filepath.Join(Home(), ".config")
		}
		return []string{filepath.Join(cfg, "CherryStudio")}
	}
}

// CherryStudioDatabases are where the app keeps its own store, MCP servers
// included: `<app data>/Data/cherrystudio.sqlite` (`app.database.file` in
// 2.0.14's out/main/main.js).
func CherryStudioDatabases() []string {
	var out []string
	for _, base := range cherryStudioAppDirs() {
		out = append(out, filepath.Join(base, "Data", "cherrystudio.sqlite"))
	}
	return out
}

// CherryStudioMCPServer is one row of the app's `mcp_server` table.
type CherryStudioMCPServer struct {
	Name    string
	Command string
	Args    []string
	// Active is the switch beside each server in Settings → MCP. The app
	// starts only the servers that have it on, and is_active defaults to off.
	Active bool
}

// CherryStudioMCPServers reads the MCP servers the app itself has, from the
// first of its databases that exists. ok is false when there is none, when
// sqlite3 is missing, or when the read fails: then nothing is known about the
// app, which is different from the app having no deja server (#4344).
//
// Read-only, through the same sqlite3 call the transcript stores use: a
// running app owns this file.
func CherryStudioMCPServers() (db string, servers []CherryStudioMCPServer, ok bool) {
	for _, p := range CherryStudioDatabases() {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			db = p
			break
		}
	}
	if db == "" || !SQLite3Available() {
		return db, nil, false
	}
	out, err := sqliteOutput(db,
		"select json_object('name', name, 'command', coalesce(command, ''), 'args', coalesce(args, ''), 'active', coalesce(is_active, 0)) from mcp_server;")
	if err != nil {
		return db, nil, false
	}
	rows, err := sqliteObjects[struct {
		Name    string `json:"name"`
		Command string `json:"command"`
		Args    string `json:"args"`
		Active  int    `json:"active"`
	}](out)
	if err != nil {
		return db, nil, false
	}
	for _, r := range rows {
		s := CherryStudioMCPServer{Name: r.Name, Command: r.Command, Active: r.Active != 0}
		// args is a JSON array in a text column; a row that does not parse
		// keeps its command, which is still enough to recognise deja.
		_ = json.Unmarshal([]byte(r.Args), &s.Args)
		servers = append(servers, s)
	}
	return db, servers, true
}

// CherryStudioSessionFiles lists the transcripts on disk.
func CherryStudioSessionFiles() []string {
	var out []string
	for _, root := range CherryStudioRoots() {
		out = append(out, walkFiles(root, func(p string) bool {
			return strings.HasSuffix(p, ".jsonl")
		})...)
	}
	return out
}

// ParseCherryStudioFile reads one transcript: Claude Code's format, with the
// snapshot run collapsed.
func ParseCherryStudioFile(path string) ([]model.Session, error) {
	return ParseCherryStudioFileFromOffset(path, 0)
}

// ParseCherryStudioFileFromOffset is the incremental read. A collapse spanning
// the watermark cannot see the earlier snapshots, so the longest text in the
// tail wins and the ingest de-duplicator drops what repeats — the same
// behaviour the appended-transcript path already relies on.
func ParseCherryStudioFileFromOffset(path string, offset int64) ([]model.Session, error) {
	return parseClaudeTypedWithOptions(path, func(fn func([]byte)) error {
		return scanJSONLBytes(path, offset, fn)
	}, claudeParseOptions{Harness: "cherrystudio", CollapseSnapshots: true})
}

// CherryStudioUnderRoot reports whether a path belongs to this store, so the
// registry can claim it without stealing a stock Claude transcript.
func CherryStudioUnderRoot(p string) bool {
	for _, root := range CherryStudioRoots() {
		if strings.HasPrefix(p, root) {
			return true
		}
	}
	return false
}

func LoadCherryStudio() []model.Session {
	return parseFiles(CherryStudioSessionFiles(), ParseCherryStudioFile)
}
