package main

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// The client's own off switches. Each client can turn deja off without
// removing it, and only the JSON entry flag (#4303) and two plugin switches
// were ever read, so every other way read `wired` while the client ran
// nothing. One probe per harness, read by the text report, --json and the
// install line alike, so the three cannot disagree. Each answer is the line to
// print, naming the switch and the file, and "" when nothing there is off.

// clientMCPEntryOff reads the switch on deja's own entry in a config that is
// not JSON: the JSON one is dejaEntrySwitchedOff's (#4466).
func clientMCPEntryOff(name string) string {
	switch name {
	case "goose":
		return yamlSwitchOff(filepath.Join(gooseConfigDir(), "config.yaml"), "goose", "extensions", "deja", "enabled")
	case "hermes":
		return yamlSwitchOff(filepath.Join(sources.HermesHome(), "config.yaml"), "hermes", "mcp_servers", "deja", "enabled")
	case "codex", "grok":
		home := sources.CodexHome()
		if name == "grok" {
			home = sources.GrokHome()
		}
		p := filepath.Join(home, "config.toml")
		if b, err := readConfig(p); err == nil && tomlDejaEntriesOff(string(b)) {
			return mcpOffNote(name, "`[mcp_servers.deja] enabled = false`", p)
		}
	case "deepseek":
		if b, err := readConfig(dshPatchPath()); err == nil && dshRowDisabled(string(b), "mcp-deja") {
			return mcpOffNote("dsh", "`disabled: true` on the mcp-deja row", dshPatchPath())
		}
	}
	return ""
}

// clientMCPDenied reads what a client keeps beside the entry: a deny list, an
// allow list without deja, a switch for every server, and the per-project
// lists claude-code and cursor-agent write for the directory doctor runs in
// (#4468).
func clientMCPDenied(name string) string {
	switch name {
	case "gemini", "qwen":
		home := sources.GeminiHome()
		if name == "qwen" {
			home = sources.QwenConfigDir()
		}
		settings := filepath.Join(home, "settings.json")
		root := readJSONConfig(settings)
		// Both compare the server key exactly. Qwen reads each entry as a
		// `*`/`?` pattern and an empty allow list as none; gemini starts
		// everything when the allow list is empty.
		listed := func(v any) bool {
			for _, s := range jsonStrings(v) {
				if s == "deja" || name == "qwen" && serverPatternMatch(s, "deja") {
					return true
				}
			}
			return false
		}
		if listed(jsonAt(root, "mcp", "excluded")) {
			return mcpOffNote(name, "`mcp.excluded` lists deja", settings)
		}
		if allowed, ok := jsonAt(root, "mcp", "allowed").([]any); ok && !listed(allowed) &&
			(name == "qwen" || len(jsonStrings(allowed)) > 0) {
			return mcpOffNote(name, "`mcp.allowed` does not list deja", settings)
		}
		// What `gemini mcp disable deja` writes, under the lowercased id it
		// looks the server up by.
		if name == "gemini" {
			p := filepath.Join(home, "mcp-server-enablement.json")
			if jsonAt(readJSONConfig(p), "deja", "enabled") == false {
				return mcpOffNote(name, "`deja.enabled: false`", p)
			}
		}
	case "copilot":
		p := filepath.Join(filepath.Dir(copilotMCPConfigPath()), "settings.json")
		if listHasDeja(jsonAt(readJSONConfig(p), "disabledMcpServers")) {
			return mcpOffNote(name, "`disabledMcpServers` lists deja", p)
		}
	case "grok":
		p := filepath.Join(sources.GrokHome(), "config.toml")
		if b, err := readConfig(p); err == nil {
			for _, s := range tomlTopLevelStrings(string(b), "disabled_mcp_servers") {
				if nameIsDeja(s) {
					return mcpOffNote(name, "`disabled_mcp_servers` lists deja", p)
				}
			}
		}
	case "omp", "gjc":
		p := filepath.Join(sources.OmpConfigDir(), "mcp.json")
		if name == "gjc" {
			p = gjcMCPPath()
		}
		if listHasDeja(jsonAt(readJSONConfig(p), "disabledServers")) {
			return mcpOffNote(name, "`disabledServers` lists deja", p)
		}
	case "vscode":
		p := filepath.Join(filepath.Dir(doctorVSCodeMCPPath()), "settings.json")
		root := readJSONConfig(p)
		if root["chat.mcp.access"] == "none" || root["chat.mcp.enabled"] == false {
			return mcpOffNote(name, "`chat.mcp.access` is off", p)
		}
	case "zcode":
		if jsonAt(readJSONConfig(zcodeConfigPath()), "features", "mcp") == false {
			return mcpOffNote(name, "`features.mcp: false`", zcodeConfigPath())
		}
	case "openclaw":
		p := filepath.Join(sources.OpenClawStateDir(), "openclaw.json")
		for _, s := range jsonStrings(jsonAt(readJSONConfig(p), "tools", "deny")) {
			if s == "bundle-mcp" || s == "*" {
				return mcpOffNote(name, "`tools.deny` holds "+s, p)
			}
		}
	case "opencode":
		// Tool names are server_tool, and a key is a glob over them.
		p := doctorOpencodeConfigPath()
		for k, v := range asMap(jsonAt(readJSONConfig(p), "tools")) {
			if ok, _ := path.Match(k, "deja_deja"); ok && v == false {
				return mcpOffNote(name, "`tools` turns "+k+" off", p)
			}
		}
	case "amp":
		p := sources.AmpSettingsFile()
		root := readJSONConfig(p)
		list := root["amp.tools.disable"]
		if list == nil {
			list = jsonAt(root, "amp", "tools", "disable")
		}
		for _, s := range jsonStrings(list) {
			if ok, _ := path.Match(s, "mcp__deja__deja"); ok {
				return mcpOffNote(name, "`amp.tools.disable` holds "+s, p)
			}
		}
	case "claude-code":
		projects := asMap(jsonAt(readJSONConfig(sources.ClaudeJSONPath()), "projects"))
		for _, dir := range doctorProjectDirs() {
			if listHasDeja(jsonAt(asMap(projects[dir]), "disabledMcpServers")) {
				return mcpOffNote(name, "`disabledMcpServers` for "+reportPath(dir)+" lists deja", sources.ClaudeJSONPath())
			}
		}
	case "cursor":
		for _, dir := range doctorProjectDirs() {
			p := filepath.Join(cursorDataDir(), "projects", cursorProjectSlug(dir), "mcp-disabled.json")
			if b, err := readConfig(p); err == nil && listHasDeja(parseJSONValue(b)) {
				return mcpOffNote(name, "deja is listed for this project", p)
			}
		}
	}
	return ""
}

// clientHooksOff reads the client's switch for every hook and its disable on
// deja's own extension or plugin, for the hook rows (#4469, #4470).
func clientHooksOff(name string) string {
	switch name {
	case "claude-code", "qwen":
		dir := sources.ClaudeConfigDir()
		if name == "qwen" {
			dir = sources.QwenConfigDir()
		}
		p := filepath.Join(dir, "settings.json")
		if readJSONConfig(p)["disableAllHooks"] == true {
			return hooksOffNote(name, "`disableAllHooks: true`", p)
		}
	case "gemini":
		p := filepath.Join(sources.GeminiHome(), "settings.json")
		if jsonAt(readJSONConfig(p), "hooksConfig", "enabled") == false {
			return hooksOffNote(name, "`hooksConfig.enabled: false`", p)
		}
		// What `gemini extensions disable deja` writes: rules over the
		// workspace path, the last one that matches winning.
		p = filepath.Join(sources.GeminiHome(), "extensions", "extension-enablement.json")
		if cwd, err := os.Getwd(); err == nil && !geminiExtensionEnabled(jsonStrings(jsonAt(readJSONConfig(p), "deja", "overrides")), cwd) {
			return pluginOffNote(name, "deja's extension is disabled for this directory", p)
		}
	case "codex-hook":
		p := filepath.Join(sources.CodexHome(), "config.toml")
		if b, err := readConfig(p); err == nil && tomlTableValue(string(b), "features", "hooks") == "false" {
			return hooksOffNote("codex", "`[features] hooks = false`", p)
		}
	case "openclaw":
		p := filepath.Join(sources.OpenClawStateDir(), "openclaw.json")
		root := readJSONConfig(p)
		switch {
		case jsonAt(root, "hooks", "internal", "enabled") == false:
			return hooksOffNote(name, "`hooks.internal.enabled: false`", p)
		case jsonAt(root, "hooks", "internal", "entries", openclawHookName, "enabled") == false:
			return pluginOffNote(name, "`hooks.internal.entries."+openclawHookName+".enabled: false`", p)
		}
		// The plugin is the per-prompt half; the hook pack above still runs
		// at bootstrap without it.
		if jsonAt(root, "plugins", "entries", openclawPluginID, "enabled") == false || listHasDeja(jsonAt(root, "plugins", "deny")) {
			return "switched off: deja's plugin, by `plugins.entries` or `plugins.deny` in " + reportPath(p) +
				" — openclaw will not run its recall on each prompt"
		}
	case "goose":
		p := filepath.Join(gooseConfigDir(), "settings.json")
		if listHasDeja(jsonAt(readJSONConfig(p), "disabledPlugins")) {
			return pluginOffNote(name, "`disabledPlugins` lists deja", p)
		}
	case "pi", "senpi":
		dir := sources.PiConfigDir()
		if name == "senpi" {
			dir = sources.SenpiConfigDir()
		}
		p := filepath.Join(dir, "settings.json")
		if s := piExtensionExcluded(jsonStrings(jsonAt(readJSONConfig(p), "extensions")), dir); s != "" {
			return pluginOffNote(name, "`"+s+"` in `extensions`", p)
		}
	case "omp", "gjc":
		dir := sources.OmpConfigDir()
		if name == "gjc" {
			dir = sources.GjcConfigDir()
		}
		p := filepath.Join(dir, "config.yml")
		if b, err := readConfig(p); err == nil {
			_, items, _ := yamlLookup(string(b), "disabledExtensions")
			for _, s := range items {
				if s == "extension-module:deja" || nameIsDeja(s) {
					return pluginOffNote(name, "`disabledExtensions` lists "+s, p)
				}
			}
		}
	case "cline":
		p := filepath.Join(sources.ClineConfigDir(), "settings", "global-settings.json")
		for _, s := range jsonStrings(jsonAt(readJSONConfig(p), "disabledPlugins")) {
			if nameIsDeja(s) || filepath.Clean(s) == filepath.Join(sources.ClinePluginsDir(), "deja") ||
				filepath.Clean(s) == filepath.Join(sources.ClinePluginsDir(), "deja", "index.js") {
				return pluginOffNote(name, "`disabledPlugins` lists deja", p)
			}
		}
	case "hermes":
		// Hermes loads a plugin only when plugins.enabled names it, which the
		// install writes; `hermes plugins disable deja` moves it out.
		p := filepath.Join(sources.HermesHome(), "config.yaml")
		b, err := readConfig(p)
		if err != nil {
			return ""
		}
		_, disabled, _ := yamlLookup(string(b), "plugins", "disabled")
		_, enabled, listed := yamlLookup(string(b), "plugins", "enabled")
		if anyDeja(disabled) || listed && !anyDeja(enabled) {
			return pluginOffNote(name, "deja is not in `plugins.enabled`", p)
		}
	case "deepseek":
		if b, err := readConfig(dshPatchPath()); err == nil && dshRowDisabled(string(b), "deja-auto") {
			return pluginOffNote("dsh", "`disabled: true` on the deja-auto row", dshPatchPath())
		}
	}
	return ""
}

func mcpOffNote(client, what, p string) string {
	return "switched off: " + what + " in " + reportPath(p) + " — " + client + " will not start it"
}

func hooksOffNote(client, what, p string) string {
	return "switched off: " + what + " in " + reportPath(p) + " — " + client + " runs no hooks at all"
}

func pluginOffNote(client, what, p string) string {
	return "switched off: " + what + " in " + reportPath(p) + " — " + client + " will not run deja's recall"
}

// doctorProjectDirs is the directory doctor runs in and the repository it is
// in, the two a client keys its per-project switches by.
func doctorProjectDirs() []string {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	dirs := []string{cwd}
	if root := gitRootOf(filepath.Join(cwd, "x")); root != "" && root != cwd {
		dirs = append(dirs, root)
	}
	return dirs
}

// cursorDataDir and cursorProjectSlug are cursor-agent's: CURSOR_DATA_DIR or
// ~/.cursor, and the project path with every run of non-alphanumerics made
// one dash.
func cursorDataDir() string {
	if p := strings.TrimSpace(os.Getenv("CURSOR_DATA_DIR")); p != "" {
		return p
	}
	return filepath.Join(sources.Home(), ".cursor")
}

var cursorSlugRun = regexp.MustCompile(`[^A-Za-z0-9]+`)

func cursorProjectSlug(dir string) string {
	return strings.Trim(cursorSlugRun.ReplaceAllString(dir, "-"), "-")
}

// geminiExtensionEnabled applies gemini's override rules to dir: `!` disables,
// a trailing `*` takes the subdirectories too, the last match wins.
func geminiExtensionEnabled(rules []string, dir string) bool {
	slashed := func(s string) string {
		s = strings.ReplaceAll(s, `\`, "/")
		if !strings.HasPrefix(s, "/") {
			s = "/" + s
		}
		if !strings.HasSuffix(s, "/") {
			s += "/"
		}
		return s
	}
	at := slashed(dir)
	enabled := true
	for _, rule := range rules {
		base, off := strings.CutPrefix(rule, "!")
		base, subdirs := strings.CutSuffix(base, "*")
		if subdirs && strings.HasPrefix(at, base) || !subdirs && at == base {
			enabled = !off
		}
	}
	return enabled
}

// piExtensionExcluded is the entry in pi's `extensions` setting that turns
// deja's extension off, and "" when none does: `-path` excludes exactly and
// wins over everything, `+path` brings back what a `!glob` took.
func piExtensionExcluded(entries []string, dir string) string {
	rel := "extensions/deja.ts"
	abs := filepath.Join(dir, "extensions", "deja.ts")
	exact := func(s string) bool {
		s = strings.TrimPrefix(s, "./")
		return s == rel || filepath.Clean(s) == abs
	}
	glob := func(s string) bool {
		for _, p := range []string{rel, filepath.ToSlash(abs), "deja.ts"} {
			if ok, _ := path.Match(strings.TrimPrefix(s, "./"), p); ok {
				return true
			}
		}
		return false
	}
	off, forced := "", false
	for _, e := range entries {
		switch {
		case strings.HasPrefix(e, "-") && exact(e[1:]):
			return e
		case strings.HasPrefix(e, "+") && exact(e[1:]):
			forced = true
		case strings.HasPrefix(e, "!") && glob(e[1:]):
			off = e
		}
	}
	if forced {
		return ""
	}
	return off
}

// serverPatternMatch is qwen's matchesServerPattern: `*` any run, `?` one
// character, everything else literal.
func serverPatternMatch(pattern, name string) bool {
	if pattern == "" {
		return name == ""
	}
	switch pattern[0] {
	case '*':
		for i := 0; i <= len(name); i++ {
			if serverPatternMatch(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	case '?':
		return name != "" && serverPatternMatch(pattern[1:], name[1:])
	}
	return name != "" && name[0] == pattern[0] && serverPatternMatch(pattern[1:], name[1:])
}
