package sources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func copilotChatAssistantText(t *testing.T, response string) (string, []string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.json")
	body := `{"version":3,"sessionId":"s","creationDate":1763727100000,"requests":[{` +
		`"timestamp":1763727104742,"message":{"text":"why does the price come back empty"},` +
		`"response":` + response + `,"responseTimestamp":1763727400000}]}`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ss, err := ParseCopilotChatFile(p)
	if err != nil || len(ss) != 1 {
		t.Fatalf("%v %#v", err, ss)
	}
	var assistant, files []string
	for _, m := range ss[0].Messages {
		switch m.Role {
		case "assistant":
			assistant = append(assistant, m.Text)
		case RoleFiles:
			files = append(files, m.Text)
		}
	}
	return strings.Join(assistant, "\n"), files
}

// VS Code draws an inlineReference part as the file's or symbol's name in the
// middle of the reply. Dropped from the text, the sentence lost its subject —
// "The bug is in , line 40:  swallows the error." — and the symbol could not
// be searched for (#4589).
func TestCopilotChatInlineReferenceKeepsItsName(t *testing.T) {
	got, files := copilotChatAssistantText(t, `[
		{"value":"The bug is in "},
		{"kind":"inlineReference","inlineReference":{"$mid":1,"fsPath":"/tmp/proj/retry.go","path":"/tmp/proj/retry.go","scheme":"file"}},
		{"value":", line 40: "},
		{"kind":"inlineReference","inlineReference":{"name":"fetchPrice","kind":12,"location":{"uri":{"$mid":1,"path":"/tmp/proj/price.go","scheme":"file"},"range":{"startLineNumber":175,"startColumn":1,"endLineNumber":175,"endColumn":10}}}},
		{"value":" swallows the error."},
		{"kind":"inlineReference","inlineReference":{"$mid":1,"path":"/tmp/proj/README.md","scheme":"file"},"name":"the readme"}
	]`)
	if want := "The bug is in retry.go, line 40: fetchPrice swallows the error.the readme"; got != want {
		t.Errorf("assistant text = %q, want %q", got, want)
	}
	// The file records stay as they were.
	if strings.Join(files, ",") != "/tmp/proj/retry.go,/tmp/proj/price.go,/tmp/proj/README.md" {
		t.Errorf("files = %v", files)
	}
}

// Agent mode writes an edit as an empty code fence around a codeblockUri and
// the edit group, and VS Code draws a pill with the file's name there. Read as
// text, the reply had an empty fence in the middle of it (#4590). A fence with
// code in it stays as it is.
func TestCopilotChatAgentEditNamesTheFileInsteadOfAnEmptyFence(t *testing.T) {
	uri := `{"$mid":1,"fsPath":"/tmp/proj/retry.go","path":"/tmp/proj/retry.go","scheme":"file"}`
	got, files := copilotChatAssistantText(t, `[
		{"value":"Changing the cap:"},
		{"value":"\n`+"```"+`\n"},
		{"kind":"codeblockUri","uri":`+uri+`,"isEdit":true},
		{"kind":"textEditGroup","uri":`+uri+`,"edits":[[{"text":"for i := 0; i < 5; i++ {","range":{"startLineNumber":40,"startColumn":1,"endLineNumber":40,"endColumn":30}}],[]],"done":true},
		{"value":"\n`+"```"+`\n"},
		{"value":"The loop now stops after five attempts."}
	]`)
	if want := "Changing the cap:\nretry.go\nThe loop now stops after five attempts."; got != want {
		t.Errorf("assistant text = %q, want %q", got, want)
	}
	if len(files) == 0 || files[0] != "/tmp/proj/retry.go" {
		t.Errorf("files = %v", files)
	}

	got, _ = copilotChatAssistantText(t, `[
		{"value":"Like this:\n`+"```"+`go\n"},
		{"kind":"codeblockUri","uri":`+uri+`},
		{"value":"for i := 0; i < 5; i++ {\n`+"```"+`\n"}
	]`)
	if want := "Like this:\n```go\nfor i := 0; i < 5; i++ {\n```"; got != want {
		t.Errorf("a code block with code in it changed: %q, want %q", got, want)
	}
}

// A reference to the workspace root or a URI string with an escaped space
// reads as a name, not "/" or "%20".
func TestCopilotChatInlineReferenceOddPaths(t *testing.T) {
	got, _ := copilotChatAssistantText(t, `[
		{"value":"see "},
		{"kind":"inlineReference","inlineReference":{"$mid":1,"path":"/","scheme":"file"}},
		{"value":"and "},
		{"kind":"inlineReference","inlineReference":"file:///tmp/proj/my%20notes.md"}
	]`)
	if want := "see and my notes.md"; got != want {
		t.Errorf("assistant text = %q, want %q", got, want)
	}
}
