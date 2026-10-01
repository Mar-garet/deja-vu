package sources

import (
	"os"
	"path/filepath"
	"testing"
)

// The folder name blanks every character that is not a letter or digit to
// "-", and the resolver tried only "/" and a literal "-" in its place. A
// directory with "_", "." or a space in its name never resolved, so a Qwen
// transcript that records no cwd could not be resumed and a Claude project
// kept its encoded name (#4402).
func TestResolveEncodedPathMatchesABlankedCharacter(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"my_app", "app.v2", "two words", "a-b_c"} {
		dir := filepath.Join(tmp, "src_x", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := resolveEncodedPath(claudeEncodePath(dir)); got != dir {
			t.Errorf("%q resolved to %q", dir, got)
		}
	}
}
