package index

import (
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/vshulcz/deja-vu/internal/model"
	"github.com/vshulcz/deja-vu/internal/redact"
)

// Lineage is ids and every session that counts as one of them when recall
// decides what not to answer with.
//
// A session id names one transcript, and an agent can be writing more than one:
// a sub-agent it spawned is its own work seen from inside, and the hooks that
// say which session is live fire under the parent's id, so the child's id is
// never stamped (#4547).
//
// The other way too: a spawned agent asking — an opencode task session runs
// its own hooks — is its parent's work, and the parent is live and the one
// that asked it (#4548).
//
// And a fork's source: the fork opens with a copy of its turns, so the source
// handed back is the fork's own opening under another id (#4549, #4251). The
// harness that says which session it forked from is believed. Claude Code and
// opencode say nothing, and there sessions that open on the same turn at the
// same millisecond are copies of one conversation, none of them history to
// the others. A session that merely shares the project is still there.
//
// heads are sessions read off their own store for an asker the index does not
// hold yet — a fork's first prompts come before any build has seen it.
//
// Read off the manifest, which recall has already loaded: no record is read,
// so the cost is one pass over the sessions in memory.
func Lineage(dir string, ids map[string]bool, heads ...model.Session) map[string]bool {
	out := make(map[string]bool, len(ids))
	for id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	openings := map[uint64]bool{}
	asker := func(kind, parent string, opening uint64) {
		if parent != "" && (spawnedKind(kind) || kind == "fork") {
			out[parent] = true
		}
		if opening != 0 {
			openings[opening] = true
		}
	}
	for _, h := range heads {
		if ids[h.ID] {
			asker(h.Kind, h.Parent, SessionOpening(h))
		}
	}
	m, err := readManifestCached(dir)
	if err != nil {
		return out
	}
	for _, meta := range m.Sessions {
		if ids[meta.ID] {
			asker(meta.Kind, meta.Parent, meta.Opening)
		}
	}
	for _, meta := range m.Sessions {
		if spawnedKind(meta.Kind) && meta.Parent != "" && ids[meta.Parent] {
			out[meta.ID] = true
		}
		if meta.Opening != 0 && openings[meta.Opening] {
			out[meta.ID] = true
		}
	}
	return out
}

// HasSession reports whether the index holds a session with this id.
func HasSession(dir, id string) bool {
	if id == "" {
		return false
	}
	m, err := readManifestCached(dir)
	if err != nil {
		return false
	}
	for _, meta := range m.Sessions {
		if meta.ID == id {
			return true
		}
	}
	return false
}

// spawnedKind reports whether a session's kind is the harness's word for an
// agent another session spawned.
func spawnedKind(kind string) bool {
	switch kind {
	case "sidechain", "subagent":
		return true
	}
	return false
}

// openingTextBytes is how much of the opening turn the fingerprint reads: the
// index caps a message's text and a transcript read straight off disk does not.
const openingTextBytes = 256

// SessionOpening fingerprints the turn a session opens with: its harness, the
// first user turn's time to the millisecond, and the start of its text after
// redaction, which the index applies before it stores anything. Zero when the
// session has no timed user turn, so a store without times never matches.
func SessionOpening(s model.Session) uint64 {
	for _, msg := range s.Messages {
		if msg.Role != "user" || msg.Time.IsZero() {
			continue
		}
		text, _ := redact.Text(strings.TrimSpace(msg.Text))
		if text == "" {
			continue
		}
		if len(text) > openingTextBytes {
			text = text[:openingTextBytes]
		}
		h := fnv.New64a()
		_, _ = h.Write([]byte(s.Harness))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(strconv.FormatInt(msg.Time.UnixMilli(), 10)))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(text))
		if sum := h.Sum64(); sum != 0 {
			return sum
		}
		return 1
	}
	return 0
}
