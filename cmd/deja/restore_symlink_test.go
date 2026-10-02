package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// On macOS /tmp is a symlink to /private/tmp, and an agent may record either
// spelling. `deja restore /private/tmp/…/retry.go` found nothing for an edit
// recorded as /tmp/…/retry.go, while the other way round matched only because
// one spelling is a suffix of the other. Both sides are compared with their
// symlinks resolved, and the -o guard does the same, so the file a span came
// from cannot be overwritten through its other name.
func TestRestoreMatchesAPathThroughASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	tmp := hermeticEnv(t)
	real := filepath.Join(tmp, "private", "tmp", "proj")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "tmp")
	if err := os.Symlink(filepath.Join(tmp, "private", "tmp"), link); err != nil {
		t.Fatal(err)
	}
	// The edit is recorded under the link; the file itself is gone, which is
	// when restore is reached for.
	recorded := filepath.Join(link, "proj", "retry.go")
	proj := filepath.Join(tmp, "claude", "project")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"type":"assistant","sessionId":"claude-edit","cwd":%q,"timestamp":"2026-01-02T03:05:00Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":%q,"old_string":"const ceiling = 9"}}]}}`,
		filepath.Join(link, "proj"), recorded)
	if err := os.WriteFile(filepath.Join(proj, "s.jsonl"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The control: the spelling the edit was recorded under.
	out, err := captureRun(t, "restore", recorded)
	if err != nil || !strings.Contains(out, "1 replaced span") {
		t.Fatalf("the recorded spelling finds nothing, so this measures nothing: %v\n%s", err, out)
	}
	out, err = captureRun(t, "restore", filepath.Join(real, "retry.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1 replaced span") {
		t.Errorf("the resolved spelling of the same file found no span:\n%s", out)
	}

	// Written back over its own file, under the name the edit did not use.
	if err := os.WriteFile(filepath.Join(real, "retry.go"), []byte("live work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = captureRun(t, "restore", recorded, "-o", filepath.Join(real, "retry.go"), "--force")
	if err == nil || !strings.Contains(err.Error(), "refusing to write over") {
		t.Errorf("restore wrote over the file the span came from through its other name: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(real, "retry.go")); string(b) != "live work\n" {
		t.Errorf("the live file was overwritten: %q", b)
	}
}
