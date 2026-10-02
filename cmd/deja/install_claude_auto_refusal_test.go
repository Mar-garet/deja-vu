package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// With ~/.claude read-only, claude-auto was reported refused after it had
// already wired the MCP server into ~/.claude.json, which lives outside that
// directory, and left a .bak beside it (#4560). The halves that can fail on the
// directory go first now.
func TestClaudeAutoRefusedOnAReadOnlyDirWritesNothing(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("the directory mode is not enforced here")
	}
	hermeticEnv(t)
	dir := sources.ClaudeConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	p := sources.ClaudeJSONPath()
	const seed = "{\n  \"numStartups\": 3\n}\n"
	if err := os.WriteFile(p, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "install", "claude-auto", "--no-index", "--no-guidance"); err == nil {
		t.Fatal("install wrote into a read-only ~/.claude")
	}
	if got, _ := os.ReadFile(p); string(got) != seed {
		t.Errorf("a refused target wired ~/.claude.json:\n%s", got)
	}
	if _, err := os.Stat(p + ".bak"); err == nil {
		t.Errorf("a refused target left %s.bak", p)
	}
}

// And a read-only ~/.claude.json stops the target before ~/.claude is touched.
func TestClaudeAutoRefusedOnAReadOnlyClaudeJSONWritesNothing(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("the file mode is not enforced here")
	}
	hermeticEnv(t)
	p := sources.ClaudeJSONPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{}\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "install", "claude-auto", "--no-index", "--no-guidance"); err == nil {
		t.Fatal("install wrote through a read-only ~/.claude.json")
	}
	if _, err := os.Stat(sources.ClaudeConfigDir()); err == nil {
		t.Errorf("a refused target wrote into %s", sources.ClaudeConfigDir())
	}
}
