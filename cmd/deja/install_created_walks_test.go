package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func countEvalSymlinks(t *testing.T) *int {
	t.Helper()
	n := 0
	orig := evalSymlinks
	evalSymlinks = func(p string) (string, error) {
		n++
		return orig(p)
	}
	t.Cleanup(func() { evalSymlinks = orig })
	return &n
}

// Each write compared its path against every config the run had created by
// resolving all of them, so an install of n configs walked n² paths. A file
// with another name cannot resolve to this one.
func TestWriteResolvesOnlyCreatedFilesOfTheSameName(t *testing.T) {
	dir := t.TempDir()
	createdByThisRun = nil
	t.Cleanup(func() { createdByThisRun = nil; snapshotsByThisRun = nil })
	for i := range 50 {
		p := filepath.Join(dir, fmt.Sprintf("config-%d.json", i))
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		createdByThisRun = append(createdByThisRun, p)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	walks := countEvalSymlinks(t)
	if _, err := backupOnceUnlessCreated(path); err != nil {
		t.Fatal(err)
	}
	if *walks != 0 {
		t.Errorf("resolved %d created files to compare against one write, want 0", *walks)
	}
}

// A file created under a linked directory is still deja's own when the write
// arrives with the resolved name.
func TestWriteKnowsACreatedFileThroughALinkedDirectory(t *testing.T) {
	tmp := t.TempDir()
	real := filepath.Join(tmp, "dotfiles", "cfg")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "cfg")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := os.WriteFile(filepath.Join(real, "mcp.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	createdByThisRun = []string{filepath.Join(link, "other.json"), filepath.Join(link, "mcp.json")}
	t.Cleanup(func() { createdByThisRun = nil; snapshotsByThisRun = nil })
	resolved, err := filepath.EvalSymlinks(filepath.Join(link, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	took, err := backupOnceUnlessCreated(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if took {
		t.Error("took a snapshot of a file this run created, reached through a linked directory")
	}
}
