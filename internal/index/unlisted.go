package index

import (
	"path/filepath"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// listedView answers, for a file this pass did not list although it is still on
// disk, whether the store it belongs to was read: a setting left it out, or the
// store is outside what this run can see (#4739).
type listedView struct {
	// byDir is the files this pass listed, by directory.
	byDir map[string][]string
	// harness is the store each indexed file's sessions came from.
	harness map[string]string
}

func newListedView(files map[string]FileState, sessions map[string]SessionMeta) *listedView {
	v := &listedView{byDir: map[string][]string{}, harness: map[string]string{}}
	for p := range files {
		d := filepath.Dir(p)
		v.byDir[d] = append(v.byDir[d], p)
	}
	for _, meta := range sessions {
		if meta.Path != "" && meta.Harness != "" {
			v.harness[meta.Path] = meta.Harness
		}
	}
	return v
}

// leftOutBySetting reports a file whose store this pass read: a listed file of
// the same store sits one or two directories above it. That is where a child
// transcript keeps its parent — Claude's <session>/subagents/, Muse's
// <session>/subagent/<child>/, Kimi's agents/<id>/ — and a store root that
// fell out of view has nothing listed there. The store is asked of the
// manifest rather than of the path, since some readers match a file by its
// name alone and would claim a path from another store.
func (v *listedView) leftOutBySetting(p string) bool {
	h := v.harness[p]
	if h == "" {
		return false
	}
	d := filepath.Dir(p)
	for range 2 {
		up := filepath.Dir(d)
		if up == d {
			return false
		}
		d = up
		for _, q := range v.byDir[d] {
			if storeOf(q) == h {
				return true
			}
		}
	}
	return false
}

// storeOf is the store a listed path belongs to, by the reader that claims it.
func storeOf(p string) string {
	k := harnessForPath(p)
	if s := sources.HarnessForKind(k); s != "" {
		return s
	}
	return k
}
