---
date: 2026-08-28
status: draft design note, research pending
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
- **Disk is complete but late.** Session stores are already present, reach
  back weeks, survive engine downtime, and carry strictly more content
  (full messages, complete tool results, per-turn token usage) than the
  curated OTel events. They are re-readable, which makes capture reliable
  and duplication a real hazard. They are also an internal format that
  churns with harness versions.

A disk reader therefore adds three capabilities OTLP cannot provide:
zero-configuration first-run value ("install tailapp and it already has
your history"), backfill and gap repair for sessions that ran while the
engine was down, and coverage of harnesses that emit no telemetry.

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

The default posture this suggests is *disk-first with OTLP as the live
upgrade*: retrospective tailapps (session-cost and similar) prefer the
complete disk stream; agent-guard needs OTLP latency; a tailapp that wants
one-of-either records per-session coverage (the `telemetry_coverage`
pattern already present in agent-guard) and prefers the live stream when it
exists.

Transcripts contain everything the user and model said, where OTel events
are curated and redacted. Enabling a disk source must be an explicit,
per-harness user choice, never a silent default, and the reader's
documentation must state plainly that disk-fed tailapps see strictly more
sensitive content.

## Duplication

There are two distinct duplication problems with two distinct mechanisms.

**1. Re-reading the same file.** This is the reader's own job. It keeps a
persistent cursor per source file — path, inode, byte offset, and the
identity of the last emitted entry as a rewrite check, since harnesses
compact and rewrite session files. Cursor state lives with the reader
(e.g. under `$TAILAPP_HOME/reader/`), never in the engine: the inbox is
not a history store, records are deleted once consumers settle, and the
engine cannot answer "was this line sent before". Emission is therefore
best-effort exactly-once, degrading to at-least-once on rewrite or cursor
loss, which matches the delivery contract tailapps already have.

**2. Overlap with live telemetry.** Byte-level deduplication cannot work: a
transcript entry and the OTel event for the same action have different
shapes, so `content_digest` never matches across paths. The default stance
is **namespace, don't deduplicate**: disk-sourced records carry distinct
event names (for example `claude_code.transcript.assistant` rather than
the live `claude_code.tool_result`) and an asserted source resource
attribute identifying the reader. Overlap then becomes a semantic join on
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
- **An OTLP mapper** producing log records: asserted `service.name` (the
  reader), harness and file provenance attributes, native identifiers as
  attributes, entry content as body. Records over the engine's 256 KiB
  canonical limit (a large assistant turn with tool results exceeds this
  easily) are truncated or split under a declared policy; the research
  below sizes this.
- **Backpressure**: the receiver answers a full inbox with 503 and
  `Retry-After`; the reader pauses and resumes — unlike a live exporter it
  never needs to drop.

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
   engine-down interval. Keep the resulting transcript files and the
   captured OTLP alongside each other as fixtures in `testdata/`.
2. **Identity comparison.** For each paired session, does any identifier
   appear on both paths? Specifically: is `session.id` in the OTel
   resource/attributes the same value as the transcript's session id
   (Claude Code: the JSONL filename / `sessionId` field)? Do tool-call ids
   in `claude_code.tool_result` / `codex.tool_result` match `tool_use` ids
   in the transcript? Are API message ids present in telemetry at all?
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
5. **Write behavior.** Measure flush cadence and rewrite behavior of each
   store: are lines appended per turn or in bursts, are files rewritten on
   compaction/resume, does inode identity survive, how are concurrent
   sessions interleaved across files? This validates or breaks the
   (path, inode, offset) cursor design.
6. **Size distribution.** Measure entry sizes across real local history
   against the 256 KiB canonical limit and choose the truncation/split
   policy from data.
7. **Subagent topology.** How do both paths represent subagent sessions
   (Claude Code `parent_id`, Codex rollout structure), and should the
   reader emit them as distinct sessions, nested attributes, or excluded
   by default as codeburn does for cost?
8. **Codeburn parser survey.** Read codeburn's Claude Code and Codex
   parsers for edge cases already discovered (schema version drift,
   malformed lines, timezone handling, env-var path overrides such as
   `CLAUDE_CONFIG_DIR`) and record which of its dedup keys are portable.

The output of the study is three artifacts: paired fixtures in
`testdata/`, a per-harness field mapping document, and a decision on
fusion-by-domain-key versus namespace-only, recorded as an amendment to
this note.

## Open questions

- Does the reader live in this repository as a second binary sharing the
  OTLP client code, or in its own repository against the public receiver
  contract?
- Should backfilled records carry their original source timestamps only,
  or also a marker distinguishing backfill from live delivery beyond the
  event-name namespace?
- Is polling sufficient for the "near-live tail" case, or is fsnotify
  needed for any real use — and does any tailapp actually want near-live
  disk data given OTLP exists for that job?
