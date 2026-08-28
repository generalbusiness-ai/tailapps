# Session cost

`session-cost` is a compact cumulative-usage Tailapp. It recognizes selected
API-request log events, normalizes their token and cost fields into a private
event, and folds those values into one durable row per harness and session. It
retains aggregates, not the source OTLP stream or per-request history.

## Input model

The normalizer accepts exactly these event names:

- `claude_code.api_request`
- `codex.api_request`
- `opencode.api_request` from an adapter

It reads session identity from `session.id`, `conversation.id`, `session_id`,
or `service.instance.id`. If all are absent, events are grouped under
`unknown:<source>`. The `harness` value comes from the OTLP source, normally
`service.name`.

The numeric token attributes are `input_tokens` and `output_tokens`. Cost may
arrive as the generic `cost_microusd` or Claude Code's native
`cost_usd_micros`; the generic name takes precedence when both are present.
Missing values become zero; this Tailapp does not currently record an `unknown`
coverage state. A zero can therefore mean either a real zero or an
absent/unmapped field.

Current Codex documentation places response token counts on stream-completion
events rather than promising them on `codex.api_request`. Those values require
an adapter mapping before this bundled revision will aggregate them.
Cache-read, cache-creation, and reasoning-token fields are also outside this
initial model.

## Table and export

`session_cost` is both the table and export. Its primary key is
`(harness, session_id)`, and each effective event adds to:

- `input_tokens`
- `output_tokens`
- `cost_microusd`

`last_source_position` identifies the latest consumed inbox position, not a
timestamp.

## Query recipes

Usage by session, with cost converted to US dollars:

```sql
SELECT harness, session_id, input_tokens, output_tokens,
       cost_microusd / 1000000.0 AS cost_usd
FROM session_cost
ORDER BY cost_microusd DESC, harness, session_id;
```

Totals by harness:

```sql
SELECT harness,
       SUM(input_tokens) AS input_tokens,
       SUM(output_tokens) AS output_tokens,
       SUM(cost_microusd) / 1000000.0 AS cost_usd
FROM session_cost
GROUP BY harness
ORDER BY harness;
```

Join the export into `agent-guard` by mounting it as `cost`:

```sh
tailapp query \
  --mount cost=session-cost \
  --sql 'SELECT p.harness, p.session_id, p.total_actions, c.input_tokens, c.output_tokens, c.cost_microusd FROM session_progress p JOIN cost.session_cost c ON c.harness = p.harness AND c.session_id = p.session_id ORDER BY p.harness, p.session_id' \
  agent-guard
```

Treat the result as cumulative since the last reset activation. It is not a
billing ledger: exporters can omit data, events can be missed while a
projection is detached, and Tailapp v1 does not retain an event log for replay.
