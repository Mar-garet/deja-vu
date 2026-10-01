package index

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A thread deja cannot decode reaches doctor as a skipped record of the zed
// store, with the reason. It used to leave no trace, so the store read as whole
// with half its threads missing (#4341).
func TestAZedThreadDejaCannotDecodeIsReportedWithWhy(t *testing.T) {
	tmp, db := zedEnv(t)
	zedThread(t, db, "good", "2026-01-01T10:00:00Z", []string{"marker-zed-good"})
	c := exec.Command("sqlite3", db)
	c.Stdin = strings.NewReader(`insert into threads (id,summary,updated_at,data_type,data) values ('later','s','2026-01-01T11:00:00Z','brotli',x'00');`)
	if o, err := c.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 seed: %v %s", err, o)
	}
	dir := filepath.Join(tmp, "index.db")
	if err := Ensure(dir, "", true, nil); err != nil {
		t.Fatal(err)
	}
	if zedHits(t, dir, "marker-zed-good") != 1 {
		t.Fatalf("the readable thread was not indexed, so this measures nothing")
	}
	if got := IngestHealth(dir)["zed"].MalformedLines; got != 1 {
		t.Fatalf("zed skipped = %d, want 1: %#v", got, IngestHealth(dir))
	}
	if r := IngestFilesReport(dir)[db].Reason; !strings.Contains(r, `unknown data_type "brotli"`) {
		t.Fatalf("reason = %q, want the unknown data_type named", r)
	}
}
