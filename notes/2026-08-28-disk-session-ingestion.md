---
date: 2026-08-28
status: draft design note, revision 3 after builder reviews 64bc1115 and 4eb42cc3; research pending
companion: notes/2026-08-28-tailapp-architecture.md
rests_on:
  - git:sha1:a24229663c518d72adfe856106d798b615a8a9ef
---

# Disk session-log ingestion

Coding harnesses persist rich session records on disk: Claude Code writes
JSONL transcripts under `~/.claude/projects/`, Codex writes rollout JSONL
under `~/.codex/sessions/`, and most other tools (Cursor, Zed, Copilot,
Gemini CLI, and the rest of the roughly forty tools that
[codeburn](https://github.com/getagentseal/codeburn) parses) keep session
state in JSON, JSONL, or SQLite stores that emit no telemetry at all. This
note proposes ingesting those on-disk stores into Tailapp through a
**sidecar reader** that converts entries to OTLP and posts them to the
existing loopback receiver, and defines the research needed before the
reader's event mapping is committed to.

The core does not change. Architecture decision 2 — *standard OTLP is the
ingestion boundary; the core does not parse Claude Code, Codex, or OpenCode
transcript files* — is preserved, not reversed: transcript parsing lives in
a separate reader process on the far side of the OTLP boundary, exactly
where a harness exporter would sit. The engine keeps a single acceptance
path, a single canonicalization, and no knowledge of file formats.

## Motivation

The two capture paths serve different jobs, and today Tailapp offers only
one of them.

- **OTLP is live but narrow and lossy.** It needs per-harness configuration
  before anything flows, captures nothing from before that moment, exists
  for only a few harnesses, and drops records whenever the engine is down
  or the inbox is full. It is the right path for anything that must react
  while a session runs.
- **Disk is durable but late, and not complete.** Session stores are
  already present, reach back weeks, survive engine downtime, and usually
  carry richer per-turn content (full messages, complete tool results,
  token usage) than the curated OTel events. They are not a superset:
  harnesses compact, rotate, and delete their own stores, and metrics and
  spans exist only on the telemetry path. They are re-readable, which
  makes capture repairable and duplication a real hazard, and they are an
  internal format that churns with harness versions.

A disk reader therefore adds three capabilities OTLP cannot provide:
single-command access to existing history (one explicit enablement, no
per-harness exporter wiring, value on the first run), backfill and gap
repair for sessions that ran while the engine was down, and coverage of
harnesses that emit no telemetry.

Rule of thumb: **OTLP for what is happening, disk for what happened.**

## Choosing between disk and OTLP

For harnesses that support both paths, the choice is driven by:

| Axis | Favors |
|---|---|
| Tool emits no telemetry at all | disk (only option) |
| Live reaction (agent-guard findings, loop detection) | OTLP |
| History, backfill, engine-downtime gap repair | disk |
| Setup friction, first-run experience | disk |
| Capture completeness (cost accounting, yield) | disk |
| Full message/tool-result content | disk |
| Metrics and spans (only exist as telemetry) | OTLP |
| Stable, intentional event contract | OTLP |
| Curated/redacted content, privacy posture | OTLP |

The posture this suggests: once a user has enabled disk capture for a
harness, retrospective tailapps (session-cost and similar) prefer the
disk stream; agent-guard needs OTLP latency; a tailapp that wants
one-of-either records per-session coverage (the `telemetry_coverage`
pattern already present in agent-guard) and prefers the live stream when
it exists.

Enablement is explicit, never a default. Disk ingestion is disabled until
the user enables it per harness, naming the store roots to read; the
enablement surface carries a plain warning that transcripts contain the
full content of sessions — prompts, file contents, tool results — where
OTel events are curated and redacted. The reader documentation must state
that disk-fed tailapps can see substantially more sensitive content than
the telemetry path exposes.

## Duplication

There are two distinct duplication problems with two distinct mechanisms.

**1. Re-reading the same file.** The delivery contract is at-least-once,
stated plainly: OTLP acceptance and cursor advancement are separate
commits, so a crash after the receiver's 2xx and before the cursor
persists re-emits those entries on restart even when no file was
rewritten. Rotation, truncation, compaction, cursor loss, and source
deletion add further duplicate and loss cases. The reader's obligations
are therefore: keep a persistent per-file cursor (path, a source
generation covering inode and rewrite detection, byte offset) that
advances only after the receiver has durably accepted the batch; emit
only complete, newline-terminated entries, holding a partial final line
until its terminator arrives; and stamp every emitted record with a
stable source-entry identity (see the OTLP mapping below) so folds keyed
on that identity make redelivery harmless. Cursor state lives with the
reader (e.g. under `$TAILAPP_HOME/reader/`), never in the engine: the
inbox is not a history store, records are deleted once consumers settle,
and the engine cannot answer "was this line sent before".

**2. Overlap with live telemetry.** Byte-level deduplication cannot work: a
transcript entry and the OTel event for the same action have different
shapes, so `content_digest` never matches across paths. The default stance
is **namespace, don't deduplicate**: disk-sourced records carry distinct
event names (for example `claude_code.transcript.assistant` rather than
the live `claude_code.tool_result`), the `tailapp.capture = "disk"`
marker, and the `tailapp.reader.name` / `tailapp.reader.version`
provenance attributes from the mapping contract below — never a
reader-invented `source`. Overlap then becomes a semantic join on
`(harness, session_id)` inside a tailapp, not an engine problem, which is
exactly where the architecture already places domain deduplication.

Where a tailapp does want to fuse the streams into one logical event, the
reader must surface the shared native identifiers (session id, API message
id, tool-call id, parent/subagent id) as attributes so a fold can key its
tables on domain identity and make duplicates harmless via upsert. Folds
that increment counters per event double-count under fusion; the tailapp
authoring docs must say so. Whether those shared identifiers actually
exist on both paths is the central research question below.

## Reader sketch

A separate process (working name `tailapp-reader`, or a `tailapp sessions`
client subcommand), structured as:

- **Provider parsers**, one per harness store format, starting with Claude
  Code and Codex. Codeburn is the reference implementation for locations,
  formats, and identity keys — Claude API message ids, Codex
  cumulative-token cross-checks, session+message ids elsewhere, and
  exclusion of subagent sessions from cost tallies — but it is an
  analytics endpoint whose JSON output is aggregated, so it is prior art
  to borrow from, not a component to pipe through.
- **A cursor store** as described above, plus discovery (directory scan on
  start, then polling or fsnotify for growth).
- **An OTLP mapper** producing log records under the mapping contract
  below.
- **Backpressure**: the receiver answers a full inbox with 503 and
  `Retry-After`; the reader pauses and resumes. Unlike a live exporter it
  need not drop — but pausing avoids loss only while the source files
  persist. A harness that rotates, compacts, or deletes its store during
  a long pause moves entries out of reach; that limit is part of the
  contract, not an implementation detail.

### OTLP mapping

The engine derives canonical `source` from resource attributes,
`service.name` first (`internal/ingest/canonical.go`, `sourceName()`),
and the shipped tailapps treat that source as the harness. A reader that
asserted `service.name=tailapp-reader` would therefore hide claude_code
and codex from every existing normalizer. The mapping contract is:

- `service.name` carries the exact native value the live telemetry path
  emits for that harness — the observed Codex profile includes both
  `codex_cli_rs` and `codex_exec`, not a normalized alias — so
  canonical `event.source` is indistinguishable across capture paths.
  The definitive per-harness values are recorded by the paired-capture
  research below. Normalizing a native source to a harness key
  (`codex_cli_rs` / `codex_exec` → `codex`) remains the normalizer's
  contract, exactly as on the live path; the reader never invents or
  rewrites a source.
  The reader identifies itself in separate resource attributes
  (`tailapp.reader.name`, `tailapp.reader.version`) — provenance, not
  source.
- Every disk-sourced record carries `tailapp.capture = "disk"` as an
  explicit capture/backfill marker, beyond the event-name namespace.
- Event names are namespaced per entry type:
  `<harness>.transcript.<entry_type>` (e.g.
  `claude_code.transcript.assistant`), disjoint from live event names.
- `time_unix_nano` is the entry's original source timestamp; the observed
  timestamp is the read time.
- Source-entry identity, exposed to folds as attributes: source path,
  source generation (the inode/rewrite epoch the cursor tracks), and
  entry position (index or byte range) — plus the native identifiers
  when present (session id, API message id, tool-call id,
  parent/subagent id). This is the identity folds key on to absorb
  at-least-once redelivery.
- Entries larger than the 256 KiB canonical limit (a large assistant
  turn with tool results exceeds this easily) are handled
  deterministically and visibly: a split emits chunks sharing the parent
  entry identity with `tailapp.chunk.index` / `tailapp.chunk.count`, so
  chunks are distinguishable from independent events; a truncation sets
  `tailapp.truncated = true` and `tailapp.original_bytes`. Which policy
  applies to which entry type is fixed by the reader version, not by
  data; the research below sizes the policy.

Deliberately out of scope for version one: SQLite-backed stores (Cursor,
Zed), watch-mode daemonization, and any engine-side changes.

## Research required

The event mapping and the fusion story both depend on facts about the two
streams that are currently assumed, not known. Before the reader's mapping
is fixed, run a paired-capture study for at least Claude Code and Codex:

1. **Paired capture.** Run scripted sessions in each harness with OTLP
   export wired to a capture endpoint (the existing fixture tooling can
   serve), exercising: plain conversation, tool use with large results,
   permission denial, a subagent/fork, an interrupted session, and an
   engine-down interval. Keep paired transcript and captured-OTLP
   fixtures in `testdata/` only as synthetic sessions or after
   irreversible scrubbing; real local transcripts and tool results must
   not be committed.
2. **Identity comparison.** For each paired session, does any identifier
   appear on both paths? Specifically: is `session.id` in the OTel
   resource/attributes the same value as the transcript's session id
   (Claude Code: the JSONL filename / `sessionId` field)? Do tool-call ids
   in `claude_code.tool_result` / `codex.tool_result` match `tool_use` ids
   in the transcript? Are API message ids present in telemetry at all?
   Record the exact native `service.name` and canonical `event.source`
   values observed per harness on both paths (currently `codex_cli_rs` and
   `codex_exec` for Codex);
   these become the mapping contract's fixed per-harness values.
   The answer determines whether stream fusion by domain key is possible
   or whether namespacing is not just the default but the only option.
3. **Coverage comparison.** Which transcript entries have no telemetry
   counterpart (thinking blocks, full tool results, token usage detail,
   compaction markers) and which telemetry events have no transcript
   counterpart (api_error, tool_decision timing, metrics)? Produce a
   field-level mapping table per harness; this becomes the reader's event
   schema and the tailapp author documentation.
4. **Redaction and privacy delta.** Enumerate concretely what the OTel
   path redacts that the transcript exposes, per harness and per relevant
   setting (e.g. Claude Code's prompt-logging opt-in), so the privacy
   warning in the reader docs is specific rather than vague.
5. **Write behavior and failure matrix.** Measure flush cadence and
   rewrite behavior of each store: are lines appended per turn or in
   bursts, are files rewritten on compaction/resume, does inode identity
   survive, how are concurrent sessions interleaved across files? Then
   exercise the cursor design against the full lifecycle: crash after
   receiver ACK before checkpoint, partial final lines, append during
   read, truncate-in-place, rename/replace, compaction, archived
   sessions, malformed and oversize entries, and source disappearance
   while the reader is paused on backpressure. The path+generation+offset
   design stands or falls on these cases, not on the happy path.
6. **Size distribution.** Measure entry sizes across real local history
   against the 256 KiB canonical limit and choose the truncation/split
   policy from data. Retain aggregate measurements only; no real entry
   content leaves the machine or enters the repository.
7. **Subagent topology.** How do both paths represent subagent sessions
   (Claude Code `parent_id`, Codex rollout structure), and should the
   reader emit them as distinct sessions, nested attributes, or excluded
   by default as codeburn does for cost?
8. **Codeburn parser survey.** Read codeburn's Claude Code and Codex
   parsers for edge cases already discovered (schema version drift,
   malformed lines, timezone handling, env-var path overrides such as
   `CLAUDE_CONFIG_DIR`) and record which of its dedup keys are portable.
   Pin the exact revision surveyed (2512f6e487e530cd52952f3e7f5aff1548245967
   as of 2026-08-28); its provider formats and discovery logic evolve
   actively, so it is prior art to learn from, never a format contract to
   depend on.

The output of the study is three artifacts: paired fixtures in
`testdata/`, a per-harness field mapping document, and a decision on
fusion-by-domain-key versus namespace-only, recorded as an amendment to
this note.

## Open questions

- Does the reader live in this repository as a second binary sharing the
  OTLP client code, or in its own repository against the public receiver
  contract?
- Is polling sufficient for the "near-live tail" case, or is fsnotify
  needed for any real use — and does any tailapp actually want near-live
  disk data given OTLP exists for that job?
