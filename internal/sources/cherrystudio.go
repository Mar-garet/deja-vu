package sources

import (
	"bufio"
	"encoding/json"
	"io"
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
// the watermark cannot see the earlier snapshots, so it is only used when
// CherryStudioResumes says the tail starts a new call.
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

// CherryStudioResumes reports whether the tail from offset can be appended to
// what is stored. An index that ran mid-stream stored the reply as far as it
// had got; when the tail carries more snapshots of that same call, appending
// kept the half reply next to the full one, and nothing de-duplicates a prefix
// against its completion (#4346). Then the file is read whole and the run
// collapses as it does on a first read.
func CherryStudioResumes(path string, offset int64) bool {
	if offset <= 0 {
		return true
	}
	key := cherrySnapshotKey(lastLineBefore(path, offset))
	if key == "" {
		return true
	}
	// Only as far as the first line that names a call: a stream's snapshots
	// run back to back, so if that line starts another call the stored one is
	// finished. Scanning the whole tail cost a search the read the inline
	// append cap exists to spare it.
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer func() { _ = f.Close() }()
	r := bufio.NewReader(io.NewSectionReader(f, offset, 1<<62))
	for {
		line, err := r.ReadBytes('\n')
		if k := cherrySnapshotKey(trimJSONSpace(line)); k != "" {
			return k != key
		}
		if err != nil {
			return true
		}
	}
}

// cherrySnapshotKey is the call a line reports on, by requestId or message id
// only: a uuid changes with every snapshot, so it never joins a run.
func cherrySnapshotKey(line []byte) string {
	var v struct {
		RequestID string `json:"requestId"`
		Message   *struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	if len(line) == 0 || json.Unmarshal(line, &v) != nil {
		return ""
	}
	if v.RequestID != "" {
		return "req:" + v.RequestID
	}
	if v.Message != nil && v.Message.ID != "" {
		return "msg:" + v.Message.ID
	}
	return ""
}

// lastLineBefore returns the last complete line ending at end, read backwards
// so a long transcript is not read from its first byte.
func lastLineBefore(path string, end int64) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var line []byte
	pos := end - 1 // the newline that ends the line
	for pos > 0 {
		n := int64(64 << 10)
		if n > pos {
			n = pos
		}
		buf := make([]byte, n)
		if _, err := f.ReadAt(buf, pos-n); err != nil && err != io.EOF {
			return nil
		}
		for i := len(buf) - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				return append(buf[i+1:], line...)
			}
		}
		line = append(buf, line...)
		pos -= n
	}
	return line
}
