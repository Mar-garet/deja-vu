package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// reasonixPackageEnabled reports whether Reasonix holds an enabled record of
// deja's package. Doctor reads it: a directory with no record loads nothing.
func reasonixPackageEnabled() bool {
	st, _, err := readReasonixState(reasonixStatePath())
	if err != nil {
		return false
	}
	i, _ := st.ours()
	if i < 0 {
		return false
	}
	var e reasonixEntry
	return json.Unmarshal(st.Plugins[i], &e) == nil && e.Enabled
}

// doctorMCPSwitchedOff is the line under a wired MCP row whose harness has
// turned the server off, and "" when it has not. Reasonix starts the server
// from deja's plugin package, so `reasonix plugin disable deja` stops it along
// with the auto-recall hooks while the manifest still declares it (#4409).
func doctorMCPSwitchedOff(name string) string {
	if name == "reasonix" && !reasonixPackageEnabled() {
		return "switched off: no enabled record in " + reportPath(reasonixStatePath()) + " — reasonix will not start it; `deja install reasonix-auto`"
	}
	return ""
}

// reasonixRuntimeMissing is the runtime command the installed package names
// when that file is gone, and "" otherwise. The command is a field of its own
// in the manifest, so the text scan doctor runs over hook files cannot see it.
func reasonixRuntimeMissing() string {
	b, err := os.ReadFile(reasonixInstalledManifest())
	if err != nil {
		return ""
	}
	var m struct {
		Runtime struct {
			Command string `json:"command"`
		} `json:"runtime"`
	}
	if json.Unmarshal(b, &m) != nil || !filepath.IsAbs(m.Runtime.Command) {
		return ""
	}
	if _, err := os.Stat(m.Runtime.Command); err != nil {
		return m.Runtime.Command
	}
	return ""
}

func reasonixRuntimeMissingFor(harness string) string {
	if harness != "reasonix" {
		return ""
	}
	return reasonixRuntimeMissing()
}
