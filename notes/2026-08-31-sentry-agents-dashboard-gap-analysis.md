---
date: 2026-08-31
status: operator-ratified competitive-analytics basis; Stage 0 evidence complete
basis: Sentry agent-dashboard documentation and screenshots, Tailapp main c43e7b3
  plus pending #2214 / #2212, harness OTLP documentation, and the scrubbed
  Claude Code 2.1.251 capture in tailapps/testdata/claude-code-2.1.251.json
---

# Gap analysis: Sentry AI Agents dashboards vs Tailapp

Every proposal here uses telemetry already emitted under default or already
documented gates. It does not widen capture into prompts, tool content, or
session stores.

## Operator disposition

- Stage 0 is this evidence refresh only: no collection or retention change.
- Stage 1 is `activity_hourly` plus a roughly sixteen-edge log-duration
  histogram, with an explicit reset-activation review.
- Stage 2 is `model_requests` and `tool_calls`. Raw `tool_name` is ratified as
  an aggregate dimension; this is a retention-posture change, not capture.
- Stage 3 enriches `session_activity` and adds the coding-agent panels.
- A per-session step ledger is excluded pending a separate retention design.

Each stage needs exact-head review plus test, vet, race, and demo evidence.
The fixture records what the vendor actually emitted; it does not turn a
documented event into an invented compatibility claim.

## What Sentry shows

Sentry’s Overview is a consistent-grid time-series dashboard: Agent Runs,
LLM Calls, Avg/P95 Duration, calls/tokens by model, and calls by tool. Its
Models table adds requests, errors, latency, cost and token families; its
Tools table adds requests, errors and latency. The Traces table carries
agents, duration, errors, LLM/tool calls, tokens, cost and time. Its spans and
waterfall drill from those rows into individual steps.

The useful visual grammar is: same-width time buckets, stacked categorical
bars whose legends also give totals, Avg/P95 latency lines, normalized token
shares, drillable rows, and a visibly lighter partial final bucket.

## What Tailapp answers today

| Sentry panel | Tailapp today | Gap |
|---|---|---|
| Agent runs / LLM calls over time | `daily_review` and cumulative `event_inventory` | no intra-day grain |
| Avg + P95 duration | `request_performance` total and five fixed buckets | coarse P95; no series |
| Calls / tokens / cost by model | pending `session_cost_detail` has session × model cost/tokens | no calls or time series |
| Calls/errors by tool | bounded `event_inventory.tool_bucket` | no identity, error or latency table |
| Traces table | `session_activity` | lacks agents, errors, duration |
| Span waterfall | none by design | deliberately no event history |
| Release / environment filters | none | no version or project analytics dimension |

Tailapp differentiators to retain are coverage states, presence-only privacy
indicators, cache-hit share, guard/loop findings, and timestamped timeline
annotations.

## Current telemetry evidence

The fixture is a fresh Claude Code 2.1.251 logs-only capture. It set
`OTEL_LOG_USER_PROMPTS`, `OTEL_LOG_ASSISTANT_RESPONSES`,
`OTEL_LOG_TOOL_CONTENT`, and `OTEL_LOG_TOOL_DETAILS` to `0`. Prompt/response
values were verified redacted before fixture construction; no raw prompt,
response, tool argument, tool content, identity, request, message, or tool-use
identifier is retained in the fixture.

It exercised a read, successful and failing shell tools, a subagent, a
loopback-only terminal HTTP 401 (`CLAUDE_CODE_MAX_RETRIES=0`), and an
intentionally unavailable temporary stdio MCP server. The fixture confirms:

- `api_request`: model, cost, duration, token families, `query_source`,
  `agent.name`, speed, effort.
- `api_error`: model, numeric status, duration and attempt. Its `error` value
  is scrubbed and is not a proposed retained analytics dimension.
- `tool_result`: tool name, success, duration, input/result size.
- `tool_decision`: decision, source and tool source.
- `mcp_server_connection`: status, transport, duration and failure error code.

`api_refusal` and `permission_mode_changed` did not arise in the controlled
run. The former needs a real policy outcome; the latter needs an interactive
mode transition. They remain documented vendor possibilities, not fixture-
proven inputs. No live `app.version`, `skill.name`, `plugin.name`,
`mcp_server.name`, or `mcp_tool.name` was admitted either: they were
unexercised, gated, or redacted.

The existing Codex 0.150.1 fixture has API token/duration/latency/TTFT/status
and tool result/decision fields, but no model; model coverage remains unknown.
The OpenCode DEVtheOPS fixture has model, provider, cost, duration, token,
tool and app-version fields.

## Recommended stages

### Stage 1: time series and useful percentiles

Add `activity_hourly (harness, hour_utc, event_family)` for records, requests,
tools, outcomes, durations, token families and cost. Add the same grain to the
later model/tool tables. Replace the five fixed duration buckets with an
approximately sixteen-edge log histogram from 50 ms to five minutes; retain a
separate Codex TTFT histogram. This makes 24h/7d charting and bounded P50/P95
estimation possible. It changes writable tables, therefore needs a stated
reset-activation release.

### Stage 2: models and tools

Add `model_requests (harness, model, hour_utc)` with requests, errors,
duration histogram, token families, cost and observed/unknown model coverage;
Codex remains unknown until evidence changes. Add `tool_calls (harness,
tool_name, hour_utc)` with calls, failures, duration histogram, size totals,
and Claude decision rejects by source. Raw tool name is the ratified bounded
dimension.

### Stage 3: session and coding-agent panels

Enrich `session_activity` with observed agents (`agent.name`/`query_source`),
API error count, observed duration and session cost-by-model. Add permission
friction, MCP health, observed refusal, cache efficiency, and guard/gap
timeline panels. Per-session step storage stays outside this work because it
would create unbounded retention.

## Presentation follow-up

When #2193 next changes the README report prompt, request the panel grammar:
a 3×2 same-bucket grid; stacked bars with totals; Avg/P95 lines; normalized
token-share area; sortable models/tools/sessions tables; a lighter partial
bucket; and a timeline with session bars and event markers. Reports must keep
the current honest coverage and completeness annotations.
