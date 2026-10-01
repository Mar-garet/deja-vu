package sources

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// zstdFrame compresses one chunk as a frame of its own, the way dsh appends
// its log.
func zstdFrame(t *testing.T, chunk string) []byte {
	t.Helper()
	cmd := exec.Command("zstd", "-q", "-c")
	cmd.Stdin = strings.NewReader(chunk)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("zstd: %v", err)
	}
	return out
}

// tornLog is body one frame per line, then one more frame cut short: what a
// log looks like when its writer died mid-frame, or when it is read mid-append.
func tornLog(t *testing.T, body, tail string) []byte {
	t.Helper()
	var b bytes.Buffer
	for _, line := range strings.SplitAfter(body, "\n") {
		if line != "" {
			b.Write(zstdFrame(t, line))
		}
	}
	last := zstdFrame(t, tail)
	b.Write(last[:len(last)-7])
	return b.Bytes()
}

// One torn frame at the end of a dsh log dropped every complete frame before
// it, and doctor called the whole store unreadable (#4294).
func TestParseDeepSeekFileKeepsTheFramesBeforeATornOne(t *testing.T) {
	if !ZstdAvailable() {
		t.Skip("zstd CLI not installed")
	}
	tail := `{"type":"assistant/message","seq":50,"time":1787320264000,"data":{"message":{"role":"assistant","content":[{"type":"text","text":"and 60 on the replica"}]}}}` + "\n"
	dir := filepath.Join(t.TempDir(), "--work-pgbouncer-lab--", "session-torn")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "session.jsonl.zstd")
	if err := os.WriteFile(path, tornLog(t, deepSeekLog, tail), 0o644); err != nil {
		t.Fatal(err)
	}
	ss, err := ParseDeepSeekFile(path)
	if err != nil {
		t.Errorf("a torn tail failed the whole file: %v", err)
	}
	if len(ss) != 1 || len(ss[0].Messages) != 4 {
		t.Fatalf("the complete frames were not kept: %+v", ss)
	}
	if ss[0].Messages[3].Text != "держим 40 на шард" {
		t.Errorf("last complete answer = %q", ss[0].Messages[3].Text)
	}
	// The one file is named, not the store.
	if DiagMalformedCounts()[path] == 0 {
		t.Errorf("the torn file is not named in the ingest diagnostics")
	}
}

// The Codex .zst reader goes through the same CLI and had the same rule.
func TestCodexKeepsTheFramesBeforeATornOne(t *testing.T) {
	if !ZstdAvailable() {
		t.Skip("zstd CLI not installed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-thread-7.jsonl.zst")
	tail := `{"timestamp":"2026-07-17T09:00:03.000Z","type":"response_item","payload":{"role":"user","content":[{"type":"input_text","text":"and the retry"}]}}` + "\n"
	if err := os.WriteFile(path, tornLog(t, codexRolloutFixture, tail), 0o644); err != nil {
		t.Fatal(err)
	}
	ss, err := ParseCodexRollout(path)
	if err != nil {
		t.Errorf("a torn tail failed the whole rollout: %v", err)
	}
	if len(ss) != 1 || len(ss[0].Messages) != 2 {
		t.Fatalf("the complete frames were not kept: %+v", ss)
	}
	if DiagMalformedCounts()[path] == 0 {
		t.Errorf("the torn file is not named in the ingest diagnostics")
	}
}
