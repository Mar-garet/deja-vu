package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An install and uninstall of goose-auto gives the reader's AGENTS.md back
// byte for byte: the block went in after a blank line and came out without
// it, two newlines longer. Install also said it created a file that was
// there, and the closing line left out the snapshot it took of it (#4269).
func TestGooseAutoRoundTripGivesAGENTSmdBack(t *testing.T) {
	hermeticEnv(t)
	dir := gooseConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	hints := gooseHintsPath()
	want := "Always run tests before saying done.\n"
	if err := os.WriteFile(hints, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("GOOSE_PROVIDER: openai\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut := captureBoth(t, "install", "goose-auto", "--no-index")
	if strings.Contains(out+errOut, "created "+shortHome(hints)) {
		t.Errorf("install says it created %s, which was there:\n%s%s", shortHome(hints), out, errOut)
	}
	out, errOut = captureBoth(t, "uninstall", "goose-auto")
	got, err := os.ReadFile(hints)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("AGENTS.md after the round trip = %q, want %q", got, want)
	}
	baks, _ := filepath.Glob(filepath.Join(dir, "*.bak"))
	if len(baks) > 0 && !strings.Contains(out+errOut, "kept "+itoa(len(baks))+" snapshot") {
		t.Errorf("%d snapshots beside goose's files, the closing line says:\n%s%s", len(baks), out, errOut)
	}
}

// The block comes out with the separator it went in with, wherever it sits.
func TestGooseRecallBlockComesOutWhole(t *testing.T) {
	hermeticEnv(t)
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	for _, doc := range []string{"one\n", "# notes\n\nkeep this\n", "one\n\n\n"} {
		if err := os.WriteFile(path, []byte(gooseRecallBlock(doc, "recall\n")), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := dropGooseRecallBlock(path); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(path)
		// The writer trims the reader's trailing blank lines before its own
		// separator, so those cannot come back; the text and its last newline
		// do.
		if want := strings.TrimRight(doc, "\n") + "\n"; string(got) != want {
			t.Errorf("%q came back as %q, want %q", doc, got, want)
		}
	}
	// Text the reader added after deja's block stays, one blank line above it.
	installed := gooseRecallBlock("one\n", "recall\n") + "\ntwo\n"
	if err := os.WriteFile(path, []byte(installed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := dropGooseRecallBlock(path); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "one\n\ntwo\n" {
		t.Errorf("block between the reader's lines came out as %q", got)
	}
	// A refresh rewrites the block with the blank line after it gone; the
	// reader's two paragraphs still come back apart, not joined into one.
	refreshed := gooseRecallBlock(installed, "newer recall\n")
	if err := os.WriteFile(path, []byte(refreshed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := dropGooseRecallBlock(path); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "one\n\ntwo\n" {
		t.Errorf("block between the reader's lines, after a refresh, came out as %q", got)
	}
}
