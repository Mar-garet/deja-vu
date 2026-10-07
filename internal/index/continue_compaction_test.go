package index

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// Continue compacts by rewriting the session file to the summary alone: on a
// stand with cn 1.5.47, seven items became one and the second turn was gone
// from disk. The index held those turns and threw them away on the next pass
// (#4795). They have to survive that pass, a second compaction, and a rebuild,
// with the summary filed under its own role.
func TestContinueTurnsSurviveTheCompactionThatRewroteTheirFile(t *testing.T) {
	tmp := hermeticIndexEnv(t)
	root := filepath.Join(tmp, "continue")
	t.Setenv("DEJA_CONTINUE_ROOT", root)
	p := filepath.Join(root, "sessions", "s1.json")
	idx := filepath.Join(tmp, "idx")
	stamp := time.Now().Add(-time.Hour)
	put := func(history string) {
		t.Helper()
		write(t, p, `{"sessionId":"s1","title":"t","workspaceDirectory":"/tmp/proj","history":[`+history+`]}`)
		stamp = stamp.Add(time.Minute)
		if err := os.Chtimes(p, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		if err := Ensure(idx, "", false, nil); err != nil {
			t.Fatal(err)
		}
	}
	held := func(dir string) []string {
		t.Helper()
		recs, err := ReadRecords(dir)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range recs {
			if r.Record.Key == "continue:s1" {
				out = append(out, r.Record.Role+": "+r.Record.Text)
			}
		}
		sort.Strings(out)
		return out
	}
	user := func(text string) string { return `{"message":{"role":"user","content":"` + text + `"}}` }
	said := func(text string) string { return `{"message":{"role":"assistant","content":"` + text + `"}}` }
	summary := func(text string) string {
		return `{"message":{"role":"assistant","content":"` + text + `"},"conversationSummary":"` + text + `"}`
	}

	put(user("why does zanzibarflux break the build") + "," + said("quokkaplex is stale") + "," + user("second turn TURN2zq"))
	put(summary("SUMMARY-ONE the build was fixed"))
	put(summary("SUMMARY-ONE the build was fixed") + "," + user("third turn TURN3zq"))
	put(summary("SUMMARY-TWO all of it"))

	want := []string{
		"assistant: quokkaplex is stale",
		sources.RoleSummary + ": SUMMARY-ONE the build was fixed",
		sources.RoleSummary + ": SUMMARY-TWO all of it",
		"user: second turn TURN2zq",
		"user: third turn TURN3zq",
		"user: why does zanzibarflux break the build",
	}
	if got := held(idx); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("after the compactions the index holds:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if err := Ensure(idx, "", true, nil); err != nil {
		t.Fatal(err)
	}
	if got := held(idx); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("a rebuild lost what the file no longer holds:\n%s", strings.Join(got, "\n"))
	}
}
