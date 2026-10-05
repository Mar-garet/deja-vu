package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// CodeBuddy Code takes MCP servers and hooks the way Claude Code does, in its
// own files.
//
// MCP: user-scope servers live in the first of <config>/.mcp.json,
// <config>/mcp.json and <base>/.codebuddy.json that exists, else the first,
// where <config> is $CODEBUDDY_CONFIG_DIR or ~/.codebuddy and <base> is
// $CODEBUDDY_CONFIG_DIR or the home directory (PathUtils.getMcpCandidatePaths
// and resolveMcpFilePath in @tencent-ai/codebuddy-code 2.161.2, and the CLI's
// MCP doc). Only that one file is read, so deja writes into it rather than a
// new one the existing servers would shadow. An entry with a command and no
// type is taken as stdio.
//
// Hooks: <config>/settings.json, {"hooks":{Event:[{matcher, hooks:[{type,
// command, timeout}]}]}}, timeout in seconds. UserPromptSubmit and
// SessionStart take hookSpecificOutput.additionalContext; PostToolUse and
// PostToolUseFailure take it too when hookEventName names the event, and put
// it beside the tool result — all as Claude Code does, which is the shape
// deja's hooks already answer in.

func codeBuddyMCPPath() string {
	cfg := sources.CodeBuddyConfigDir()
	base := homeDir()
	if v := strings.TrimSpace(os.Getenv("CODEBUDDY_CONFIG_DIR")); v != "" {
		base = v
	}
	candidates := []string{
		filepath.Join(cfg, ".mcp.json"),
		filepath.Join(cfg, "mcp.json"),
		filepath.Join(base, ".codebuddy.json"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return candidates[0]
}

func codeBuddySettingsPath() string {
	return filepath.Join(sources.CodeBuddyConfigDir(), "settings.json")
}

// codeBuddyHookWiring is every event deja installs into CodeBuddy. Bash and
// PowerShell are CodeBuddy's shell tools by those names. PreToolUse is left
// out: what CodeBuddy does with a PreToolUse hook's context is not checked.
var codeBuddyHookWiring = []struct{ Event, Sub, Matcher string }{
	{"SessionStart", "hook-context", ""},
	{"UserPromptSubmit", "hook-prompt", ""},
	{"PostToolUse", "hook-tool-after", "Bash|PowerShell"},
	{"PostToolUseFailure", "hook-tool-after", "Bash|PowerShell"},
	{"PreCompact", "hook-precompact", ""},
	{"SessionEnd", "hook-session-end", ""},
}

// codeBuddyHookTimeout is in seconds, CodeBuddy's default.
const codeBuddyHookTimeout = 60

func installCodeBuddyMCP(exe string, uninstall bool) (installResult, error) {
	return installMCPJSON(codeBuddyMCPPath(), exe, uninstall)
}

func installCodeBuddyHooks(exe string, uninstall bool) (installResult, error) {
	exe = hookExeFor(exe, uninstall)
	path := codeBuddySettingsPath()
	var res installResult
	for i, h := range codeBuddyHookWiring {
		r, err := installSettingsHookCmd(path, h.Event, h.Matcher, codeBuddyHookTimeout, hookRun(exe, h.Sub), uninstall)
		if err != nil {
			return installResult{}, err
		}
		if i == 0 || (res.Action == "unchanged" && r.Action != "unchanged") {
			res = r
		}
	}
	return res, nil
}

// installCodeBuddyAuto writes the hooks first: a settings file deja refuses
// should leave nothing half-wired (#2745).
func installCodeBuddyAuto(exe string, uninstall bool) (installResult, error) {
	hooks, err := installCodeBuddyHooks(exe, uninstall)
	if err != nil {
		return installResult{}, err
	}
	mcp, err := installCodeBuddyMCP(exe, uninstall)
	if err != nil {
		return installResult{}, err
	}
	return wroteAll(hooks, mcp), nil
}
