package index

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// These tests hold an index built a pass at a time to what a rebuild of the
// same stores gives. Each one found a case where the two disagreed.

// parityStores points every store deja reads at a directory that does not
// exist, so a test reads only the store it sets up.
var parityStores = []string{
	"DEJA_AIDER_ROOTS", "DEJA_AMP_ROOT", "DEJA_ANTIGRAVITY_ROOT", "DEJA_CC_MIRROR_ROOT", "DEJA_CHERRYSTUDIO_ROOTS",
	"DEJA_CLAUDE_ROOT", "DEJA_CLINE_ROOT", "DEJA_CLINE_ROOTS", "DEJA_CODEWHALE_ROOT", "DEJA_CODEX_ROOT",
	"DEJA_COMMANDCODE_ROOT", "DEJA_CONTINUE_ROOT", "DEJA_COPILOT_CHAT_ROOTS", "DEJA_COPILOT_ROOT", "DEJA_CRUSH_ROOT",
	"DEJA_CURSOR_CLI_ROOT", "DEJA_CURSOR_ROOT", "DEJA_DEEPSEEK_ROOT", "DEJA_GEMINI_ROOT", "DEJA_GJC_ROOT",
	"DEJA_GOOSE_DB", "DEJA_GOOSE_ROOT", "DEJA_GROK_DB", "DEJA_GROK_ROOT", "DEJA_HERMES_DB", "DEJA_HERMES_HOME",
	"DEJA_HERMES_PROFILES_ROOT", "DEJA_KILO_DB", "DEJA_KILO_ROOTS", "DEJA_KIMCHI_ROOT", "DEJA_KIMI_ROOT",
	"DEJA_KIRO_DB", "DEJA_KIRO_ROOT", "DEJA_OMP_ROOT", "DEJA_OPENCLAW_ROOT", "DEJA_OPENCODE_DB", "DEJA_OPENCODE_DIFFS",
	"DEJA_PI_ROOT", "DEJA_PRIME_ROOT", "DEJA_QWEN_ROOT", "DEJA_REASONIX_ROOT", "DEJA_ROO_CLI_ROOT", "DEJA_ROO_ROOTS",
	"DEJA_SENPI_ROOT", "DEJA_XCODE_CLAUDE_ROOT", "DEJA_XCODE_CODEX_ROOT", "DEJA_ZCODE_DB", "DEJA_ZCODE_LEGACY_ROOT",
	"DEJA_ZCODE_ROOT", "DEJA_ZED_DB", "DEJA_ZED_ROOT",
}

// parityEnv isolates a test to the stores it names (env var to a path under
// the returned root) and returns that root.
func parityEnv(t *testing.T, stores map[string]string) string {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	setHome(t, home)
	t.Setenv("HERMES_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("DEJA_NOTES_FILE", filepath.Join(home, "notes.jsonl"))
	t.Setenv("DEJA_INCLUDE_SUBAGENTS", "")
	for _, e := range parityStores {
		t.Setenv(e, filepath.Join(tmp, "absent", strings.ToLower(e)))
	}
	for k, v := range stores {
		t.Setenv(k, filepath.Join(tmp, v))
	}
	return tmp
}

func parityPass(t *testing.T, dir string, force bool) {
	t.Helper()
	if err := Ensure(dir, "", force, io.Discard); err != nil {
		t.Fatalf("Ensure(%s, force=%v): %v", dir, force, err)
	}
}

func parityWrite(t *testing.T, path, body string, appendTo bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if appendTo {
		flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(body); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func paritySQL(t *testing.T, db, stmts string) {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sqlite3", db)
	cmd.Stdin = strings.NewReader(stmts)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 %s: %v\n%s", db, err, out)
	}
}

// paritySnapshot is what a reader can see of an index: each session's row
// and records, and the two command tables.
func paritySnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	metas, err := AllMeta(dir)
	if err != nil {
		t.Fatalf("AllMeta %s: %v", dir, err)
	}
	out := map[string]string{}
	var ids []Identity
	for _, m := range metas {
		out[m.Harness+":"+m.ID] = fmt.Sprintf("title=%q project=%q counted=%d asked=%v words=%d touched=%v",
			m.Title, m.Project, m.Counted, m.Asked, m.Words, m.Touched)
		ids = append(ids, Identity{Harness: m.Harness, ID: m.ID})
	}
	full, err := FindManyByIdentity(dir, ids)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range full {
		var msgs []string
		for _, m := range s.Messages {
			msgs = append(msgs, m.Role+"|"+m.Time.UTC().Format(time.RFC3339)+"|"+m.Text)
		}
		out[s.Harness+":"+s.ID] += "\n    " + strings.Join(msgs, "\n    ")
	}
	out["commands"] = fmt.Sprint(ReadCommands(dir))
	out["fails"] = fmt.Sprint(ReadCommandFails(dir))
	out["facts"] = fmt.Sprint(ReadSessionFacts(dir))
	return out
}

// sameAsRebuild fails the test where the index in inc differs from a fresh
// build of the same stores.
func sameAsRebuild(t *testing.T, inc string) {
	t.Helper()
	fresh := inc + "-fresh"
	parityPass(t, fresh, true)
	a, b := paritySnapshot(t, inc), paritySnapshot(t, fresh)
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	var diffs []string
	for k := range keys {
		if a[k] != b[k] {
			diffs = append(diffs, fmt.Sprintf("%s\n  incremental: %s\n  rebuild:     %s", k, a[k], b[k]))
		}
	}
	sort.Strings(diffs)
	if len(diffs) > 0 {
		t.Errorf("incremental index differs from a rebuild:\n%s", strings.Join(diffs, "\n"))
	}
}

func parityClaudeLine(id, ts, role, content string) string {
	return fmt.Sprintf(`{"type":%q,"sessionId":%q,"timestamp":"2026-07-17T09:%s","cwd":"/tmp/proj","message":{"role":%q,"content":%s}}`, role, id, ts, role, content) + "\n"
}

// A command left in one session after an update rewrote the other is no
// longer a recurring one; the recomputed table was empty and the old one
// stayed (#4441).
func TestUpdateThatEmptiesTheCommandTableDropsIt(t *testing.T) {
	tmp := parityEnv(t, map[string]string{"DEJA_CLAUDE_ROOT": "claude"})
	proj := filepath.Join(tmp, "claude", "-tmp-proj")
	session := func(id string, withCommand bool) {
		body := parityClaudeLine(id, "00:00Z", "user", `"fix the retry loop"`)
		if withCommand {
			body += parityClaudeLine(id, "00:01Z", "assistant", `[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"make flakycheck"}}]`) +
				parityClaudeLine(id, "00:02Z", "user", `[{"type":"tool_result","tool_use_id":"t1","content":"ok retry 0.1s"}]`)
		}
		parityWrite(t, filepath.Join(proj, id+".jsonl"), body, false)
	}
	session("s1", true)
	session("s2", true)
	inc := filepath.Join(tmp, "inc")
	parityPass(t, inc, false)
	if len(ReadCommands(inc)) == 0 {
		t.Fatal("control: two sessions running one command make a table")
	}
	session("s2", false)
	later := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(filepath.Join(proj, "s2.jsonl"), later, later)
	parityPass(t, inc, false)
	if got := ReadCommands(inc); len(got) != 0 {
		t.Errorf("command table after the update = %v, want none", got)
	}
	sameAsRebuild(t, inc)
}

// A line the client writes after a pass took its file state is read in that
// pass and again in the next one, unless the pass stops where it recorded
// (#4442). The append is made between the walk and the parse here, which is
// the window a live client writes into.
func TestLineWrittenDuringAPassIsReadOnce(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(fmt.Sprint("first-build=", full), func(t *testing.T) {
			tmp := parityEnv(t, map[string]string{"DEJA_PI_ROOT": "pi"})
			f := filepath.Join(tmp, "pi", "--tmp-proj--", "s-retry.jsonl")
			line := func(id, ts, text string) string {
				return fmt.Sprintf(`{"type":"message","id":%q,"timestamp":"2026-09-01T09:%s","message":{"role":"user","content":[{"type":"text","text":%q}]}}`, id, ts, text) + "\n"
			}
			parityWrite(t, f, `{"type":"session","version":3,"id":"s-retry","timestamp":"2026-09-01T09:00:00Z","cwd":"/tmp/proj"}`+"\n"+line("u1", "00:01Z", "fix the retry loop"), false)
			inc := filepath.Join(tmp, "inc")
			if !full {
				parityPass(t, inc, false)
				parityWrite(t, f, line("u2", "01:00Z", "now run the tests"), true)
			}
			want := currentFiles("")
			parityWrite(t, f, line("u3", "02:00Z", "written while the pass ran"), true)
			if full {
				if err := rebuild(inc, "", "", want, io.Discard); err != nil {
					t.Fatal(err)
				}
			} else if err := updateIndex(inc, "", "", want, false, io.Discard); err != nil {
				t.Fatal(err)
			}
			parityPass(t, inc, false)
			sameAsRebuild(t, inc)
		})
	}
}
