package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// copilotHome is Copilot CLI's own directory. It honours COPILOT_HOME the way
// the CLI does: with it set, 1.0.79 keeps its config, sessions and logs there
// and never looks at ~/.copilot.
func copilotHome() string {
	if h := os.Getenv("COPILOT_HOME"); filepath.IsAbs(h) {
		return h
	}
	return filepath.Join(sources.Home(), ".copilot")
}

// copilotHooksPath is the file Copilot will read deja's hook from.
//
// Copilot CLI 1.0.79 keeps user settings in settings.json and rewrites
// config.json as a file it manages. On every start it moves the user keys it
// finds in config.json into settings.json, and a `hooks` key moves whole: it
// replaces the one in settings.json rather than merging into it. So while the
// reader's hooks are still in config.json, an entry deja put in settings.json
// is gone after one launch; written beside theirs, it moves with them.
func copilotHooksPath() string {
	config := filepath.Join(copilotHome(), "config.json")
	if b, err := os.ReadFile(config); err == nil {
		var root map[string]any
		if json.Unmarshal([]byte(jsoncToJSON(string(bytes.TrimPrefix(b, utf8BOM)))), &root) == nil {
			if _, ok := root["hooks"]; ok {
				return config
			}
		}
	}
	return filepath.Join(copilotHome(), "settings.json")
}

// copilotHookCommand is the line deja's sessionStart entry runs. --copilot
// makes hook-context answer in the only shape Copilot reads.
func copilotHookCommand(exe string) string {
	return hookRun(exe, "hook-context", "--copilot")
}

// installCopilotAuto wires Copilot CLI's sessionStart hook to the digest, with
// the MCP server beside it (#4231).
//
// Only sessionStart. Measured on 1.0.79, what it prints goes in front of the
// first request as a message of its own, is kept for every later turn of the
// session, and is not written into the user's message; userPromptSubmitted
// output is appended to the user's own turn instead. The payload names the
// session as `sessionId`, and `source` is "resume" when an old one is reopened.
func installCopilotAuto(exe string, uninstall bool) (installResult, error) {
	mcp, err := installCopilotMCP(exe, uninstall)
	if err != nil {
		return installResult{}, err
	}
	target := copilotHooksPath()
	var hooks installResult
	for _, path := range []string{filepath.Join(copilotHome(), "settings.json"), filepath.Join(copilotHome(), "config.json")} {
		// The other file is only ever cleared: an entry left there from an
		// earlier install would run twice, or be what the move overwrites.
		r, err := installCopilotHooks(path, exe, uninstall || path != target)
		if err != nil {
			return installResult{}, err
		}
		if path == target || r.Action != "unchanged" {
			hooks = r
		}
	}
	return wroteAll(mcp, hooks), nil
}

// copilotHookTimeoutSec bounds how long Copilot waits for the digest before
// the first request. It is served from a cache and answers in milliseconds;
// one that cannot is not worth holding someone's prompt for.
const copilotHookTimeoutSec = 10

func installCopilotHooks(path, exe string, uninstall bool) (installResult, error) {
	exe = hookExeFor(exe, uninstall)
	cmd := copilotHookCommand(exe)
	old, err := readConfig(path)
	if err != nil {
		return installResult{}, err
	}
	if uninstall && len(bytes.TrimSpace(old)) == 0 {
		return installResult{Path: path, Action: "unchanged"}, nil
	}
	jsonc := configIsJSONC(old)
	source := old
	if jsonc {
		source = []byte(jsoncToJSON(string(old)))
	}
	root := map[string]any{}
	if len(bytes.TrimSpace(source)) > 0 {
		if err := json.Unmarshal(source, &root); err != nil {
			return installResult{}, configParseError(path, err)
		}
		if root == nil {
			root = map[string]any{}
		}
	}
	before, _ := json.Marshal(root)
	hooks, isMap := root["hooks"].(map[string]any)
	if _, has := root["hooks"]; has && !isMap {
		if uninstall {
			return installResult{Path: path, Action: "unchanged"}, nil
		}
		return installResult{}, fmt.Errorf("%s: `hooks` is not an object — deja leaves it as it is", path)
	}
	if hooks == nil {
		if uninstall {
			return installResult{Path: path, Action: "unchanged"}, nil
		}
		hooks = map[string]any{}
		root["hooks"] = hooks
		noteBlockAdded(path, "hooks")
	}
	setCopilotHook(hooks, "sessionStart", cmd, uninstall)
	if len(hooks) == 0 && blockWasAdded(path, "hooks") {
		delete(root, "hooks")
		forgetBlockAdded(path, "hooks")
	}
	after, _ := json.Marshal(root)
	if string(after) == string(before) {
		return installResult{Path: path, Action: "unchanged"}, nil
	}
	if jsonc {
		return installResult{}, fmt.Errorf("%s: deja cannot edit hooks in a file that carries comments — add or remove the hook by hand, or take the comments out", path)
	}
	next, err := marshalConfigLike(old, root)
	if err != nil {
		return installResult{}, err
	}
	next = append(next, '\n')
	if uninstall {
		next = snapshotIfSame(path, root, next)
	}
	a, err := writeIfChanged(path, old, next)
	return installResult{Path: path, Action: a}, err
}

// snapshotIfSame gives back the snapshot deja took before its first write when
// what is left after taking deja out says the same thing. Marshalling keeps the
// top-level order and nothing below it, so Copilot's own
// {"type","bash","timeoutSec"} came back as {"bash","timeoutSec","type"}: the
// same settings, and a diff in the reader's dotfiles all the same.
func snapshotIfSame(path string, root map[string]any, next []byte) []byte {
	if !snapshotTaken(path) {
		return next
	}
	b, err := os.ReadFile(path + ".bak")
	if err != nil {
		return next
	}
	b = bytes.TrimPrefix(b, utf8BOM)
	var was map[string]any
	if json.Unmarshal([]byte(jsoncToJSON(string(b))), &was) != nil || !reflect.DeepEqual(was, root) {
		return next
	}
	return b
}

// setCopilotHook keeps one deja entry under an event and leaves every other
// one alone. Entries are flat — {"type","bash","timeoutSec"} — the same schema
// as a repository's .github/hooks/*.json.
func setCopilotHook(hooks map[string]any, event, cmd string, uninstall bool) {
	base := strings.TrimSuffix(cmd, " --copilot")
	entries, _ := hooks[event].([]any)
	var kept []any
	found := false
	for _, entryAny := range entries {
		entry, _ := entryAny.(map[string]any)
		kind := hookNotDejas
		if entry != nil {
			s, _ := entry["bash"].(string)
			// The flag is deja's own; without it the command is the plain
			// hook-context line, which answers in a shape Copilot ignores and
			// is taken over the same way.
			kind = hookCommandKindOf(strings.TrimSuffix(strings.TrimSpace(s), " --copilot"), base)
		}
		if kind == hookWrapsDejas {
			found = true
			kept = append(kept, entryAny)
			continue
		}
		if kind == hookDejas {
			if uninstall || found {
				continue
			}
			found = true
			entry["type"] = "command"
			entry["bash"] = cmd
			entry["timeoutSec"] = copilotHookTimeoutSec
			if runtime.GOOS == "windows" {
				entry["powershell"] = "& " + cmd
			}
		}
		kept = append(kept, entryAny)
	}
	if !uninstall && !found {
		entry := map[string]any{"type": "command", "bash": cmd, "timeoutSec": copilotHookTimeoutSec}
		// On Windows Copilot picks the powershell line when there is one;
		// `&` is what lets PowerShell run a quoted path.
		if runtime.GOOS == "windows" {
			entry["powershell"] = "& " + cmd
		}
		kept = append(kept, entry)
	}
	if len(kept) == 0 {
		delete(hooks, event)
		return
	}
	hooks[event] = kept
}

// copilotHooksDisabled reports whether Copilot's own switch turns every hook
// off, which leaves deja's entry looking installed and running nothing.
func copilotHooksDisabled() bool {
	for _, name := range []string{"settings.json", "config.json"} {
		b, err := os.ReadFile(filepath.Join(copilotHome(), name))
		if err != nil {
			continue
		}
		var root map[string]any
		if json.Unmarshal([]byte(jsoncToJSON(string(bytes.TrimPrefix(b, utf8BOM)))), &root) != nil {
			continue
		}
		if off, _ := root["disableAllHooks"].(bool); off {
			return true
		}
	}
	return false
}
