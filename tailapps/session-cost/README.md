# Session cost

`session-cost` is a compact cumulative-usage Tailapp. It recognizes selected
API-request and completed-stream log events, normalizes their token and cost
fields into a private event, and folds those values into one durable row per
harness and session. It retains aggregates, not the source OTLP stream or
per-request history.

## Input model

The normalizer accepts exactly these event names:

- `claude_code.api_request`
- `codex.api_request`
- `codex.sse_event` when at least one supported token counter is present
- `api_request` from the DEVtheOPS OpenCode plugin 1.5.1 when
  `service.name = opencode`
- `opencode.api_request` from an adapter

It reads session identity from `session.id`, `conversation.id`, `session_id`,
or `service.instance.id`. If all are absent, events are grouped under
`unknown:<harness>`. The normalizer prefers a nonempty semantic `event.name`
attribute over the OTLP log-record name. For `claude-code`, whose current
native records use a short `event.name`, a namespaced `claude_code.*` string
body takes precedence. The `harness` value comes from the
OTLP source, normally `service.name`; Codex's native `codex_cli_rs`,
`codex_exec`, and `codex-app-server` values are normalized to `codex`. Event
time uses the source timestamp when present and otherwise the observed
timestamp.

Generic numeric attributes are `input_tokens`, `output_tokens`,
`cached_input_tokens`, and `reasoning_output_tokens`. Native Codex
`codex.sse_event` attributes are `input_token_count`, `output_token_count`,
`cached_token_count`, and `reasoning_token_count`; the alternate
`cached_input_token_count` and `reasoning_output_token_count` aliases remain
accepted for compatibility. Claude Code's native `cache_read_tokens` maps to
`cached_input_tokens`. Cost may arrive
as the generic `cost_microusd` or Claude Code's native `cost_usd_micros`; the
generic name takes precedence when both are present. Missing individual values
become zero only after at least one supported token or cost counter is present.
A counterless API-request record remains ineffective, so native
latency/status-only `codex.api_request` events do not create zero-valued
unknown-session rows. This Tailapp does not record an `unknown` coverage state
for individual counters; within an effective row, a zero can therefore mean
either a real zero or an absent sibling field.

The DEVtheOPS OpenCode profile maps `reasoning_tokens` to
`reasoning_output_tokens`, `cache_read_tokens` to `cached_input_tokens`, and
rounds `cost_usd` dollars to integer `cost_microusd`. Only its `api_request`
logs are counted. Its LLM and tool spans are ignored both to prevent duplicate
accounting and because they can carry complete prompts, outputs, and tool
parameters.

Codex documents token counts on `response.completed` SSE events. Other
`codex.sse_event` and API-request records remain ineffective unless a supported
counter is present, avoiding a stream of meaningless zero-valued rows. The
checked-in fixture is a structurally representative, scrubbed Codex CLI
0.150.1 capture; vendor fields remain a tested compatibility profile, not an
engine contract.

## Table and export

`session_cost` is both the table and export. Its primary key is
`(harness, session_id)`, and each effective event adds to:

- `input_tokens`
- `output_tokens`
- `cached_input_tokens`
- `reasoning_output_tokens`
- `cost_microusd`

`last_event_time_unix_nano` records the source timestamp or its observed-time
fallback. `last_source_position` identifies the latest consumed inbox
position.

## Query recipes

Usage by session, with cost converted to US dollars:

```sql
SELECT harness, session_id, input_tokens, cached_input_tokens,
       output_tokens, reasoning_output_tokens,
       cost_microusd / 1000000.0 AS cost_usd,
       last_event_time_unix_nano
FROM session_cost
ORDER BY cost_microusd DESC, harness, session_id;
```

Totals by harness:

```sql
SELECT harness,
       SUM(input_tokens) AS input_tokens,
       SUM(cached_input_tokens) AS cached_input_tokens,
       SUM(output_tokens) AS output_tokens,
       SUM(reasoning_output_tokens) AS reasoning_output_tokens,
       SUM(cost_microusd) / 1000000.0 AS cost_usd
FROM session_cost
GROUP BY harness
ORDER BY harness;
```

Join the export into `agent-guard` by mounting it as `cost`:

```sh
tailapp query \
  --mount cost=session-cost \
  --sql 'SELECT p.harness, p.session_id, p.total_actions, c.input_tokens, c.cached_input_tokens, c.output_tokens, c.reasoning_output_tokens, c.cost_microusd FROM session_progress p JOIN cost.session_cost c ON c.harness = p.harness AND c.session_id = p.session_id ORDER BY p.harness, p.session_id' \
  agent-guard
```

Treat the result as cumulative since the last reset activation. It is not a
billing ledger: exporters can omit data, events can be missed while a
projection is detached, and Tailapp v1 does not retain an event log for replay.
Updating a normalizer with continue activation affects only newly consumed
records: already-materialized rows keep their previous harness label. Reset
activation would apply the new label to subsequently received records, but it
also discards the existing projection history; Tailapp cannot replay that
history, so a normalizer update does not force a reset.
Updating an older installed copy to this schema requires reset activation
because the materialized table gains cached-input, reasoning-output, and
last-event-time columns.
