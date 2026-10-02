package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// snapshotIfOnlyEmptyBlocksDiffer gives back deja's snapshot when the config
// an uninstall is about to write differs from it only by empty containers.
// The writers decide on their own whether an emptied block goes with deja's
// entry, and most decided wrong one way or the other: the hook writers, zed,
// VS Code, grok, prime and amp deleted a `"hooks": {}` or `"servers": {}` the
// reader had written, and opencode and kilo kept the `"mcp": {}` they had added
// to an empty `{}` (#4562). The snapshot is what the file was before deja, so
// it settles which.
//
// Only deja's own snapshot, only when nothing but empty blocks differs — a
// server added since, a changed value, and the writer's result stands — and
// only when the comments in the file are the snapshot's, so restoring it does
// not take back a comment written since.
func snapshotIfOnlyEmptyBlocksDiffer(path string, old, next []byte) []byte {
	b, ok := ownSnapshot(path)
	if !ok {
		return next
	}
	want, ok := decodeJSONCExact(next)
	if !ok {
		return next
	}
	have, ok := decodeJSONCExact(b)
	if !ok || reflect.DeepEqual(want, have) {
		return next
	}
	if jsonCommentText(string(b)) != jsonCommentText(lfText(old)) {
		return next
	}
	if !reflect.DeepEqual(withoutEmptyContainers(want), withoutEmptyContainers(have)) {
		return next
	}
	return b
}

// ownSnapshot reads the .bak deja took of path, without its byte order mark.
func ownSnapshot(path string) ([]byte, bool) {
	bak := path + ".bak"
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		bak = resolved + ".bak"
	}
	if !snapshotTaken(path) {
		return nil, false
	}
	b, err := os.ReadFile(bak)
	if err != nil {
		return nil, false
	}
	return bytes.TrimPrefix(b, utf8BOM), true
}

// decodeJSONCExact is decodeJSONExact for a config that may carry comments and
// trailing commas.
func decodeJSONCExact(b []byte) (any, bool) {
	return decodeJSONExact([]byte(jsoncToJSON(lfText(b))))
}

// jsonCommentText is the comments of a JSONC text, in order.
func jsonCommentText(s string) string {
	stripped := stripJSONComments(s)
	var out strings.Builder
	for i := 0; i < len(s) && i < len(stripped); i++ {
		if s[i] != stripped[i] {
			out.WriteByte(s[i])
		}
	}
	return out.String()
}

// withoutEmptyContainers drops every object key whose value is, or prunes to,
// an empty object or array.
func withoutEmptyContainers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, child := range t {
			child = withoutEmptyContainers(child)
			if isEmptyContainer(child) {
				continue
			}
			out[k] = child
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = withoutEmptyContainers(child)
		}
		return out
	}
	return v
}

func isEmptyContainer(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		return len(t) == 0
	case []any:
		return len(t) == 0
	}
	return false
}
