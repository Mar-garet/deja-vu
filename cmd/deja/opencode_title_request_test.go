package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The title request runs the system transform as well, and the 1.x plugin put
// the session digest into the title generator's prompt (#4795). Kilo sends it
// under "title-<session>"; opencode 1.18 under the session's own id, with the
// title agent's prompt first. Both shapes are driven here next to a session
// request that has to keep its digest.
func TestTheTitleRequestGetsNoDigest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub deja is a shell script")
	}
	for _, target := range []string{"opencode-auto", "kilocode-auto"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "deja")
			script := "#!/bin/sh\ncat >/dev/null\n" +
				"if [ \"$1\" = hook-context ]; then echo '{\"hookSpecificOutput\":{\"additionalContext\":\"DIGEST-MARK\"}}'; fi\n"
			if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			plugin := filepath.Join(dir, "deja.mjs")
			if err := os.WriteFile(plugin, []byte(legacyPluginJSFor(target, bin)), 0o644); err != nil {
				t.Fatal(err)
			}
			driver := `
import { execSync } from "node:child_process";
import { DejaRecall } from "` + plugin + `";
const q = (v) => "'" + String(v).replace(/'/g, "'\\''") + "'";
const $ = (strings, ...values) => {
  const line = strings.reduce((acc, s, i) => acc + s + (i < values.length ? q(values[i]) : ""), "");
  const run = () => { try { return execSync(line, { shell: "/bin/sh" }).toString() } catch { return "" } };
  return { text: async () => run(), quiet: async () => { run() } };
};
const h = await DejaRecall({ $, client: { tui: { showToast: async () => {} } }, directory: "` + dir + `" });
const call = async (sessionID, first) => { const o = { system: [first] }; await h["experimental.chat.system.transform"]({ sessionID }, o); return o.system.join("|") };
console.log(JSON.stringify([
  await call("title-ses_K", "You are Kilo."),
  await call("ses_O", "You are a title generator. You output ONLY a thread title. Nothing else."),
  await call("ses_O", "You are opencode, an interactive CLI tool."),
]));
`
			out := runNode(t, dir, driver)
			parts := strings.SplitN(out, `","`, 3)
			if len(parts) != 3 {
				t.Fatalf("driver printed %q", out)
			}
			for i, what := range []string{"Kilo's title request", "opencode's title request"} {
				if strings.Contains(parts[i], "DIGEST-MARK") {
					t.Errorf("%s carried the session digest: %s", what, parts[i])
				}
			}
			if !strings.Contains(parts[2], "DIGEST-MARK") {
				t.Errorf("the session's own request lost its digest: %s", out)
			}
		})
	}
}
