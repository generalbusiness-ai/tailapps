# Activity stats

`activity-stats` turns supported Claude Code, Codex, and OpenCode adapter log
events into privacy-preserving, queryable statistical tables. It stores only
bounded categories, counters, source-supplied lengths and durations, token and
cost totals, coverage counts, and first/last timestamps. It never stores raw
prompts, responses, tool input/output, command text, file or project paths,
account identifiers, request/message/tool-use IDs, or arbitrary attributes.

This is a materialized analytics bundle, not a chart renderer. CLI and MCP SQL
queries are the presentation boundary; a dashboard can consume the same
exports without changing what is retained.

## Input and privacy model

The normalizer recognizes:

- Claude Code: `claude_code.tool_result`, `claude_code.tool_decision`,
  `claude_code.api_request`, `claude_code.assistant_response`, and the optional
  adapter families `claude_code.user_prompt`, `claude_code.turn_ttft`, and
  `claude_code.websocket_request`;
- Codex: `codex.tool_call`, `codex.tool_decision`, `codex.tool_result`,
  `codex.user_prompt`, `codex.turn_ttft`, `codex.api_request`,
  `codex.websocket_request`, and counter-bearing `codex.sse_event`; and
- DEVtheOPS OpenCode plugin 1.5.1 logs: `tool_result` and `api_request` when
  `service.name = opencode`; and
- generic OpenCode adapters: `opencode.tool.execute.before`,
  `opencode.tool.execute.after`, `opencode.api_request`, and the optional
  `opencode.user_prompt`, `opencode.assistant_response`,
  `opencode.turn_ttft`, and `opencode.websocket_request` families.

Codex's native `codex_cli_rs` and `codex_exec` service names become `codex`.
Session identity uses the same bounded fallback as the other bundles; absent
identity is grouped under `unknown:<harness>` and counted as unknown coverage.

The DEVtheOPS profile consumes logs only. Its duplicate `opencode.tool.*`
spans are intentionally ineffective, avoiding double counting and excluding
the prompt, output, and tool-parameter content those spans may carry. Different
OpenCode plugins require separate, named compatibility profiles and fixtures;
their event shapes are not assumed to be interchangeable.

Tool names are reduced to `shell`, `read`, `write`, `edit`, `search`, `other`,
or `unknown`. Endpoint values are reduced to `responses`, `chat-completions`,
`websocket`, `other`, or `unknown`. This prevents arbitrary command or URL text
from becoming a high-cardinality dimension. Lengths are accepted only from
numeric `response_length`, `prompt_length`, `content_length`,
`message_length`, or `body_length` attributes; the normalizer never calculates
a length by reading content. Durations use numeric `duration_ms`, `latency_ms`,
`request_latency_ms`, `ttft_ms`, or `duration_milliseconds` attributes.

The shipped OTLP profiles do not currently provide model, project/cwd, MCP
server, parent/subagent, git, retry, command text, or safe file-target fields.
Therefore by-model and by-project cost, MCP counts, one-shot/retry and
delegation rates, git-yield correlation, and command-type breakdown are
explicitly unavailable here. They require the opt-in disk sidecar designed in
`notes/2026-08-28-disk-session-ingestion.md`; this bundle does not infer them.

## Tables and coverage

- `event_inventory` counts semantic event families by harness and bounded tool
  or endpoint bucket, with first/last seen timestamps and observed/unknown
  counts for session, length, duration, outcome, and token coverage.
- `session_activity` aggregates events, tools, shell-call counts, prompt and
  response counts, requests, outcomes, safe lengths, tokens, cost, and the
  number of unknown field observations per harness/session.
- `request_performance` accumulates API request, websocket, and TTFT counts,
  outcomes, latency totals, unknown duration counts, and fixed cumulative
  buckets at 100, 500, 1,000, and 5,000 milliseconds.

Coverage is per installed Tailapp. An `activity-stats` ineffective record means
only that this bundle did not recognize the record; it does not mean the
receiver dropped it or that `agent-guard`, `session-cost`, or another Tailapp
also found it ineffective. Compare the inventory with installed application
status and each projection's own `ineffective_records` value.

```sql
SELECT harness, event_family, SUM(event_count) AS events,
       SUM(length_unknown_count) AS length_unknown,
       SUM(duration_unknown_count) AS duration_unknown,
       SUM(outcome_unknown_count) AS outcome_unknown,
       SUM(session_unknown_count) AS session_unknown,
       SUM(token_unknown_count) AS token_unknown
FROM event_inventory
GROUP BY harness, event_family
ORDER BY harness, event_family;
```

No row is not evidence of zero activity: the event may be absent, delayed,
redacted before export, or outside this bundle's recognized vocabulary.

## Query recipes

Token volume and cache efficiency by UTC Unix-day number:

```sql
SELECT harness,
       CAST(last_seen_unix_nano AS INTEGER) / 86400000000000 AS unix_day,
       SUM(input_tokens) AS input_tokens,
       SUM(cached_input_tokens) AS cached_input_tokens,
       CASE WHEN SUM(input_tokens + cached_input_tokens) = 0 THEN NULL
            ELSE 100.0 * SUM(cached_input_tokens) /
                 SUM(input_tokens + cached_input_tokens) END AS cache_percent,
       SUM(output_tokens) AS output_tokens,
       SUM(cost_microusd) / 1000000.0 AS cost_usd
FROM session_activity
GROUP BY harness, unix_day
ORDER BY unix_day, harness;
```

Tool frequency and observed outcome rate:

```sql
SELECT harness, tool_bucket, SUM(event_count) AS calls,
       SUM(success_count) AS successes, SUM(failure_count) AS failures,
       SUM(outcome_unknown_count) AS outcome_unknown
FROM event_inventory
WHERE event_family = 'tool'
GROUP BY harness, tool_bucket
ORDER BY calls DESC, harness, tool_bucket;
```

Latency and TTFT distribution:

```sql
SELECT harness, event_family, endpoint_bucket, SUM(request_count) AS requests,
       SUM(duration_observed_count) AS duration_observed,
       SUM(duration_unknown_count) AS duration_unknown,
       SUM(duration_le_100ms) AS le_100ms,
       SUM(duration_le_500ms) AS le_500ms,
       SUM(duration_le_1000ms) AS le_1000ms,
       SUM(duration_le_5000ms) AS le_5000ms,
       SUM(duration_gt_5000ms) AS gt_5000ms
FROM request_performance
GROUP BY harness, event_family, endpoint_bucket
ORDER BY harness, event_family, endpoint_bucket;
```

Run the same statements through MCP with `tailapp_query`, naming
`activity-stats` and passing the SQL unchanged.

For out-of-bound operation evidence, explicit missing policy telemetry,
repetition/no-progress signals, and a practical stalled-session cutoff query,
use the complementary [`agent-guard`](../agent-guard/README.md). Both bundles
are detective: OTLP observation is not inline prevention.
