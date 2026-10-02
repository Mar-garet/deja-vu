package index

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/vshulcz/deja-vu/internal/search"
)

// dsh 2.0.15 migrates a session into session.v4.jsonl beside its old log and
// appends there from then on. The incremental pass has to move the session onto
// the new file: the old one stopped at the upgrade (#4600).
func TestDeepSeekV4LogTakesOverFromTheLegacyLog(t *testing.T) {
	tmp := t.TempDir()
	setHome(t, tmp)
	root := filepath.Join(tmp, "dsh", "sessions")
	t.Setenv("DSH_HOME", filepath.Join(tmp, "dsh"))
	t.Setenv("DEJA_DEEPSEEK_ROOT", root)
	dir := filepath.Join(root, "--work-pgbouncer-lab--", "session-7c2e9f40-3b1a-4d5e-8f60-1a2b3c4d5e6f")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"type":"session","version":0,"id":"session-7c2e9f40-3b1a-4d5e-8f60-1a2b3c4d5e6f","createdAt":1790506922540,"cwd":"/work/pgbouncer-lab","delegationDepth":0}
{"type":"user/message","seq":1,"time":1790506922600,"data":{"content":[{"type":"text","text":"oldneedle what holds the pool size"}],"source":{"kind":"user"},"role":"user"}}
`)
	if err := os.WriteFile(filepath.Join(dir, "session.jsonl"), legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	idx := filepath.Join(tmp, "index.db")
	if err := Ensure(idx, "", true, nil); err != nil {
		t.Fatal(err)
	}

	v4 := []byte(`{"type":"session","version":4,"id":"session-7c2e9f40-3b1a-4d5e-8f60-1a2b3c4d5e6f","createdAt":1790506922540,"cwd":"/work/pgbouncer-lab","isSeeded":false,"delegationDepth":0}
{"type":"user/message","seq":1,"time":1790506922600,"data":{"content":[{"type":"text","text":"oldneedle what holds the pool size"}],"source":{"kind":"user"},"role":"user"}}
{"type":"user/message","seq":2,"time":1790593322600,"data":{"content":[{"type":"text","text":"v4needle raise it to 60"}],"source":{"kind":"user"},"role":"user"}}
`)
	if err := os.WriteFile(filepath.Join(dir, "session.v4.jsonl"), v4, 0o644); err != nil {
		t.Fatal(err)
	}
	var progress bytes.Buffer
	if err := Ensure(idx, "", false, &progress); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"oldneedle", "v4needle"} {
		ss, err := Search(idx, search.Options{Query: q, All: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(ss) != 1 {
			t.Errorf("%q is in %d sessions, want 1\n%s", q, len(ss), progress.String())
		}
	}
	if n := len(sessionsOf(t, idx, "deepseek")); n != 1 {
		t.Errorf("%d deepseek sessions, want 1", n)
	}
}
