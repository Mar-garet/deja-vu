package index

import "testing"

// CodeBuddy reports a shell call as `Command: …`, `Stdout: …`, `Stderr: …`,
// `Exit Code: N`, and Qwen Code as `Command: …`, `Output: …`, `Error: …`. The
// first line of what the command printed shares a line with the label, so the
// error was stored as `Stdout: fatal: …`: a fix learned in CodeBuddy answered
// no other harness, and `deja fix` with the error as printed found nothing.
func TestAShellEnvelopeLabelIsNotPartOfTheError(t *testing.T) {
	for _, c := range []struct{ wrapped, bare string }{
		{"Stdout: fatal: unrecognized argument: --onelin", "fatal: unrecognized argument: --onelin"},
		{"Stderr: ModuleNotFoundError: No module named 'app.db'", "ModuleNotFoundError: No module named 'app.db'"},
		{"Output: fatal: your current branch 'master' does not have any commits yet", "fatal: your current branch 'master' does not have any commits yet"},
		{"Stderr: zsh:1: command not found: timeout", "zsh:1: command not found: timeout"},
	} {
		wl, wsig, wok := FrictionSignature(c.wrapped)
		bl, bsig, bok := FrictionSignature(c.bare)
		if !bok {
			t.Fatalf("control: %q is not read as friction", c.bare)
		}
		if !wok || wsig != bsig || wl != bl {
			t.Errorf("%q read as %q (%v), want %q like the bare line", c.wrapped, wl, wok, bl)
		}
	}
	// Only a label at the very start, and only these three: an error that
	// names its own output keeps its words.
	for _, keep := range []string{
		"Error: cannot find module 'express'",
		"panic: Output: buffer closed while writing",
		"Stdout:fatal: no space after the colon is not the label",
	} {
		if got := normalizeFriction(keep); got != keep {
			t.Errorf("normalizeFriction(%q) = %q, want it unchanged", keep, got)
		}
	}
}
