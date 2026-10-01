package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// codexHome writes a hooks.json holding the given events and the trust entry
// codex needs before it runs any of them.
func codexHome(t *testing.T, events ...string) {
	t.Helper()
	home := sources.CodexHome()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	sub := func(event string) string {
		for _, h := range codexHookWiring {
			if h.Event == event {
				return h.Sub
			}
		}
		return "hook-unknown"
	}
	var entries []string
	for _, e := range events {
		entries = append(entries, `"`+e+`":[{"matcher":"","hooks":[{"type":"command","command":"/usr/local/bin/deja `+
			sub(e)+`"}]}]`)
	}
	if err := os.WriteFile(filepath.Join(home, "hooks.json"),
		[]byte(`{"hooks":{`+strings.Join(entries, ",")+`}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"),
		[]byte("[hooks.json:session_start]\ntrusted_hash = \"sha256:abc\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Codex trusts a hooks.json by its hash, and the one it approved may be the one
// an older deja wrote. Trusted said "wired" whatever was in it, so the events
// added since ran nowhere and nothing said so.
func TestDoctorNamesTheCodexEventsAnOlderInstallLacks(t *testing.T) {
	withStatsStores(t)
	codexHome(t, "SessionStart")

	var out bytes.Buffer
	doctorCodexHook(&out)
	got := out.String()
	if !strings.Contains(got, "out of date") {
		t.Errorf("a one-of-three wiring was not called out:\n%s", got)
	}
	for _, want := range []string{"PreToolUse", "PostToolUse", "deja install"} {
		if !strings.Contains(got, want) {
			t.Errorf("the report does not name %q:\n%s", want, got)
		}
	}
}

func TestDoctorIsQuietWhenCodexHasEveryEvent(t *testing.T) {
	withStatsStores(t)
	var all []string
	for _, h := range codexHookWiring {
		all = append(all, h.Event)
	}
	codexHome(t, all...)

	var out bytes.Buffer
	doctorCodexHook(&out)
	if got := out.String(); strings.Contains(got, "out of date") {
		t.Errorf("a complete wiring was reported as stale:\n%s", got)
	}
}

// Codex users keep their own hooks in hooks.json. One that holds no entry of
// deja's is a machine deja never wired, whatever codex's trust store says
// about the user's hooks: the row reads missing (#4297).
func TestDoctorCodexHookWithOnlyTheUsersHooksIsMissing(t *testing.T) {
	for _, trust := range []string{"no config.toml", "untrusted", "trusted"} {
		t.Run(trust, func(t *testing.T) {
			hermeticEnv(t)
			home := sources.CodexHome()
			if err := os.MkdirAll(home, 0o755); err != nil {
				t.Fatal(err)
			}
			hooks := filepath.Join(home, "hooks.json")
			own := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"/usr/local/bin/my-notes --start"}]}]}}`
			if err := os.WriteFile(hooks, []byte(own), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg := ""
			switch trust {
			case "untrusted":
				cfg = "model = \"gpt-5\"\n"
			case "trusted":
				cfg = "[hooks.state." + strconv.Quote(hooks+":session_start:0:0") + "]\ntrusted_hash = \"sha256:abc\"\n"
			}
			if cfg != "" {
				if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(cfg), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if st := codexHookWiringState(); st.state != "missing" {
				t.Errorf("state = %q, want missing", st.state)
			}
			var buf bytes.Buffer
			doctorCodexHook(&buf)
			if out := buf.String(); !strings.HasPrefix(strings.TrimSpace(out), "codex-hook   missing") || strings.Contains(out, "approve") {
				t.Errorf("row = %q, want missing and no advice to approve someone else's hook", out)
			}

			// Control: deja's own hook beside the user's is read as before.
			if _, err := installCodexHooks("/usr/local/bin/deja", false); err != nil {
				t.Fatalf("install codex-auto: %v", err)
			}
			if st := codexHookWiringState(); st.state == "missing" {
				t.Errorf("deja's hooks beside the user's: state %q", st.state)
			}
		})
	}
}

// A hooks.json that does not parse says nothing about deja's entries, so the
// trust store is not asked about it either: the row reads unreadable, as
// Claude's does (#4297).
func TestDoctorCodexHookUnparseableIsUnreadable(t *testing.T) {
	hermeticEnv(t)
	home := sources.CodexHome()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	hooks := filepath.Join(home, "hooks.json")
	if err := os.WriteFile(hooks, []byte(`{"hooks":{"SessionStart":[{"hooks":[`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := "[hooks.state." + strconv.Quote(hooks+":session_start:0:0") + "]\ntrusted_hash = \"sha256:abc\"\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if st := codexHookWiringState(); st.state != "unreadable" {
		t.Errorf("state = %q, want unreadable", st.state)
	}
	var buf bytes.Buffer
	doctorCodexHook(&buf)
	if out := buf.String(); !strings.Contains(out, "unreadable") || strings.Contains(out, "events wired") {
		t.Errorf("row = %q, want unreadable and no event count", out)
	}
}

// With the Codex plugin enabled, deja's hooks ride the plugin, and a
// hooks.json holding only the user's hook leaves the row at plugin (#4297).
func TestDoctorCodexHookWithThePluginBesideTheUsersHooks(t *testing.T) {
	hermeticEnv(t)
	home := sources.CodexHome()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	own := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"/usr/local/bin/my-notes --start"}]}]}}`
	if err := os.WriteFile(filepath.Join(home, "hooks.json"), []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("[plugins.\"deja-vu@deja-vu\"]\nenabled = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st := codexHookWiringState(); st.state != "plugin" {
		t.Errorf("state = %q, want plugin", st.state)
	}
}
