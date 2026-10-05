package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// The WorkBuddy AI desktop app (5.6.2) starts every server in the user's
// mcp.json as trust-pending and leaves it out of the config it hands its agent
// until the user clicks Trust in Settings > MCP (#4740). The click writes
// <configDir>/mcp-approvals.json, {"<sha256 hex of material>::<name>": <ms>},
// where for a stdio server material is
//
//	command | sorted args | sorted env keys [| cwd]
//
// The app reads that file at start only, so a Trust needs a restart too. deja
// never writes it: it is the app's security prompt, and the hash moves with
// the command path. The plain CodeBuddy CLI has no such gate.

const workBuddyTrustNote = "WorkBuddy keeps new MCP servers off until trusted — click Trust for deja in WorkBuddy Settings > MCP, then restart the app"

// workBuddyApprovalKey is the key the app files a stdio server's approval
// under.
func workBuddyApprovalKey(name, command string, args, envKeys []string, cwd string) string {
	args = append([]string(nil), args...)
	sort.Strings(args)
	envKeys = append([]string(nil), envKeys...)
	sort.Strings(envKeys)
	parts := []string{command, strings.Join(args, ","), strings.Join(envKeys, ",")}
	if cwd != "" {
		parts = append(parts, cwd)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:]) + "::" + name
}

// workBuddyEntryApprovalKey is the key for one mcpServers entry as the file
// holds it.
func workBuddyEntryApprovalKey(name string, entry map[string]any) string {
	command, _ := entry["command"].(string)
	var args []string
	if list, ok := entry["args"].([]any); ok {
		for _, a := range list {
			args = append(args, jsStringOf(a))
		}
	}
	var envKeys []string
	if env, ok := entry["env"].(map[string]any); ok {
		for k := range env {
			envKeys = append(envKeys, k)
		}
	}
	cwd, _ := entry["cwd"].(string)
	return workBuddyApprovalKey(name, command, args, envKeys, cwd)
}

// jsStringOf is String(v) for the values JSON can hold.
func jsStringOf(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return "null"
	default:
		return fmt.Sprint(x)
	}
}

// workBuddyHasTrustGate reports whether the WorkBuddy home deja wires is the
// WorkBuddy AI app's, the edition the gate was read from.
func workBuddyHasTrustGate(dir string) bool {
	if strings.TrimSpace(os.Getenv("WORKBUDDY_CONFIG_DIR")) != "" {
		return true
	}
	return filepath.Base(dir) == ".workbuddy-ai"
}

// workBuddyUntrustedNote is the fix line when the WorkBuddy AI app has not
// approved the deja entry as it stands in mcp.json, and "" when it has, when
// there is no deja entry, or when the home has no trust gate.
func workBuddyUntrustedNote() string {
	dir := sources.WorkBuddyConfigDir()
	if !workBuddyHasTrustGate(dir) {
		return ""
	}
	servers, _ := readJSONConfig(workBuddyMCPPath())["mcpServers"].(map[string]any)
	approvals := readJSONConfig(filepath.Join(dir, "mcp-approvals.json"))
	found := false
	for name, v := range servers {
		entry, _ := v.(map[string]any)
		if entry == nil || (name != "deja" && !entryRunsDeja(entry)) {
			continue
		}
		found = true
		if _, ok := approvals[workBuddyEntryApprovalKey(name, entry)]; ok {
			return ""
		}
	}
	if !found {
		return ""
	}
	return workBuddyTrustNote
}
