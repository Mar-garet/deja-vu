package sources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// aider writes its history at the git root it runs in, not in $HOME, so a
// project nobody listed in DEJA_AIDER_ROOTS was never read. `deja aider`
// records the directory it starts aider in (#4326).
func TestAiderFilesReadsRecordedProjects(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
	t.Setenv("DEJA_AIDER_ROOTS", "")
	t.Setenv("AIDER_CHAT_HISTORY_FILE", "")
	proj := t.TempDir()
	hist := filepath.Join(proj, ".aider.chat.history.md")
	if err := os.WriteFile(hist, []byte("# aider chat started at 2026-01-01 00:00:00\n#### q\nans\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := AiderFiles(); len(got) != 0 {
		t.Fatalf("nothing recorded yet, found %v", got)
	}
	for range 2 {
		if err := RecordAiderProject(proj); err != nil {
			t.Fatal(err)
		}
	}
	if got := AiderFiles(); len(got) != 1 || got[0] != hist {
		t.Fatalf("AiderFiles = %v, want [%s]", got, hist)
	}
	b, err := os.ReadFile(AiderProjectsPath())
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), proj); n != 1 {
		t.Fatalf("project recorded %d times, want once:\n%s", n, b)
	}
}
