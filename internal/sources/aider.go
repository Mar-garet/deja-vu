package sources

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vshulcz/deja-vu/internal/model"
)

// aider writes one markdown file per launch directory, appending forever:
// sessions start with "# aider chat started at <ts>", user input lines are
// prefixed "#### ", tool/system output "> ", and assistant output is raw
// markdown in between. Fenced code blocks may contain any of those prefixes,
// so the parser tracks fences.

const aiderHistoryName = ".aider.chat.history.md"

// AiderFiles returns history files to index: $HOME, every project `deja aider`
// has started aider in, plus any directories listed in DEJA_AIDER_ROOTS
// (colon-separated, scanned two levels deep — scanning all of $HOME would make
// warmup unusable).
func AiderFiles() []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if seen[p] {
			return
		}
		// Regular files only: a FIFO at the history path would block the
		// parser's Open forever (same hang walkFiles guards against).
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			seen[p] = true
			out = append(out, p)
		}
	}
	add(filepath.Join(Home(), aiderHistoryName))
	// aider maps every flag to an env var; --chat-history-file is the
	// documented way to move the history off the default name.
	if p := os.Getenv("AIDER_CHAT_HISTORY_FILE"); p != "" {
		add(p)
	}
	// aider writes the history at the git root it runs in, so $HOME holds one
	// only for a launch from $HOME. The projects `deja aider` was started in
	// are read without anyone listing them (#4326).
	for _, dir := range aiderProjects() {
		add(filepath.Join(dir, aiderHistoryName))
	}
	for _, root := range filepath.SplitList(os.Getenv("DEJA_AIDER_ROOTS")) {
		if root == "" {
			continue
		}
		add(filepath.Join(root, aiderHistoryName))
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			add(filepath.Join(root, e.Name(), aiderHistoryName))
			sub, err := os.ReadDir(filepath.Join(root, e.Name()))
			if err != nil {
				continue
			}
			for _, s := range sub {
				if s.IsDir() {
					add(filepath.Join(root, e.Name(), s.Name(), aiderHistoryName))
				}
			}
		}
	}
	return out
}

// AiderProjectsPath is the list of directories `deja aider` has started aider
// in, one per line.
func AiderProjectsPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(Home(), ".config")
	}
	return filepath.Join(base, "deja", "aider-projects")
}

func aiderProjects() []string {
	b, err := os.ReadFile(AiderProjectsPath())
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// RecordAiderProject adds dir to the list AiderFiles reads, once.
func RecordAiderProject(dir string) error {
	for _, p := range aiderProjects() {
		if p == dir {
			return nil
		}
	}
	path := AiderProjectsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(dir + "\n"); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func LoadAider() []model.Session {
	return parseFiles(AiderFiles(), ParseAiderFile)
}

const aiderSessionMark = "# aider chat started at "

// aiderCommands are aider's own inputs, which it records in the transcript the
// same way it records a question. None of them is something the person said:
// most take no prose or a path, `/run`, `/test` and `/git` are shell lines
// (kept as commands, see aiderShellCommand), and `/ask`, `/code`, `/context`
// and `/architect` hand their question to a sub-coder, which logs it again as
// its own `#### ` line — kept, the question was indexed twice and the session
// was titled with the command (#4325).
var aiderCommands = map[string]bool{
	"add": true, "architect": true, "ask": true, "chat-mode": true, "clear": true,
	"clipboard": true, "code": true, "commit": true, "context": true, "copy": true,
	"copy-context": true, "diff": true, "drop": true, "edit": true, "editor": true,
	"editor-model": true, "exit": true, "git": true, "help": true, "lint": true,
	"load": true, "ls": true, "map": true, "map-refresh": true, "model": true,
	"models": true, "multiline-mode": true, "paste": true, "quit": true,
	"read-only": true, "reasoning-effort": true, "report": true, "reset": true,
	"run": true, "save": true, "settings": true, "test": true, "think-tokens": true,
	"tokens": true, "undo": true, "voice": true, "weak-model": true, "web": true,
}

// aiderShellCommand returns the shell line an input ran: `!cmd` and `/run cmd`
// run it, `/test cmd` runs it as the test command, and `/git args` is git.
func aiderShellCommand(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if rest, ok := strings.CutPrefix(t, "!"); ok {
		return strings.TrimSpace(rest), true
	}
	if !strings.HasPrefix(t, "/") {
		return "", false
	}
	name, rest, _ := strings.Cut(t[1:], " ")
	rest = strings.TrimSpace(rest)
	switch strings.ToLower(name) {
	case "run", "test":
		return rest, true
	case "git":
		if rest == "" {
			return "", true
		}
		return "git " + rest, true
	}
	return "", false
}

// aiderOutputFile returns the file an output line says aider added or edited:
// "Applied edit to x", "Added x to the chat", "Added x to the chat (read-only)."
// and "Added x to read-only files.". "Added 3 lines of output to the chat." is
// not a file.
func aiderOutputFile(out string) string {
	out = strings.TrimSuffix(out, ".")
	if f, ok := strings.CutPrefix(out, "Applied edit to "); ok {
		return f
	}
	f, ok := strings.CutPrefix(out, "Added ")
	if !ok {
		return ""
	}
	for _, tail := range []string{" to the chat (read-only)", " to the chat", " to read-only files"} {
		if g, ok := strings.CutSuffix(f, tail); ok {
			if aiderOutputLines.MatchString(g) {
				return ""
			}
			return g
		}
	}
	return ""
}

var aiderOutputLines = regexp.MustCompile(`^\d+ lines? of output$`)

// aiderSlashCommand reports that a line is one of aider's own commands rather
// than something the person asked.
//
// The name has to be one aider has, not merely a leading slash: "/etc/hosts is
// wrong on the build box" opens the same way and is the reader's, and so is a
// line that starts with any absolute path.
func aiderSlashCommand(line string) bool {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "/") || len(t) < 2 {
		return false
	}
	name := t[1:]
	if i := strings.IndexAny(name, " \t"); i >= 0 {
		name = name[:i]
	}
	drop, known := aiderCommands[strings.ToLower(name)]
	return known && drop
}

func ParseAiderFile(path string) ([]model.Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	project := projectName(filepath.Dir(path))
	// aider names files relative to its git root, which is where the history
	// sits unless it was moved with --chat-history-file.
	fileRoot := ""
	if filepath.Base(path) == aiderHistoryName {
		fileRoot = filepath.Dir(path)
	}
	var out []model.Session
	var seenFiles map[string]bool
	var cur *model.Session
	var role string
	var buf []string
	inFence := false
	idx := 0
	// aider marks its own output with "> " on the first line only: the
	// --verbose configuration dump and a multi-line commit message continue
	// unprefixed, and those lines read as the assistant speaking — one of them
	// went on to title the session (#3311). Two rules cover what aider writes:
	// nothing before the session's first `#### ` turn is speech (the banner
	// and the dump come before anyone has asked anything, and the dump has
	// blank lines inside it), and a line directly under a "> " line is the
	// rest of that block.
	afterOutput := false
	seenUser := false

	flush := func() {
		if cur == nil || len(buf) == 0 {
			role, buf = "", nil
			return
		}
		// A markdown transcript is bytes, not JSON: nothing on the way in
		// rejects a sequence that is not UTF-8, which is what every other
		// parser gets for free from its decoder. Replace them here, with the
		// same character a JSON decoder would leave, so the index holds text
		// and two words either side of the damage stay two words (#1740).
		text := strings.ToValidUTF8(strings.TrimSpace(strings.Join(buf, "\n")), "\uFFFD")
		if text != "" && role != "" {
			cur.Messages = append(cur.Messages, model.Message{Role: role, Text: text, Time: cur.Started})
		}
		role, buf = "", nil
	}
	endSession := func() {
		flush()
		if cur != nil && len(cur.Messages) > 0 {
			out = append(out, *cur)
		}
		cur = nil
	}

	// Unbounded line reader: a single pasted blob can exceed any fixed
	// scanner cap, which would silently drop every session after it. Read
	// line by line with no cap, matching the other parsers.
	r := bufio.NewReader(f)
	for {
		raw, readErr := r.ReadString('\n')
		if raw == "" && readErr != nil {
			break
		}
		line := strings.TrimRight(raw, " \t\r\n")
		if !inFence && strings.HasPrefix(line, aiderSessionMark) {
			endSession()
			idx++
			ts, _ := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(strings.TrimPrefix(line, aiderSessionMark)), time.Local)
			id := aiderSessionID(path, idx)
			cur = &model.Session{Harness: "aider", ID: id, Project: project, Path: path, Started: ts, Updated: ts}
			seenFiles = map[string]bool{}
			inFence = false
			afterOutput, seenUser = false, false
			continue
		}
		if cur == nil {
			continue
		}
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			afterOutput = false
			if role == "" {
				role = "assistant"
			}
			buf = append(buf, line)
			continue
		}
		if inFence {
			buf = append(buf, line)
			continue
		}
		switch {
		case strings.HasPrefix(line, "#### "):
			afterOutput = false
			seenUser = true
			t := strings.TrimPrefix(line, "#### ")
			// A shell line is what ran, not what was asked (#4324).
			if cmd, ok := aiderShellCommand(t); ok {
				flush()
				if cmd != "" && IndexCommands() && worthIndexing(cmd) {
					cur.Messages = append(cur.Messages, model.Message{Role: RoleCommand, Text: cmd, Time: cur.Started})
				}
				continue
			}
			// aider logs its own commands the same way — `/undo`, `/clear`,
			// `/add x` — and they are not the person's question; a message
			// that merely opens with a path ("/etc/hosts is wrong") is (#3248).
			if aiderSlashCommand(t) {
				flush()
				role = ""
				continue
			}
			if role != "user" {
				flush()
				role = "user"
			}
			if t == "<blank>" {
				t = ""
			}
			buf = append(buf, t)
		case strings.HasPrefix(line, "> "), line == ">":
			// tool/system output: ends any assistant block, not indexed as a
			// message. What it says aider added, edited or ran is kept as the
			// record the JSONL harnesses write for the same thing (#4324).
			flush()
			afterOutput = true
			said := strings.TrimSpace(strings.TrimPrefix(line, ">"))
			if f := aiderOutputFile(said); f != "" && IndexToolPaths() && !strings.ContainsAny(f, "\n\r") {
				if fileRoot != "" && !filepath.IsAbs(f) {
					f = filepath.Join(fileRoot, f)
				}
				if !seenFiles[f] {
					seenFiles[f] = true
					cur.Messages = append(cur.Messages, model.Message{Role: RoleFiles, Text: f, Time: cur.Started})
				}
			} else if cmd, ok := strings.CutPrefix(said, "Running "); ok && IndexCommands() && worthIndexing(cmd) {
				cur.Messages = append(cur.Messages, model.Message{Role: RoleCommand, Text: cmd, Time: cur.Started})
			}
		case strings.TrimSpace(line) == "":
			afterOutput = false
			buf = append(buf, "")
		case afterOutput, !seenUser:
			// The rest of the output block the "> " line opened, or the
			// banner and the --verbose dump aider prints before the first
			// turn.
			continue
		default:
			if role != "assistant" {
				flush()
				role = "assistant"
			}
			buf = append(buf, line)
		}
	}
	endSession()
	return out, nil
}

// aider has no session ids; derive a stable one from file path + ordinal.
func aiderSessionID(path string, idx int) string {
	h := sha1.Sum([]byte(path))
	return "aider-" + hex.EncodeToString(h[:6]) + "-" + itoa(idx)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	// Wide enough for any int on a 64-bit build. At eight it panicked on the
	// ninth digit, which nothing here reaches today and which a caller passing
	// a unix stamp would.
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
