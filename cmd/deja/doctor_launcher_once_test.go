package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// With the launcher gone, a hook row said so twice: once as the binary the hook
// runs and once as the launcher, the same file both times (#4245).
func TestDoctorNamesAMissingLauncherOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no launcher on windows")
	}
	for _, target := range []string{"hermes-auto", "claude-auto"} {
		t.Run(target, func(t *testing.T) {
			hermeticEnv(t)
			bin := filepath.Join(os.Getenv("HOME"), "bin", "deja")
			if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			if _, err := installTarget(target, bin, false); err != nil {
				t.Fatal(err)
			}
			launcher := dejaLauncherPath()
			if err := os.Remove(launcher); err != nil {
				t.Fatal(err)
			}
			out, err := captureRun(t, "doctor")
			if err != nil {
				t.Fatal(err)
			}
			notes := 0
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, filepath.Base(launcher)+", which is not there") {
					notes++
				}
			}
			if notes != 1 {
				t.Errorf("the missing launcher is named %d times, want once:\n%s", notes, out)
			}
		})
	}
}
