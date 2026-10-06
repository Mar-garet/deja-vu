package index

import (
	"strings"

	"github.com/vshulcz/deja-vu/internal/model"
	"github.com/vshulcz/deja-vu/internal/query"
)

// withDirectorySessions puts the sessions about an asked file's directory at
// the head of an answer that found nothing exact, or only loosened matches. A rule is usually written
// for a package ("every change in cmd/reconcile is replayed against ..."),
// while an agent about to edit asks by file ("cmd/reconcile/match.go"). The
// path is one term that only matched sessions naming the file, so the package
// rule never surfaced, or lost to a session sharing one ordinary word (#4762).
//
// Only a directory named exactly counts; the answer is labelled relevance, so
// a caller still reads it as leads rather than as a record of the file.
func withDirectorySessions(dir string, o query.Options, r SearchResult) SearchResult {
	// A strict head holds every word, the path included: that answer names the
	// file and needs nothing put in front of it.
	if strings.Contains(o.Query, "\"") || r.Strict > 0 {
		return r
	}
	for _, d := range parentDirs(RelevanceTerms(o.Query)) {
		o2 := o
		o2.Query = d
		r2, err := searchDetailedOnce(dir, o2)
		if err != nil || len(r2.Sessions) == 0 || r2.Tier == query.TierRelevance {
			continue
		}
		seen := make(map[string]bool, len(r2.Sessions))
		merged := make([]model.Session, 0, len(r2.Sessions)+len(r.Sessions))
		for _, s := range r2.Sessions {
			seen[s.Harness+"\x00"+s.ID] = true
			merged = append(merged, s)
		}
		for _, s := range r.Sessions {
			if !seen[s.Harness+"\x00"+s.ID] {
				merged = append(merged, s)
			}
		}
		r.Total += len(merged) - len(r.Sessions)
		r.Capped = r.Capped || r2.Capped
		r.Sessions = merged
		r.Tier = query.TierRelevance
		r.Directory = d
		return r
	}
	return r
}

// parentDirs lists the directories of the file paths among the terms, whole
// and as their last two segments, so an absolute path still meets a session
// that wrote the repo-relative one. A single segment is left out: "cmd" or
// "internal" names half the corpus.
func parentDirs(terms []string) []string {
	var out []string
	for _, t := range terms {
		i := strings.LastIndexByte(t, '/')
		if i <= 0 || !strings.Contains(t[i+1:], ".") {
			continue
		}
		segs := strings.Split(strings.Trim(t[:i], "/"), "/")
		if len(segs) < 2 {
			continue
		}
		out = append(out, strings.Join(segs, "/"))
		if len(segs) > 2 {
			out = append(out, strings.Join(segs[len(segs)-2:], "/"))
		}
	}
	return out
}
