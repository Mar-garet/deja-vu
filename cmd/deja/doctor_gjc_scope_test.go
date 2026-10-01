package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gjc 0.18 keeps a `.gjc-managed-session-scope.v2.json` in every project
// directory, and its sub-agent passes sit under `<session>/N-*.jsonl`. The row
// counted both as transcripts it could not read: one "not recognised here" per
// project, plus one per pass deja skips on purpose (#4393).
func TestDoctorGjcRowKnowsItsScopeFilesAndPasses(t *testing.T) {
	tmp := hermeticEnv(t)
	root := filepath.Join(tmp, "gjc-sessions")
	t.Setenv("DEJA_GJC_ROOT", root)
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	session := func(dir, id, cwd string) string {
		p := filepath.Join(root, dir, "2026-10-01T21-00-00-000Z_"+id+".jsonl")
		write(p, `{"type":"session","version":3,"id":"`+id+`","cwd":"`+cwd+`","timestamp":"2026-10-01T21:00:00.000Z"}`+"\n"+
			`{"type":"message","id":"a1","timestamp":"2026-10-01T21:00:01.000Z","message":{"role":"user","content":[{"type":"text","text":"fix the retry loop"}]}}`+"\n")
		return p
	}
	for i, dir := range []string{"v2-xq3v", "v2-et2c"} {
		cwd := []string{"/tmp/proj", "/tmp/proj2"}[i]
		session(dir, "01a0f93c-0000-7000-8000-00000000000"+string(rune('1'+i)), cwd)
		write(filepath.Join(root, dir, ".gjc-managed-session-scope.v2.json"),
			`{"schemaVersion":1,"layoutVersion":2,"canonicalPath":"`+cwd+`","identityDigest":"x"}`)
	}
	parent := session("v2-xq3v", "01a0f93c-0000-7000-8000-000000000009", "/tmp/proj")
	write(filepath.Join(strings.TrimSuffix(parent, ".jsonl"), "0-explore.jsonl"), "{}\n")

	row := func() string {
		t.Helper()
		var buf bytes.Buffer
		doctorHarnesses(&buf, t.TempDir())
		for _, l := range strings.Split(buf.String(), "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "gjc ") {
				return l
			}
		}
		t.Fatalf("no gjc row:\n%s", buf.String())
		return ""
	}
	got := row()
	if strings.Contains(got, "not recognised") {
		t.Errorf("row = %q: the scope files and the pass are not transcripts deja failed to read", got)
	}
	if !strings.Contains(got, "3 files") {
		t.Errorf("row = %q, want the three sessions counted", got)
	}
	if !strings.Contains(got, "1 subagent transcripts skipped") {
		t.Errorf("row = %q, want the pass named as a skip", got)
	}
	// The note still does its job for a file deja really does not know.
	write(filepath.Join(root, "v2-xq3v", "something-new.json"), "{}")
	if got := row(); !strings.Contains(got, "1 not recognised here") {
		t.Errorf("row = %q, want the unknown file counted", got)
	}
}
