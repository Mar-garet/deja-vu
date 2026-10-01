package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
)

// marshalConfigLike renders a config in the shape the reader wrote it in.
//
// The goose writer explains why it edits text rather than round-tripping: a
// config holds settings someone wrote by hand, and re-serialising drops the
// ordering they left there. The JSON writers do round-trip — they unmarshal
// into map[string]any and marshal it back — so an install that touched one
// block rewrote the whole document: a four-space file came back at two, and
// the keys came back alphabetised, because Go sorts map keys (#2640).
//
// The indent and the key order are cheap to give back and this is the one
// place the writers pass through. The order is the reader's at every depth:
// giving it back only at the top left `kiro-cli mcp add`'s command-first
// servers with args first after an install and an uninstall (#4306). What it
// does not give back is an inline object or array staying inline: that is
// keepInlineBlocks.
func marshalConfigLike(old []byte, root map[string]any) ([]byte, error) {
	compact, err := marshalInOrder(reflect.ValueOf(root), readKeyOrder(old))
	if err != nil {
		return nil, err
	}
	// MarshalIndent is Marshal and then Indent, so this is its output with
	// the reader's order.
	var next bytes.Buffer
	if err := json.Indent(&next, compact, "", jsonIndentOf(old)); err != nil {
		return nil, err
	}
	return keepInlineBlocks(old, next.Bytes()), nil
}

// keyOrder is the order a document's objects list their keys in, by path. The
// entries of one array share a shape, so they share one order, merged first
// seen first: an entry deja added or removed shifts positions, and a position
// is no evidence of which entry is which.
type keyOrder struct {
	keys []string
	sub  map[string]*keyOrder
	elem *keyOrder
}

func (o *keyOrder) child(k string) *keyOrder {
	if o == nil {
		return nil
	}
	return o.sub[k]
}

// readKeyOrder reads the order out of the reader's file; nil, and Go's sorted
// order, for one that does not parse even as JSONC.
func readKeyOrder(old []byte) *keyOrder {
	for _, b := range [][]byte{old, []byte(jsoncToJSON(string(old)))} {
		if o, err := decodeKeyOrder(json.NewDecoder(bytes.NewReader(b))); err == nil {
			return o
		}
	}
	return nil
}

func decodeKeyOrder(dec *json.Decoder) (*keyOrder, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return nil, nil
	}
	o := &keyOrder{sub: map[string]*keyOrder{}}
	for dec.More() {
		if d == '[' {
			e, err := decodeKeyOrder(dec)
			if err != nil {
				return nil, err
			}
			o.elem = mergeKeyOrder(o.elem, e)
			continue
		}
		k, err := dec.Token()
		if err != nil {
			return nil, err
		}
		name, _ := k.(string)
		c, err := decodeKeyOrder(dec)
		if err != nil {
			return nil, err
		}
		if _, dup := o.sub[name]; !dup {
			o.keys = append(o.keys, name)
		}
		o.sub[name] = mergeKeyOrder(o.sub[name], c)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return o, nil
}

func mergeKeyOrder(a, b *keyOrder) *keyOrder {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	for _, k := range b.keys {
		if _, ok := a.sub[k]; !ok {
			a.keys = append(a.keys, k)
		}
		a.sub[k] = mergeKeyOrder(a.sub[k], b.sub[k])
	}
	a.elem = mergeKeyOrder(a.elem, b.elem)
	return a
}

// marshalInOrder is json.Marshal with each object's keys in the reader's
// order, and any key they did not have after theirs, sorted the way Marshal
// sorts. Everything that is not a map or a list is Marshal's own.
func marshalInOrder(v reflect.Value, o *keyOrder) ([]byte, error) {
	for v.Kind() == reflect.Interface && !v.IsNil() {
		v = v.Elem()
	}
	if v.IsValid() && v.Type().Implements(reflect.TypeOf((*json.Marshaler)(nil)).Elem()) {
		return json.Marshal(v.Interface())
	}
	switch {
	case v.Kind() == reflect.Map && v.Type().Key().Kind() == reflect.String && !v.IsNil():
		keys := make([]string, 0, v.Len())
		for _, k := range v.MapKeys() {
			keys = append(keys, k.String())
		}
		sort.Strings(keys)
		if o != nil {
			rank := make(map[string]int, len(o.keys))
			for i, k := range o.keys {
				rank[k] = i
			}
			sort.SliceStable(keys, func(i, j int) bool {
				ri, iok := rank[keys[i]]
				rj, jok := rank[keys[j]]
				if iok && jok {
					return ri < rj
				}
				return iok && !jok
			})
		}
		var b bytes.Buffer
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			b.Write(kb)
			b.WriteByte(':')
			vb, err := marshalInOrder(v.MapIndex(reflect.ValueOf(k).Convert(v.Type().Key())), o.child(k))
			if err != nil {
				return nil, err
			}
			b.Write(vb)
		}
		b.WriteByte('}')
		return b.Bytes(), nil
	case (v.Kind() == reflect.Slice && !v.IsNil() || v.Kind() == reflect.Array) && v.Type().Elem().Kind() != reflect.Uint8:
		var elem *keyOrder
		if o != nil {
			elem = o.elem
		}
		var b bytes.Buffer
		b.WriteByte('[')
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			eb, err := marshalInOrder(v.Index(i), elem)
			if err != nil {
				return nil, err
			}
			b.Write(eb)
		}
		b.WriteByte(']')
		return b.Bytes(), nil
	}
	if !v.IsValid() {
		return []byte("null"), nil
	}
	return json.Marshal(v.Interface())
}

// jsonIndentOf is the indent unit the file already used. Two spaces for a file
// deja is creating, which has no shape to keep, and for one written on a single
// line, where there is nothing to read an indent from.
func jsonIndentOf(old []byte) string {
	for _, line := range strings.Split(string(old), "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || trimmed == line {
			continue
		}
		return line[:len(line)-len(trimmed)]
	}
	return "  "
}

// topLevelKeyOrder reads the keys of a JSON object in the order they appear.
func topLevelKeyOrder(b []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(b))
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	var keys []string
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return nil
		}
		name, ok := k.(string)
		if !ok {
			return nil
		}
		keys = append(keys, name)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil
		}
	}
	return keys
}
