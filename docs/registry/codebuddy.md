# CodeBuddy Code

- **ID**: `codebuddy`
- **Store**: `${CODEBUDDY_CONFIG_DIR:-~/.codebuddy}/projects/<mangled-cwd>/<session-id>.jsonl` — one file per session
- **Sub-agents**: `<mangled-cwd>/<session-id>/subagents/agent-<id>.jsonl`, read with `DEJA_INCLUDE_SUBAGENTS=1`
- **WorkBuddy**: `${WORKBUDDY_CONFIG_DIR:-~/.workbuddy}/projects/...`, the same layout and records, read when it exists
- **Read overrides**: `DEJA_CODEBUDDY_ROOTS`, a path list, replaces both stores
- **Format**: JSONL — OpenAI Responses-style items
- **Needs**: nothing
- **Wiring**: `deja install codebuddy` adds the MCP server; `deja install codebuddy-auto` adds the hooks as well

CodeBuddy Code is Tencent's terminal agent (`@tencent-ai/codebuddy-code`). Its
store is laid out like Claude Code's — a directory per working directory, one
JSONL per session — but the directory name is the cwd with `/`, `\` and `:`
turned into `-` and the leading dash dropped, and the records inside are not
Claude Code's. Each line is one item:

```
{"type":"message","role":"user","content":[{"type":"input_text","text":"..."}],"timestamp":1780000000000,"cwd":"/repo","providerData":{...}}
{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"..."}]}
{"type":"function_call","callId":"call_1","name":"Bash","arguments":"{\"command\":\"go test ./...\"}"}
{"type":"function_call_result","callId":"call_1","name":"Bash","output":{"type":"text","text":"..."}}
```

plus `reasoning`, `summary` (the compaction digest), `ai-title`,
`custom-title`, `topic`, `turn-metrics`, `file-history-snapshot` and
`session-meta`. Timestamps are epoch milliseconds and `cwd` is on the record.
The tools are Claude Code's by name and arguments (`Bash`, `Read`, `Edit`,
`Write`, `MultiEdit`, `file_path`, `old_string`), so a call becomes a command,
file, edit or wrote record the way it does for Claude Code, and a
`function_call_result` becomes tool output.

Role `user` is shared with the client's own plumbing. A record with
`providerData.skipRun` never reached the model as anyone's words (hook output,
filter notices, local commands), and `isMeta`, `isCompactInternal`,
`isCompacted`, `isSummary`, `compactType` and `teammateMessage` mark
continuation, compaction and teammate turns. All are dropped, which is what
CodeBuddy's own `isRealUserMessageItem` does. A text item that opens with
`<command-name>`, `<system-reminder>`, `<local-command-stdout>`,
`<teammate-message>` or a task notification is dropped from the turn it rides in.
`reasoning` and `summary` are not indexed.

The title follows CodeBuddy's own order: the newest `custom-title`, then the
newest `ai-title`, then the newest `topic`, skipping the placeholders it skips.

MCP servers go into the first of `<config>/.mcp.json`, `<config>/mcp.json` and
`~/.codebuddy.json` that exists, else `<config>/.mcp.json` — the one file
CodeBuddy reads for user scope. Hooks go into `<config>/settings.json` in the
Claude Code shape, `timeout` in seconds: `SessionStart`, `UserPromptSubmit`,
`PostToolUse` and `PostToolUseFailure` on `Bash|PowerShell`, `PreCompact` and
`SessionEnd`. CodeBuddy reads `hookSpecificOutput.additionalContext` from all
of them the way Claude Code does.

**Last verified:** 2026-10-05

## Known quirks and drift

- **Read from the bundle, not a live store.** The record shapes, the store
  path, `CODEBUDDY_CONFIG_DIR`, the plumbing flags, the title order, the MCP
  file order and the hook events were read out of
  `@tencent-ai/codebuddy-code` 2.161.2 (`dist/codebuddy.js`) and CodeBuddy's
  CLI docs; the fixture is synthetic.
- **The IDE store is separate.** The IDE and the VS Code extension keep their
  history under `CodeBuddyExtension/Data/...` in the platform data directory,
  never in this tree. Not read yet (#4681).
- **No exit codes.** The Bash result text carries no exit status line deja
  has checked, so a command is stored without `→ exit N`.
- **WorkBuddy shares the harness id.** Its sessions read as `codebuddy`.
