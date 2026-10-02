package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A developer with opencode installed ran the suite and three tests went red:
// install asked the real `opencode --version`, which wrote its log under the
// test's home, and doctor judged the hand-written plugin by the installed major
// (#4155). The suite answers as a machine with no opencode unless a test says
// which one it wants.
func TestTheSuiteNeverAsksTheOpencodeOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	dir := t.TempDir()
	asked := filepath.Join(dir, "asked")
	stub := "#!/bin/sh\ntouch '" + asked + "'\necho 2.0.18\n"
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if got := opencodeVersionMajor(); got != 0 {
		t.Errorf("the suite read opencode %d from PATH, want 0", got)
	}
	if _, err := os.Stat(asked); err == nil {
		t.Error("the suite ran the opencode on PATH")
	}
}

// With the suite pinned to "no opencode", nothing else runs the probe itself:
// the PATH lookup and the version it parses are checked here, against a
// stand-in, so the pin does not hide a broken probe.
func TestTheOpencodeMajorIsReadFromTheBinaryOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	t.Setenv("DEJA_OPENCODE_MAJOR", "")
	for out, want := range map[string]int{"2.0.18": 2, "opencode 1.14.3": 1, "dev": 0} {
		dir := t.TempDir()
		stub := "#!/bin/sh\necho '" + out + "'\n"
		if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		if got := opencodeVersionMajorReal(); got != want {
			t.Errorf("opencode printing %q reads as major %d, want %d", out, got, want)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if got := opencodeVersionMajorReal(); got != 0 {
		t.Errorf("no opencode on PATH reads as major %d, want 0", got)
	}
	t.Setenv("DEJA_OPENCODE_MAJOR", "1")
	if got := opencodeVersionMajorReal(); got != 1 {
		t.Errorf("DEJA_OPENCODE_MAJOR=1 reads as major %d", got)
	}
}
