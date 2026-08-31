# Session cost

`session-cost` is a compact cumulative-usage Tailapp. It recognizes selected
API-request and completed-stream log events, normalizes their token and cost
fields into a private event, and folds those values into one durable row per
harness and session. A second export retains model, project, readable session
prefix, and first/last timestamps when those dimensions are present in OTLP.
It does not read harness session stores or retain per-request history.

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

Model uses `model`, `gen_ai.request.model`, `gen_ai.response.model`,
`request.model`, or `model_name`. Project uses `project`, `project.name`,
`project_path`, `cwd`, `working_directory`, or `workspace.root`. These values
are retained as supplied because Tailapp runs locally for the same user. Each
dimension has an explicit `observed`/`unknown` coverage field; it is never
inferred from a local session store.

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

`session_cost` remains the compact compatibility table and export. Its primary key is
`(harness, session_id)`, and each effective event adds to:

- `input_tokens`
- `output_tokens`
- `cached_input_tokens`
- `reasoning_output_tokens`
- `cost_microusd`

`last_event_time_unix_nano` records the source timestamp or its observed-time
fallback. `last_source_position` identifies the latest consumed inbox
position.

`session_cost_detail` adds `session_id_prefix`, model, project, coverage, and
first/last event timestamps at `(harness, session_id, model)` grain. A session
that uses multiple models therefore has one row per model rather than having
all of its usage attributed to the last model observed. Events without a model
remain in an explicit `unknown` row. Project stays a session-level label within
each model row; a later observed project can fill an earlier unknown value.

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

Cost by model and project, with unknown coverage visible:

```sql
SELECT harness, model, project, model_coverage, project_coverage,
       SUM(cost_microusd) / 1000000.0 AS cost_usd,
       SUM(input_tokens) AS input_tokens,
       SUM(cached_input_tokens) AS cached_input_tokens,
       SUM(output_tokens) AS output_tokens
FROM session_cost_detail
GROUP BY harness, model, project, model_coverage, project_coverage
ORDER BY cost_usd DESC, harness, model, project;
```

Readable session-and-model labels without inventing names:

```sql
SELECT harness, session_id_prefix, first_event_time_unix_nano,
       last_event_time_unix_nano, project, model,
       cost_microusd / 1000000.0 AS cost_usd
FROM session_cost_detail
ORDER BY first_event_time_unix_nano DESC, harness, session_id_prefix, model;
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
projection is detached, and the current `tailapp` engine does not retain an
event log for replay.
Updating a normalizer with continue activation affects only newly consumed
records: already-materialized rows keep their previous harness label. Reset
activation would apply the new label to subsequently received records, but it
also discards the existing projection history; the `tailapp` engine cannot
replay that history, so a normalizer update does not force a reset.
The detailed table is additive, so a continue activation preserves the compact
history and begins detailed coverage at the activation boundary.

This release also changes the runtime identity, so an existing resident holds
ingestion closed until every active Tailapp is explicitly continued or reset.
Follow [Upgrading an existing resident](../../docs/reference/cli.md#upgrading-an-existing-resident);
continuing `session-cost` preserves its compact history and starts this new
model-grained detail at the activation boundary.
