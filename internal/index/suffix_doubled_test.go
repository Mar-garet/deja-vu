package index

import (
	"slices"
	"testing"
)

// English doubles a final consonant before -ing, -ed and -er, and the ladder
// only stripped the suffix: "logging" reached "logg" and stopped, so recall
// asked about "structured logging" missed the session that says "log" and
// "logs" (#4785).
func TestSuffixFormsUndoTheDoubledConsonant(t *testing.T) {
	for _, tc := range []struct {
		word string
		want []string
	}{
		{"logging", []string{"log"}},
		{"logged", []string{"log"}},
		{"logger", []string{"log"}},
		{"pinned", []string{"pin"}},
		{"committing", []string{"commit"}},
		{"commit", []string{"committing", "committed"}},
	} {
		got := suffixForms(tc.word)
		for _, want := range tc.want {
			if !slices.Contains(got, want) {
				t.Errorf("%q did not reach %q: %v", tc.word, want, got)
			}
		}
	}
	// A short word goes through the plural-only branch; the doubled forms
	// are the inflections it has.
	catalog := map[string]bool{"logging": true, "logged": true, "logs": true}
	if got := stemMatches("log", catalog); !slices.Contains(got, "logging") || !slices.Contains(got, "logged") {
		t.Errorf("log did not reach logging and logged: %v", got)
	}
}

// Only a doubled consonant is undone. "user" is not "us", "falling" keeps
// "fall", and a vowel ending doubles nothing.
func TestSuffixFormsLeaveUndoubledWordsAlone(t *testing.T) {
	if slices.Contains(suffixForms("user"), "us") {
		t.Errorf("user reached us: %v", suffixForms("user"))
	}
	if !slices.Contains(suffixForms("falling"), "fall") {
		t.Errorf("falling lost fall: %v", suffixForms("falling"))
	}
	for _, w := range []string{"see", "go", "show", "fix", "play"} {
		if f := doubledForms(w); f != nil {
			t.Errorf("doubledForms(%q) = %v, want none", w, f)
		}
	}
}
