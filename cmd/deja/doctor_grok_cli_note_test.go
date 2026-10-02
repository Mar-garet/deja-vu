package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The line about @vibe-kit/grok-cli suggests `grok mcp add deja -c deja -a
// mcp`, which Grok Build rejects ("unexpected argument '-c' found"). Grok Build
// reads the server from ~/.grok/config.toml already, so the line is only for a
// machine whose `grok` is the vibe-kit CLI, or that has none on PATH (#4587).
func TestDoctorGrokCLINoteOnlyForTheCLIItIsAbout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake grok is a symlink")
	}
	for _, tc := range []struct {
		name, pkg string
		want      bool
	}{
		{"grok build", filepath.Join("@xai-official", "grok", "bin", "grok-native"), false},
		{"vibe-kit", filepath.Join("@vibe-kit", "grok-cli", "dist", "index.js"), true},
		{"none", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			bin := filepath.Join(tmp, "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.pkg != "" {
				target := filepath.Join(tmp, "lib", "node_modules", tc.pkg)
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(bin, "grok")); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin)
			note := doctorWiringNote("grok")
			if got := strings.Contains(note, "grok mcp add"); got != tc.want {
				t.Errorf("note shown = %v, want %v: %q", got, tc.want, note)
			}
		})
	}
}
