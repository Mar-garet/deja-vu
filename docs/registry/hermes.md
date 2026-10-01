# Hermes

- **ID**: `hermes`
- **Store**: `~/.hermes/state.db` (0.17+) or `~/.hermes/profiles/<profile>/state.db` (older builds, one store per profile)
- **Home**: `HERMES_HOME`, Hermes's own variable, is followed — profiles, plugins and `config.yaml` are read and written under it. `DEJA_HERMES_HOME` overrides it.
- **Read overrides**: `DEJA_HERMES_PROFILES_ROOT` for the profiles directory, `DEJA_HERMES_DB` to pin a single store
- **Postgres**: with `sessiondb.provider: postgresql` Hermes stops writing `state.db`; set `DEJA_HERMES_PG_DSN` and deja reads the same columns from the `messages` table through `psql` (#1018)
- **Format**: SQLite relational store

A flat `messages` table, grouped by `session_id`: `role`, `content`, and `timestamp`
as REAL epoch seconds. An assistant row that calls tools has no `content` and an
OpenAI-style `tool_calls` array instead; the result lands on a `tool` row under the
same `tool_call_id`. Calls to `terminal` become commands: a non-zero `exit_code` from
the result rides on the command, and `-1`, which Hermes returns for a command it never
ran (denied, blocked, waiting on approval), drops the command record. `read_file`,
`write_file` and `patch` name the files, and `patch` and `write_file` give the edit and
written sides — `patch` in its V4A mode as Hermes' own parser reads it, `Move File`
included. A `tool` row is kept as tool output by what it says (`output`, `content`,
`diff`, `matches_text`, `error`); bookkeeping such as a byte count is not. Rows
rewound away (`active = 0`, `compacted = 0`) are skipped, a call compaction wrote again
counts once, and its compressor stubs are dropped. Multimodal content, stored as
`\x00json:` and a list of parts, keeps its text parts and not the image (#4242). The
Postgres path still reads prose only. A `sessions` table beside it carries `id`, `cwd`,
`git_repo_root` and `title`; the `cwd` is where the work happened and is what names
the project, the same way a Cline or Roo workspace does. A store without that table,
or a session whose row has no `cwd`, falls back to the profile name. The title is left
to the index rather than taken from the first row.

- **MCP**: `mcp_servers` in `~/.hermes/config.yaml`; `deja install hermes-auto` also drops a plugin (`~/.hermes/plugins/deja`, added to `plugins.enabled`) whose `pre_llm_call` hook injects recall and registers `/deja`, and a memory provider (`deja-memory`) that `hermes memory setup` lists next to mem0 and supermemory — the same recall in the `memory.provider` slot, with `deja_recall`, `deja_fix` and `deja_blame` as its tools. The provider is written against the interface Hermes has: `RecallStatus` arrived after `MemoryProvider`, so it is imported behind a guard and the status line is skipped where the class is missing — checked against Hermes 0.17.0, which has none. The hook stands aside for the provider only after loading it, so a provider that cannot import leaves the hook doing the recall rather than nobody.
- **Skill**: `~/.hermes/skills/deja-history/SKILL.md`, top-level rather than inside the plugin: a plugin-bundled skill is opt-in in Hermes and never reaches the system prompt.
- **Resume**: `hermes --resume <id>`; Hermes takes the same session id deja indexes, so it reopens that conversation rather than the most recent one.
- **Handoff**: paste.

Requested in [#355](https://github.com/vshulcz/deja-vu/issues/355).

**Last verified:** 2026-07-28
