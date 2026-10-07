package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// opencode builds every model call afresh from its store, so recall added to
// the user's message lasted one call: after the agent's first tool call it was
// gone, and deja, having shown it once, would not send it again (#4786). Each
// plugin is driven here through three calls the way opencode makes them: the
// turn's first, the one after a tool ran, and the next turn's.
func TestOpencodePromptRecallOutlivesTheFirstCall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub deja is a shell script")
	}
	for _, shape := range []string{"1.x", "2.x"} {
		t.Run(shape, func(t *testing.T) {
			dir := t.TempDir()
			bin, calls := filepath.Join(dir, "deja"), filepath.Join(dir, "calls")
			// Like deja: a block it has shown in this session is not sent again.
			shown := filepath.Join(dir, "shown")
			script := "#!/bin/sh\nin=$(cat)\nprintf '%s %s\\n' \"$1\" \"$in\" >> " + calls + "\n" +
				"if [ \"$1\" = hook-prompt ] && [ ! -e " + shown + " ]; then touch " + shown +
				"; echo '{\"hookSpecificOutput\":{\"additionalContext\":\"RECALL-DANA\"}}'; fi\n"
			if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			plugin := filepath.Join(dir, "deja.mjs")
			src, driver := opencodeLegacyPluginJS(bin), `
import { execSync } from "node:child_process";
import { DejaRecall } from "`+plugin+`";
const q = (v) => "'" + String(v).replace(/'/g, "'\\''") + "'";
const $ = (strings, ...values) => {
  const line = strings.reduce((acc, s, i) => acc + s + (i < values.length ? q(values[i]) : ""), "");
  const run = () => { try { return execSync(line, { shell: "/bin/sh" }).toString() } catch { return "" } };
  return { text: async () => run(), quiet: async () => { run() } };
};
const h = await DejaRecall({ $, client: { tui: { showToast: async () => {} } }, directory: "`+dir+`" });
const user = (id, text) => ({ info: { id, role: "user", sessionID: "ses_P" }, parts: [{ type: "text", text }] });
const reply = { info: { id: "m2", role: "assistant", sessionID: "ses_P" }, parts: [{ type: "text", text: "reading go.mod" }] };
const call = async (msgs) => { await h["experimental.chat.messages.transform"]({ sessionID: "ses_P" }, { messages: msgs }); return msgs };
const a = await call([user("m1", "who reviews the pgx PR?")]);
const b = await call([user("m1", "who reviews the pgx PR?"), reply]);
const c = await call([user("m1", "who reviews the pgx PR?"), reply, user("m3", "and the bulk import?")]);
console.log(JSON.stringify([a[0].parts[0].text, b[0].parts[0].text, c[0].parts[0].text, c[2].parts[0].text]));
`
			if shape == "2.x" {
				src, driver = opencodePluginJS(bin), `
import plugin from "`+plugin+`";
const hooks = { session: {}, tool: {} };
const domain = (name) => ({ hook: async (event, fn) => { (hooks[name][event] ||= []).push(fn) } });
await plugin.setup({ location: { directory: "`+dir+`" }, session: domain("session"), tool: domain("tool") });
const user = (id, text) => ({ id, role: "user", content: [{ type: "text", text }] });
const reply = { id: "m2", role: "assistant", content: [{ type: "text", text: "reading go.mod" }] };
const call = async (msgs) => { await hooks.session.context[1]({ sessionID: "ses_P", system: [], messages: msgs }); return msgs };
const a = await call([user("m1", "who reviews the pgx PR?")]);
const b = await call([user("m1", "who reviews the pgx PR?"), reply]);
const c = await call([user("m1", "who reviews the pgx PR?"), reply, user("m3", "and the bulk import?")]);
console.log(JSON.stringify([a[0].content[0].text, b[0].content[0].text, c[0].content[0].text, c[2].content[0].text]));
`
			}
			if err := os.WriteFile(plugin, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			out := runNode(t, dir, driver)
			for i, when := range []string{"on the turn's first call", "after the agent ran a tool", "on the next turn"} {
				if strings.Count(strings.SplitN(out, `","`, 4)[i], "RECALL-DANA") != 1 {
					t.Errorf("%s the question does not carry its recall exactly once:\n%s", when, out)
				}
			}
			if strings.Contains(strings.SplitN(out, `","`, 4)[3], "RECALL-DANA") {
				t.Errorf("the next question was handed the first one's recall:\n%s", out)
			}
			if n := len(stubCalls(calls, "hook-prompt")); n != 2 {
				t.Errorf("hook-prompt ran %d times for two questions over three calls", n)
			}
		})
	}
}
