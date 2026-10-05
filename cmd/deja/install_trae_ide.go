package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// TRAE IDE is a VS Code fork with its agent in a native library, read off
// 3.5.104 (macOS). Its chats are in an encrypted database, so deja wires it
// and does not read it.
//
// MCP: <user data>/User/mcp.json, {"mcpServers":{"deja":{"command","args"}}}.
// The schema has additionalProperties false, so no type key. Writing the file
// while the IDE runs loads the server; the agent called deja from chat.
//
// Hooks: ~/<dataFolderName>/hooks.json, Claude's shape and event names. The
// IDE ships with hooks off: they run only once Settings > Hooks has global
// hooks on, and that switch is kept in the agent's own configuration store,
// not in a file deja can read. A live UserPromptSubmit payload carried
// session_id, cwd, prompt and workspace_roots. ~/.trae/hooks.json is also
// where TRAE CLI 1.x kept hooks; TRAE CLI 2.0 no longer reads it.
//
// Skill: <userHome>/<dataFolderName>/skills/<name>/SKILL.md, the global root
// the IDE's own skill installer names. TRAE CLI 0.208 lists that root too and
// shows a single deja-history when ~/.agents/skills holds the same skill.
//
// The CN build is the same app named "Trae CN", with ~/.trae-cn.

type traeIDEEdition struct{ app, dataFolder string }

var traeIDEEditions = []traeIDEEdition{{"Trae", ".trae"}, {"Trae CN", ".trae-cn"}}

// traeIDEAppDir is where an edition keeps its user data, in VS Code's layout.
func traeIDEAppDir(app string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(sources.Home(), "Library", "Application Support", app)
	case "windows":
		dir := os.Getenv("APPDATA")
		if dir == "" {
			dir = filepath.Join(sources.Home(), "AppData", "Roaming")
		}
		return filepath.Join(dir, app)
	}
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(sources.Home(), ".config")
	}
	return filepath.Join(cfg, app)
}

// traeIDE is the edition on this machine: the international one unless only
// the CN build has user data.
func traeIDE() traeIDEEdition {
	for _, e := range traeIDEEditions {
		if _, err := os.Stat(filepath.Join(traeIDEAppDir(e.app), "User")); err == nil {
			return e
		}
	}
	return traeIDEEditions[0]
}

func traeIDEUserDir() string { return filepath.Join(traeIDEAppDir(traeIDE().app), "User") }

func traeIDEMCPPath() string { return filepath.Join(traeIDEUserDir(), "mcp.json") }

func traeIDEHooksPath() string {
	return filepath.Join(sources.Home(), traeIDE().dataFolder, "hooks.json")
}

func traeIDESkillPath() string {
	return filepath.Join(sources.Home(), traeIDE().dataFolder, "skills", "deja-history", "SKILL.md")
}

// traeIDEHookWiring has no SessionEnd: the IDE fires no such event. The tool
// matchers carry the IDE's own tool names and Claude's, since which of the two
// a matcher is tested against is not measured yet.
var traeIDEHookWiring = []hookWire{
	{"SessionStart", "hook-context", ""},
	{"UserPromptSubmit", "hook-prompt", ""},
	{"PreToolUse", "hook-tool", "Edit|Write|write_to_file|update_file|edit_file_fast_apply"},
	{"PostToolUse", "hook-tool-after", "Bash|run_command"},
	{"PreCompact", "hook-precompact", ""},
}

// traeIDEHooksOffNote is said under the hooks wherever deja reports them: the
// switch cannot be read, and the IDE's default is off.
const traeIDEHooksOffNote = "TRAE IDE runs these only once hooks are on: Settings > Hooks, enable global hooks and run them locally"

func installTraeIDE(exe string, uninstall bool) (installResult, error) {
	return installMCPJSON(traeIDEMCPPath(), exe, uninstall)
}

// installTraeIDEAuto writes the hooks first: a hooks.json deja refuses should
// leave nothing half-wired (#2745). The IDE pins no trust, so no config path.
func installTraeIDEAuto(exe string, uninstall bool) (installResult, error) {
	if err := readableStrictJSON(traeIDEHooksPath()); err != nil {
		return installResult{}, err
	}
	hooks, err := installCodexHooksAt(exe, traeIDEHooksPath(), "", traeIDEHookWiring, uninstall)
	if err != nil {
		return installResult{}, err
	}
	mcp, err := installTraeIDE(exe, uninstall)
	if err != nil {
		return installResult{}, err
	}
	if !uninstall {
		fmt.Println("trae-ide: hooks are off by default in TRAE IDE — turn them on in Settings > Hooks (global hooks, run locally); until then only the MCP tool works")
	}
	return wroteAll(hooks, mcp), nil
}
