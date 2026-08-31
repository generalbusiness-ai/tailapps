# Agent guard

`agent-guard` is a reference detective-policy Tailapp. It normalizes selected
tool events from Claude Code, Codex, and OpenCode into one private action
vocabulary, then folds each short-lived event into durable coverage, policy,
loop, session-progress, and failed-tool detail tables. It does not retain the
source OTLP stream. For observed failures it does retain available command,
arguments, error detail, project, and model so a local same-user review can
explain the failure rather than merely count it.

The model separates an observed violation from missing evidence. A result can
prove that a named action occurred, but an absent or redacted target cannot
prove that the action stayed within a boundary. Missing session, tool, or
target fields therefore produce `unknown` coverage and an
`insufficient-telemetry` finding rather than a compliance claim.

This is a starting policy, not a universal policy pack. Its built-in rules
flag:

- tools named `dangerous_shell` or `external_network`;
- operation kinds named `destructive` or `network_external`; and
- observed targets beginning with `/outside/`.

Adapt `folds/normalize.jsonata` to derive the canonical fields available from
your harness, and adapt `folds/guard.jsonata` to your actual tool names, roots,
and operation classes before relying on the findings operationally.

## Input model

The normalizer accepts these event names:

- Claude Code: `claude_code.tool_result`, `claude_code.tool_decision`
- Codex: `codex.tool_result`, `codex.tool_decision`, `codex.tool_call`
- DEVtheOPS OpenCode plugin 1.5.1 logs: `tool_result` when
  `service.name = opencode`
- OpenCode adapters: `opencode.tool.execute.after`,
  `opencode.tool.execute.before`

It reads session identity from `session.id`, `conversation.id`, `session_id`,
or `service.instance.id`; tool identity from `tool_name` or `tool`; target from
`target`, `file_path`, or `full_command`; and optional `success`/`tool.success`,
`operation_kind`, `argument_digest`, and `progress_fingerprint` attributes.
The normalizer prefers a nonempty semantic `event.name` attribute over the
OTLP log-record name, which accommodates Codex's tracing-callsite envelope.
For `claude-code`, whose current native records use a short `event.name`, a
namespaced `claude_code.*` string body takes precedence.
The canonical `harness` comes from the OTLP source, normally `service.name`;
the native `codex_cli_rs`, `codex_exec`, and `codex-app-server` values are
normalized to `codex`. Event time uses the
source timestamp when present and otherwise the observed timestamp. A
recognized event with neither timestamp remains ineffective.

For an observed `success = false`, the normalizer retains up to 8 KiB each of
available command, tool arguments, and failure detail in
`tool_failure_detail`. Structured values are retained as their JSON string.
Successful and unknown-outcome calls do not retain these payloads. The detail
comes only from OTLP attributes; no harness session store is consulted.

Do not emit both an OpenCode `before` and `after` record for one completed call
unless two counted actions are intended. Prefer `after` for completed calls and
reserve `before` for an attempted call that will not have a corresponding
result.

The DEVtheOPS plugin also emits `opencode.tool.*` spans for the same actions.
They are deliberately ineffective here to avoid double counting and because
those spans can carry raw tool parameters and results. Its log record already
supplies `tool_name` and `success`; target and progress coverage normally remain
unknown.

## Analytics model

`session_progress` keeps one row per harness and session. Exact action
fingerprints count consecutive repeated actions; observed `success = false`
values count consecutive failures; repeated observed progress fingerprints
count bounded no-progress. A run becomes a loop finding at three consecutive
observations. The fold selects one finding kind per input in this order:
repeated failure, repeated action, then bounded no-progress.

When any action-fingerprint input is absent, gated, or redacted, repeated-action
evidence marks `action_fingerprint_coverage` as `degraded` and carries an
`action_fingerprint_reason`. For example, without target detail the fingerprint
can distinguish tools but not their arguments or targets, so three consecutive
calls to the same tool can satisfy the repetition threshold.

`bounded-no-progress` requires an observed `progress_fingerprint`. The native
Claude Code, Codex, and OpenCode telemetry shapes documented here do not
currently provide one, so that finding cannot fire for those records until a
harness or adapter emits explicit progress fingerprints. Missing progress
telemetry remains `unknown`; it is never counted as evidence of no progress.

This is bounded event evidence, not a wall-clock monitor. A silent process
emits no record, so stalled behavior is detected by comparing the last distinct
progress timestamp with a caller-supplied cutoff.

The queryable tables and exports are:

- `telemetry_coverage`: latest observed/unknown state per harness capability,
  with first/last observed timestamps;
- `session_progress`: rolling counters and progress timestamps per session;
- `policy_findings`: per-position violations plus stable per-session unknowns,
  with the triggering record timestamp;
- `loop_findings`: latest evidence per session and loop kind, with the first
  and last timestamps of the aggregated run;
- `tool_failure_detail`: one row per observed failed tool call, including raw
  telemetry detail and explicit observed/partial/unknown coverage.

## Query recipes

Recent policy evidence:

```sql
SELECT observed_unix_nano, harness, session_id, rule_id, severity,
       summary, evidence, coverage_state
FROM policy_findings
ORDER BY source_position DESC;
```

Current instrumentation coverage:

```sql
SELECT harness, capability, state, reason, first_seen_unix_nano,
       last_seen_unix_nano, last_source_position
FROM telemetry_coverage
ORDER BY harness, capability;
```

Loop evidence:

```sql
SELECT first_observed_unix_nano, last_observed_unix_nano,
       harness, session_id, finding_kind, repeat_count,
       consecutive_failures, no_progress_count, evidence
FROM loop_findings
ORDER BY source_position DESC;
```

Failed commands and tool details:

```sql
SELECT event_time_unix_nano, harness, session_id_prefix, project, model,
       tool, command, tool_arguments, target, failure_detail, detail_coverage
FROM tool_failure_detail
ORDER BY source_position DESC;
```

A truly idle process emits no event, so stalled detection is a query-time
check. Compute a nanosecond cutoff in the caller and bind it:

```sql
SELECT harness, session_id, last_activity_unix_nano,
       last_distinct_progress_unix_nano, total_actions
FROM session_progress
WHERE last_distinct_progress_unix_nano < ?
ORDER BY last_distinct_progress_unix_nano, harness, session_id;
```

```sh
tailapp query --sql 'SELECT harness, session_id, last_distinct_progress_unix_nano FROM session_progress WHERE last_distinct_progress_unix_nano < ? ORDER BY last_distinct_progress_unix_nano' --param '"1787900000009999999"' agent-guard
```

`loop_findings` retains the latest row per session and finding kind. Policy
violations retain per-position evidence, while insufficient telemetry uses a
stable per-session ID to avoid a missing-field firehose. This remains detective
evidence: Tailapp neither blocks a call nor proves that an unobserved operation
did not happen.

The finding and coverage timestamp columns change existing writable table
shapes. Upgrading an older installed `agent-guard` therefore requires reset
activation and discards its materialized guard history. `session-cost`,
`daily-review`, and `activity-stats` are unaffected by this storage change.
