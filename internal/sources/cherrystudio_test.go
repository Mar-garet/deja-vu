package sources

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Cherry Studio writes Claude Code transcripts from a desktop app, and appends
// the same API call three or four times as the stream progresses: a new uuid
// each time, the same requestId and message id, the text growing. Read plainly
// that is one reply stored three times in prefixes — measured on this fixture
// before the collapse: "the advisory", "the advisory lock was", "the advisory
// lock was never released" (#3644).
const cherrySnapshotTranscript = `{"type":"user","sessionId":"cs-1","timestamp":"2026-07-17T09:00:00.000Z","cwd":"/work/api","message":{"role":"user","content":"why does the migration hang"}}
{"type":"assistant","uuid":"u1","requestId":"req-1","sessionId":"cs-1","timestamp":"2026-07-17T09:00:01.000Z","message":{"id":"msg-1","role":"assistant","content":[{"type":"text","text":"the advisory"}]}}
{"type":"assistant","uuid":"u2","requestId":"req-1","sessionId":"cs-1","timestamp":"2026-07-17T09:00:01.500Z","message":{"id":"msg-1","role":"assistant","content":[{"type":"text","text":"the advisory lock was"}]}}
{"type":"assistant","uuid":"u3","requestId":"req-1","sessionId":"cs-1","timestamp":"2026-07-17T09:00:02.000Z","message":{"id":"msg-1","role":"assistant","content":[{"type":"text","text":"the advisory lock was never released"}]}}
{"type":"assistant","uuid":"u4","requestId":"req-2","sessionId":"cs-1","timestamp":"2026-07-17T09:00:05.000Z","message":{"id":"msg-2","role":"assistant","content":[{"type":"text","text":"rerun the migration with a lock timeout"}]}}
`

func writeCherryStore(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, "CherryStudio", "Data", "Agents", ".claude", "projects", "-work-api")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cs-1.jsonl"), []byte(cherrySnapshotTranscript), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEJA_CHERRYSTUDIO_ROOTS", filepath.Dir(root))
	return root
}

func TestCherryStudioCollapsesAStreamingRun(t *testing.T) {
	writeCherryStore(t)

	files := CherryStudioSessionFiles()
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	ss := LoadCherryStudio()
	if len(ss) != 1 {
		t.Fatalf("sessions = %d, want 1", len(ss))
	}
	s := ss[0]
	if s.Harness != "cherrystudio" {
		t.Errorf("harness = %q — a Cherry Studio session must not report as Claude Code", s.Harness)
	}
	var assistant []string
	for _, m := range s.Messages {
		if m.Role == "assistant" {
			assistant = append(assistant, m.Text)
		}
	}
	if len(assistant) != 2 {
		t.Fatalf("assistant messages = %d %q, want one per API call", len(assistant), assistant)
	}
	if assistant[0] != "the advisory lock was never released" {
		t.Errorf("the collapsed reply is %q, want the complete snapshot", assistant[0])
	}
	if assistant[1] != "rerun the migration with a lock timeout" {
		t.Errorf("the second call was folded into the first: %q", assistant[1])
	}
	// The user turn survives the collapse.
	if len(s.Messages) < 3 || s.Messages[0].Role != "user" {
		t.Errorf("the question is missing: %+v", s.Messages)
	}
}

// The same file read through the stock Claude parser keeps every snapshot,
// which is what makes the collapse a property of this store rather than a
// change to Claude Code's own reader.
func TestClaudeItselfIsUnchangedByTheCollapse(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cs-1.jsonl")
	if err := os.WriteFile(p, []byte(cherrySnapshotTranscript), 0o644); err != nil {
		t.Fatal(err)
	}
	ss, err := ParseClaudeFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) != 1 {
		t.Fatalf("sessions = %d", len(ss))
	}
	n := 0
	for _, m := range ss[0].Messages {
		if m.Role == "assistant" {
			n++
		}
	}
	if n != 4 {
		t.Errorf("the stock reader kept %d assistant records, want all four", n)
	}
	if ss[0].Harness != "claude" {
		t.Errorf("harness = %q, want claude", ss[0].Harness)
	}
}

// A stock Claude transcript must not be claimed by this harness, and a Cherry
// Studio one must not be claimed by Claude's kind.
func TestCherryStudioAndClaudeDoNotClaimEachOther(t *testing.T) {
	root := writeCherryStore(t)
	cherry := filepath.Join(root, "cs-1.jsonl")

	stockHome := t.TempDir()
	stock := filepath.Join(stockHome, ".claude", "projects", "-work-api")
	if err := os.MkdirAll(stock, 0o755); err != nil {
		t.Fatal(err)
	}
	stockFile := filepath.Join(stock, "s-1.jsonl")
	if err := os.WriteFile(stockFile, []byte(cherrySnapshotTranscript), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEJA_CLAUDE_ROOT", stock)

	kinds := map[string]string{}
	for _, h := range Registry() {
		for _, k := range h.Kinds {
			for _, p := range []string{cherry, stockFile} {
				if k.Match(p) {
					kinds[p] += h.Name + ":" + k.Name + " "
				}
			}
		}
	}
	if !strings.Contains(kinds[cherry], "cherrystudio") {
		t.Errorf("the Cherry Studio transcript is claimed by %q", kinds[cherry])
	}
	if strings.Contains(kinds[stockFile], "cherrystudio") {
		t.Errorf("a stock Claude transcript is claimed by cherrystudio: %q", kinds[stockFile])
	}
}

// cherryHome points every app-dir lookup at a fresh home, so the default
// Cherry Studio data dir is a temp dir on every platform.
func cherryHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("DEJA_CHERRYSTUDIO_ROOTS", "")
	return home
}

func copyFixture(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", from))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Cherry Studio runs agents on three runtimes and gives each its own store
// under Data/Agents: .claude, .pi/sessions and .dsh/sessions. Before the fix
// only the Claude one was read, so a Cherry agent on pi or dsh left nothing in
// the index (#4342).
func TestCherryStudioReadsItsPiAndDshAgents(t *testing.T) {
	cherryHome(t)
	agents := filepath.Join(cherryStudioAppDirs()[0], "Data", "Agents")
	pi := filepath.Join(agents, ".pi", "sessions", "2026-07-17T10-00-00-000Z_c0ffee00-0000-4000-8000-000000000002.jsonl")
	dsh := filepath.Join(agents, ".dsh", "sessions", "--work-pgbouncer-lab--", "session-c0ffee00-0000-4000-8000-000000000003", "session.jsonl")
	copyFixture(t, "fixtures/registry/pi/session.jsonl", pi)
	copyFixture(t, "fixtures/registry/deepseek/sessions/--work-pgbouncer-lab--/session-eaf5c9ac-0e47-4d2f-b982-8bae306062d1/session.jsonl", dsh)

	got := map[string]bool{}
	for _, s := range LoadCherryStudio() {
		if s.Harness != "cherrystudio" {
			t.Errorf("%s: harness = %q, want cherrystudio", s.Path, s.Harness)
		}
		if len(s.Messages) == 0 {
			t.Errorf("%s: no messages", s.Path)
		}
		got[s.Path] = true
	}
	if !got[pi] || !got[dsh] {
		t.Fatalf("read %v, want the pi and the dsh session", got)
	}
	// The incremental path must route each file to the same reader: the dsh
	// log would otherwise fall to the deepseek kind, which matches by name.
	for _, p := range []string{pi, dsh} {
		if k := KindForPath(p); !strings.HasPrefix(k, "cherrystudio") {
			t.Errorf("%s is claimed by kind %q", p, k)
		}
	}
}

// Cherry Studio lets a user move its data dir; the new place is kept in
// ~/.cherrystudio/boot-config.json under app.user_data_path, a map from the
// executable to the directory. Before the fix deja only looked at the default
// and found nothing after a move (#4347).
func TestCherryStudioFollowsAMovedDataDir(t *testing.T) {
	home := cherryHome(t)
	moved := filepath.Join(t.TempDir(), "moved-data")
	cfg := `{"app.disable_hardware_acceleration":false,"app.user_data_path":{"/Applications/Cherry Studio.app/Contents/MacOS/Cherry Studio":` + strconvQuote(moved) + `},"temp.user_data_relocation":null}`
	if err := os.MkdirAll(filepath.Join(home, ".cherrystudio"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".cherrystudio", "boot-config.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(moved, "Data", "Agents", ".claude", "projects", "-work-api", "cs-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(cherrySnapshotTranscript), 0o644); err != nil {
		t.Fatal(err)
	}
	files := CherryStudioSessionFiles()
	if len(files) != 1 || files[0] != p {
		t.Fatalf("files = %v, want the transcript in the moved dir", files)
	}
	if k := KindForPath(p); k != "cherrystudio" {
		t.Errorf("kind = %q, want cherrystudio", k)
	}
}

func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }
