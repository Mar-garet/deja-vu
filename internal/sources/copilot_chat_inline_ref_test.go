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
