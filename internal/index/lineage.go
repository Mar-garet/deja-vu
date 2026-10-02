package index

// Lineage is ids and every session that counts as one of them when recall
// decides what not to answer with.
//
// A session id names one transcript, and an agent can be writing more than one:
// a sub-agent it spawned is its own work seen from inside, and the hooks that
// say which session is live fire under the parent's id, so the child's id is
// never stamped (#4547).
//
// Read off the manifest, which recall has already loaded: no record is read,
// so the cost is one pass over the sessions in memory.
func Lineage(dir string, ids map[string]bool) map[string]bool {
	out := make(map[string]bool, len(ids))
	for id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	m, err := readManifestCached(dir)
	if err != nil {
		return out
	}
	for _, meta := range m.Sessions {
		if spawnedKind(meta.Kind) && meta.Parent != "" && ids[meta.Parent] {
			out[meta.ID] = true
		}
	}
	return out
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
