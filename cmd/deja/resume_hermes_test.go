package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/model"
)

// A profile keeps its sessions in its own store, and `hermes --resume <id>`
// looks only in the active profile's: a session recorded under `hermes -p work`
// came back "Session not found" (#4248). A sticky `hermes profile use work` turns
// it around, and the root store then needs `-p default`.
func TestResumeHermesNamesTheProfileTheSessionIsIn(t *testing.T) {
	tmp := hermeticEnv(t)
	home := filepath.Join(tmp, "hermes")
	t.Setenv("DEJA_HERMES_HOME", home)
	t.Setenv("DEJA_HERMES_PROFILES_ROOT", "")
	const id = "20261001_182638_264fd2"
	cmdFor := func(path string) string {
		t.Helper()
		_, cmd, err := resumeCommand(model.Session{ID: id, Harness: "hermes", Project: "p", Path: path})
		if err != nil {
			t.Fatalf("resume %s: %v", path, err)
		}
		return cmd
	}
	profile := filepath.Join(home, "profiles", "work", "state.db")
	root := filepath.Join(home, "state.db")
	if got, want := cmdFor(profile), "hermes -p work --resume "+id; got != want {
		t.Errorf("profile session: %q, want %q", got, want)
	}
	if got, want := cmdFor(root), "hermes --resume "+id; got != want {
		t.Errorf("root session: %q, want %q", got, want)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "active_profile"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := cmdFor(root), "hermes -p default --resume "+id; got != want {
		t.Errorf("root session with work active: %q, want %q", got, want)
	}
	if got, want := cmdFor(profile), "hermes -p work --resume "+id; got != want {
		t.Errorf("profile session with work active: %q, want %q", got, want)
	}
}

// `hermes sessions delete` takes the row out of state.db while deja keeps the
// session searchable, and the printed `hermes --resume` then failed with
// "Session not found" (#4250) — the opencode case of #4205.
func TestResumeRefusesAHermesSessionDeletedInHermes(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not installed")
	}
	tmp := hermeticEnv(t)
	db := filepath.Join(tmp, "state.db")
	script := `create table sessions(id text primary key, cwd text);
create table messages(id integer primary key, session_id text, role text, content text, timestamp real);
insert into sessions values('20261001_181726_aaaaaa','/tmp/proj');
insert into messages values(1,'20261001_181726_aaaaaa','user','hello',1);`
	if out, err := exec.Command("sqlite3", db, script).CombinedOutput(); err != nil {
		t.Fatalf("seed: %v %s", err, out)
	}
	gone := model.Session{Harness: "hermes", ID: "20261001_181726_5aedef", Path: db}
	if err := resumeGoneError(gone); err == nil || !strings.Contains(err.Error(), "deja show") {
		t.Fatalf("a session hermes no longer has was offered for resume: %v", err)
	}
	kept := model.Session{Harness: "hermes", ID: "20261001_181726_aaaaaa", Path: db}
	if err := resumeGoneError(kept); err != nil {
		t.Fatalf("a session still in hermes was refused: %v", err)
	}
	// No store to ask: nothing is refused on a guess.
	missing := model.Session{Harness: "hermes", ID: "20261001_181726_5aedef", Path: filepath.Join(tmp, "missing.db")}
	if err := resumeGoneError(missing); err != nil {
		t.Fatalf("refused with no store to ask: %v", err)
	}
}
