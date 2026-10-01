package sources

// QwenRecordedCWD is the directory a Qwen Code transcript records as cwd, when
// its project folder was named for it; "" otherwise. Qwen names the folder the
// way Claude Code does, so the same check applies. It is read even when the
// directory is gone, where the folder name no longer resolves on disk and is
// ambiguous about which dashes were slashes (#4259).
func QwenRecordedCWD(path string) string {
	return claudeTranscriptCWD(path, QwenProjectDirBase(path))
}
