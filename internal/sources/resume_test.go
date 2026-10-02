package sources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A line longer than the chunk the walk reads back is still handed over
// whole, and the walk stops where fn says.
func TestEachLineBeforeReadsLongLinesWhole(t *testing.T) {
	long := strings.Repeat("x", 70<<10)
	body := "first\n" + long + "\nthird\n"
	p := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var got []string
	eachLineBefore(p, int64(len(body)), func(line []byte) bool {
		got = append(got, string(line))
		return true
	})
	if len(got) != 3 || got[0] != "third" || got[1] != long || got[2] != "first" {
		t.Fatalf("got %d lines, first %.10q", len(got), got)
	}

	got = nil
	eachLineBefore(p, int64(len("first\n")+len(long)+1), func(line []byte) bool {
		got = append(got, string(line))
		return false
	})
	if len(got) != 1 || got[0] != long {
		t.Fatalf("stopped walk got %d lines", len(got))
	}
}
