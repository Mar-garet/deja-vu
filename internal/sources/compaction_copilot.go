package sources

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vshulcz/deja-vu/internal/model"
)

// IsCopilotTranscript reports whether path is a log one of the two Copilot
// hosts names as a PreCompact transcript_path: Copilot CLI's
// session-state/<id>/events.jsonl, or VS Code Copilot Chat's
// GitHub.copilot-chat/transcripts/<id>.jsonl.
func IsCopilotTranscript(path string) bool {
	return copilotAgentTranscript(path) || copilotCLIEventLog(path)
}

// copilotCLIEventLog is session-state/<id>/events.jsonl. Grok and Reasonix
// write an events.jsonl too, so the directory has to say Copilot.
func copilotCLIEventLog(path string) bool {
	return filepath.Base(path) == "events.jsonl" && filepath.Base(filepath.Dir(filepath.Dir(path))) == "session-state"
}

// ReadCompactionCopilot captures a Copilot session from the log its PreCompact
// hook names, with the reader deja indexes it by. Both logs are append-only
// event streams, so the turns about to be summarised are still in them.
//
// The general by-registry reader (Files, Kind.Parse, select by id) will cover
// this once it lands; until then the two Copilot logs are read here.
func ReadCompactionCopilot(path, nativeSessionID string) (CompactionTranscript, error) {
	if strings.TrimSpace(nativeSessionID) == "" {
		return CompactionTranscript{}, fmt.Errorf("%w: missing session id", ErrTranscriptIdentity)
	}
	info, err := regularCompactionFile(path)
	if err != nil {
		return CompactionTranscript{}, err
	}
	var (
		ss        []model.Session
		harness   string
		workspace string
	)
	switch {
	case copilotAgentTranscript(path):
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return CompactionTranscript{}, rerr
		}
		ss, err = parseCopilotAgentTranscript(path, data)
		harness, workspace = "copilot-chat", CopilotChatWorkspaceDir(path)
	case copilotCLIEventLog(path):
		ss, err = ParseCopilotFile(path)
		harness, workspace = "copilot", copilotHeadCWD(path)
	default:
		return CompactionTranscript{}, ErrUnsupportedCompactionTranscript
	}
	if err != nil {
		return CompactionTranscript{}, err
	}
	var s model.Session
	for _, c := range ss {
		if c.ID == nativeSessionID {
			s = c
			break
		}
	}
	if s.ID == "" {
		return CompactionTranscript{}, fmt.Errorf("%w: no session %q in %s", ErrTranscriptIdentity, nativeSessionID, path)
	}
	return CompactionTranscript{
		Session: s, Harness: harness, NativeSessionID: nativeSessionID, Workspace: workspace,
		Path: path, Fingerprint: sessionFingerprint(s), SourceSize: info.Size(), SourceMTime: info.ModTime(),
		// Read through the indexing parser, which keeps no byte offsets, so
		// there is nothing for the compaction-to-edit metric to count against.
		MetricComplete: false,
	}, nil
}
