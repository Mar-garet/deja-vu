package sources

import (
	"os"
	"path/filepath"
	"testing"
)

// WorkBuddy AI, the edition workbuddy.ai serves, keeps its home in
// ~/.workbuddy-ai (dataFolderName in the app's cli/product.json); the
// workbuddy.cn edition keeps ~/.workbuddy. Both stores are read.
func TestWorkBuddyAIStoreIsRead(t *testing.T) {
	home := codeBuddyEnv(t)
	intl := filepath.Join(home, ".workbuddy-ai", "projects", "repo", "s1.jsonl")
	writeCodeBuddyFile(t, intl, "{}\n")

	if got := CodeBuddySessionFiles(); len(got) != 1 || got[0] != intl {
		t.Fatalf("session files = %v, want [%s]", got, intl)
	}
	if !IsWorkBuddyTranscript(intl) || !IsCodeBuddyTranscript(intl) {
		t.Fatalf("%s not recognised as WorkBuddy's", intl)
	}
	if got := WorkBuddyConfigDir(); got != filepath.Join(home, ".workbuddy-ai") {
		t.Fatalf("WorkBuddyConfigDir = %q with only ~/.workbuddy-ai on disk", got)
	}

	cn := filepath.Join(home, ".workbuddy", "projects", "repo", "s2.jsonl")
	writeCodeBuddyFile(t, cn, "{}\n")
	if got := CodeBuddySessionFiles(); len(got) != 2 {
		t.Fatalf("both editions: session files = %v", got)
	}

	// WORKBUDDY_CONFIG_DIR names the one home, as it does for the app.
	moved := filepath.Join(home, "wb")
	t.Setenv("WORKBUDDY_CONFIG_DIR", moved)
	if err := os.MkdirAll(filepath.Join(moved, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := CodeBuddyRoots(); len(got) != 2 || got[1] != filepath.Join(moved, "projects") {
		t.Fatalf("WORKBUDDY_CONFIG_DIR roots = %v", got)
	}
}
