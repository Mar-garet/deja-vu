package sources

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/vshulcz/deja-vu/internal/model"
)

// Before native tool calling, Roo (v3.20, for one) and the Cline extension
// kept a tool call as XML inside the assistant's text block —
// `<execute_command>\n<command>go test</command>\n</execute_command>` — and its
// result as user text blocks headed `[execute_command for 'go test'] Result:`.
// The readers took tool_use blocks only, so a task from that era had no
// command, files or edit record, and what a command printed was indexed as the
// person's words (#4424).

// rooXMLTools are the calls of that era that leave a record. Anything else in
// angle brackets is prose, and a name that is not here is not parsed.
var rooXMLTools = []string{"execute_command", "read_file", "write_to_file", "apply_diff",
	"insert_content", "search_and_replace", "replace_in_file"}

var (
	rooXMLParam = regexp.MustCompile(`<([a-z_]+)>`)
	rooXMLPath  = regexp.MustCompile(`<path>([^<]*)</path>`)
)

// rooXMLCalls reads the XML calls in an assistant's text into blocks of the
// tool_use shape, so the same records come out of them that come out of a
// native call. A call whose closing tag never arrives was cut off mid-stream
// and is not one the client ran.
func rooXMLCalls(text string) []any {
	var out []any
	for pos := 0; pos < len(text); {
		name, at := "", -1
		for _, n := range rooXMLTools {
			if i := strings.Index(text[pos:], "<"+n+">"); i >= 0 && (at < 0 || i < at) {
				name, at = n, i
			}
		}
		if at < 0 {
			break
		}
		start := pos + at + len(name) + 2
		end := strings.Index(text[start:], "</"+name+">")
		if end < 0 {
			break
		}
		body := text[start : start+end]
		pos = start + end + len(name) + 3
		in := rooXMLParams(body)
		// read_file of that era takes several files at once under <args>,
		// each a <file><path>; one call per path keeps the shared readers.
		if args, ok := in["args"].(string); ok && name == "read_file" {
			for _, m := range rooXMLPath.FindAllStringSubmatch(args, -1) {
				if p := strings.TrimSpace(m[1]); p != "" {
					out = append(out, map[string]any{"type": "tool_use", "name": name, "input": map[string]any{"path": p}})
				}
			}
			continue
		}
		if len(in) > 0 {
			out = append(out, map[string]any{"type": "tool_use", "name": name, "input": in})
		}
	}
	return out
}

// rooXMLParams reads the parameters of one call. The values are not escaped —
// a file's content goes in as written — so content and diff run to the last
// closing tag of their name, the way Roo's own parser reads them, and the rest
// to the first.
func rooXMLParams(body string) map[string]any {
	in := map[string]any{}
	for pos := 0; pos < len(body); {
		loc := rooXMLParam.FindStringSubmatchIndex(body[pos:])
		if loc == nil {
			break
		}
		key := body[pos+loc[2] : pos+loc[3]]
		start := pos + loc[1]
		closeTag := "</" + key + ">"
		end := strings.Index(body[start:], closeTag)
		if key == "content" || key == "diff" {
			end = strings.LastIndex(body[start:], closeTag)
		}
		if end < 0 {
			break
		}
		v := body[start : start+end]
		if key == "content" || key == "diff" {
			v = strings.Trim(v, "\r\n")
		} else {
			v = strings.TrimSpace(v)
		}
		if _, seen := in[key]; !seen {
			in[key] = v
		}
		pos = start + end + len(closeTag)
	}
	return in
}

// rooWithXMLCalls adds the XML calls of an assistant turn to its blocks, for a
// turn that has no native call of its own: a client that wrote tool_use
// blocks did not also run what its text quotes.
func rooWithXMLCalls(blocks []any) []any {
	var calls []any
	for _, it := range blocks {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		switch t, _ := m["type"].(string); t {
		case "tool_use":
			return blocks
		case "text":
			s, _ := m["text"].(string)
			calls = append(calls, rooXMLCalls(s)...)
		}
	}
	return append(blocks, calls...)
}

// rooResultHeader is the line Roo and Cline put ahead of a tool's result:
// `[execute_command for 'go test'] Result:`, or `[attempt_completion] Result:`.
var rooResultHeader = regexp.MustCompile(`(?s)^\[([a-z_]+)(?: for '.*?')?\] Result:\s*`)

// rooAnswerTools return what the person said, not what a tool printed: the
// answer to a question, the feedback on a completion. Their results stay the
// person's words, without the header.
var rooAnswerTools = map[string]bool{"ask_followup_question": true, "attempt_completion": true}

// rooUserTurn splits a user turn's text blocks into what tools printed and
// what the person wrote. A result runs from its header to the next header or
// to the host's environment_details block. A turn in any other shape is all
// the person's.
func rooUserTurn(raw json.RawMessage) (results []string, words string) {
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return nil, clineContentText(raw)
	}
	var parts []string
	result := -1
	for _, blk := range blocks {
		if blk.Type != "text" {
			continue
		}
		text := strings.TrimSpace(blk.Text)
		if text == "" {
			continue
		}
		if m := rooResultHeader.FindStringSubmatch(text); m != nil {
			rest := strings.TrimSpace(text[len(m[0]):])
			if rooAnswerTools[m[1]] {
				result = -1
				if rest != "" {
					parts = append(parts, rest)
				}
				continue
			}
			results = append(results, rest)
			result = len(results) - 1
			continue
		}
		if strings.HasPrefix(text, "<environment_details>") {
			result = -1
		}
		if result >= 0 {
			results[result] = strings.TrimSpace(results[result] + "\n" + text)
			continue
		}
		parts = append(parts, text)
	}
	kept := results[:0]
	for _, r := range results {
		if r != "" {
			kept = append(kept, r)
		}
	}
	return kept, strings.Join(parts, "\n")
}

// rooLegacyToolOutput is clineTurnToolOutput for the results of that era.
func rooLegacyToolOutput(results []string, ts time.Time) []model.Message {
	if !IndexToolOutput() {
		return nil
	}
	var out []model.Message
	for _, body := range results {
		out = append(out, model.Message{Role: RoleToolOutput, Text: capParsedMessage(body), Time: ts})
	}
	return out
}
