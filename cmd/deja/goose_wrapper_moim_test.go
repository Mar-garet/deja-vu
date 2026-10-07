package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// goose reads the MOIM file by path and ignores the session, so the wrapper's
// file has to be its own: with one path for every `deja goose`, two sessions
// in two projects read whichever recall was written last (#4795). And it goes
// with the session, or every run leaves a file behind.
func TestEachGooseWrapperHasItsOwnRecallFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub goose is a shell script")
	}
	hermeticEnv(t)
	t.Setenv("GOOSE_MOIM_MESSAGE_FILE", "")
	bin := t.TempDir()
	seen := filepath.Join(t.TempDir(), "seen")
	script := "#!/bin/sh\nprintf '%s\\n' \"$GOOSE_MOIM_MESSAGE_FILE\" >> " + seen + "\n" +
		"test -f \"$GOOSE_MOIM_MESSAGE_FILE\" && echo present >> " + seen + "\n"
	if err := os.WriteFile(filepath.Join(bin, "goose"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := cmdGoose(t.TempDir(), []string{"run", "--text", "hi"}, ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(seen)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 || lines[1] != "present" {
		t.Fatalf("goose did not find a recall file waiting:\n%s", b)
	}
	if got, want := lines[0], gooseWrapperMOIMPath(os.Getpid()); got != want {
		t.Errorf("goose was handed %s, want this process's own %s", got, want)
	}
	if _, err := os.Stat(lines[0]); !os.IsNotExist(err) {
		t.Errorf("the recall file outlived its session: %v", err)
	}
}
