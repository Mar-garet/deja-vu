package sources

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/vshulcz/deja-vu/internal/model"
)

// Command Code (commandcode.ai) writes one transcript per session under a
// Claude-Code-style project directory:
//
//	~/.commandcode/projects/<encoded-cwd>/<session>.jsonl
//
// Each line is one message, flat: role, content, timestamp, sessionId. Beside
// it sits `<session>.checkpoints.jsonl`, a snapshot stream rather than a
// conversation — read as a transcript it adds a session with no words in it, so
// it is skipped by name (#3647).

// CommandCodeRoot is the project store root. DEJA_COMMANDCODE_ROOT replaces it.
func CommandCodeRoot() string {
	return EnvPath("DEJA_COMMANDCODE_ROOT", filepath.Join(Home(), ".commandcode", "projects"))
}

// CommandCodeSessionFiles lists the transcripts, checkpoints excluded.
func CommandCodeSessionFiles() []string {
	return walkFiles(CommandCodeRoot(), commandCodeIsTranscript)
}

// commandCodeIsTranscript reports whether a path is a conversation rather than
// the checkpoint stream that shares its extension.
func commandCodeIsTranscript(p string) bool {
	return strings.HasSuffix(p, ".jsonl") && !strings.HasSuffix(p, ".checkpoints.jsonl")
}

// CommandCodeCheckpointFiles lists the snapshot streams sitting beside the
// transcripts. deja does not read them — they are not conversations — and they
// are named so `deja doctor` can count them as a deliberate skip rather than
// as a file it failed to understand, which is how a store reports drift.
func CommandCodeCheckpointFiles() []string {
	return walkFiles(CommandCodeRoot(), func(p string) bool {
		return strings.HasSuffix(p, ".checkpoints.jsonl")
	})
}

// CommandCodeUnderRoot lets the registry claim a path for incremental ingest.
func CommandCodeUnderRoot(p string) bool {
	return strings.HasPrefix(p, CommandCodeRoot()) && commandCodeIsTranscript(p)
}

func LoadCommandCode() []model.Session {
	return parseFiles(CommandCodeSessionFiles(), ParseCommandCodeFile)
}

// ParseCommandCodeFile reads one transcript.
func ParseCommandCodeFile(path string) ([]model.Session, error) {
	return ParseCommandCodeFileFromOffset(path, 0)
}

// ParseCommandCodeFileFromOffset is the incremental read.
func ParseCommandCodeFileFromOffset(path string, offset int64) ([]model.Session, error) {
	return parseFlatRoleJSONL(path, offset, "commandcode", commandCodeProject(path))
}

func commandCodeProject(path string) string {
	dir := projectDir(CommandCodeRoot(), path)
	if dir == "" || dir == CommandCodeRoot() {
		return ""
	}
	return claudeProjectName(dir)
}

// CommandCodeSessionDir is the directory a session ran in: the cwd on the v3
// header line, or on any of the first records that carries one. The folder
// name is a lossy slug of it. "" when none names an absolute path (#4372).
func CommandCodeSessionDir(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	r := bufio.NewReader(f)
	for i := 0; i < claudeCWDScanLines; i++ {
		line, err := r.ReadBytes('\n')
		if bytes.Contains(line, []byte(`"cwd"`)) {
			var v struct {
				CWD string `json:"cwd"`
			}
			if json.Unmarshal(line, &v) == nil && filepath.IsAbs(v.CWD) {
				return v.CWD
			}
		}
		if err != nil {
			return ""
		}
	}
	return ""
}
