# Gap analysis: codeburn charts vs. tailapp OTLP coverage (main @ 055687c)

Scope justification for the activity-stats bundle (#764). Sources: the two
shipped compatibility fixtures (codex-cli-0.150.1.json, claude-code-2.1.250.json),
both bundled normalizers, and a repo-wide grep confirming ZERO occurrences of
model / mcp / cwd / gen_ai.* attributes anywhere.

## Decisive finding

Codeburn reads the coding tools' on-disk session files, which carry `model`,
full `arguments.command`, `arguments.path`, project path, and `parent_id`. The
OTLP telemetry tailapp receives carries NONE of those. So codeburn's marquee
charts split cleanly into "feasible from OTLP now" and "needs the disk reader."

## Per-chart verdict

| Codeburn chart | Needs | Over OTLP today | Verdict |
|---|---|---|---|
| Cache efficiency % | cache_read + input tokens | mapped (cached_input_tokens, input_tokens) | SHIP NOW |
| Cost / token volume by day | tokens/cost + timestamp | mapped (event_time_unix_nano) | SHIP NOW (Claude cost real; Codex cost=0) |
| Core tools count / frequency | tool_name per call | mapped (agent-guard tool) | SHIP NOW |
| Tool outcome rates | success flag | mapped (agent-guard success) | SHIP NOW |
| Shell commands COUNT | count Bash/exec_command events | tool name mapped | SHIP NOW (count only) |
| Cost/usage BY MODEL | model name | ABSENT from both fixtures/grep | BLOCKED -> disk |
| Cost BY PROJECT | cwd/project path | ABSENT | BLOCKED -> disk |
| MCP servers count | mcp identity/config | ABSENT | BLOCKED -> disk/config |
| One-shot / retry rate | file path per edit + order | target is <scrubbed>/absent | BLOCKED -> disk |
| Delegation rate | parent/subagent id | ABSENT (trace_id/span_id null) | BLOCKED -> disk |
| Yield / git correlation | git log + project | ABSENT | BLOCKED -> disk |
| Shell command breakdown by type | command text | <scrubbed> by harness (never interpreted) | BLOCKED -> disk |

## Event families in the operator coverage table not yet captured by any bundle

| Event family | Value | Feasible over OTLP? |
|---|---|---|
| Claude assistant_response | response count + length (no text) | yes (count + body/attr size) |
| Codex turn_ttft | time-to-first-token distribution | yes (ttft attr) |
| Codex api_request | endpoint latency + status (NOT cost) | yes; today it makes a useless zero-cost session-cost row |
| Codex websocket_request | transport success + latency | yes (moderate) |
| Codex user_prompt | prompt count + length only | yes (count + size) |
| Empty internal callsite events | none | ignore |

## Conclusion

Build the counting / length / latency / distribution charts as an OTLP-native
activity-stats bundle now (feasible today, privacy-preserving). The
by-model / by-project / MCP-count / one-shot / yield charts are the payoff that
justifies the disk-ingestion sidecar reader
(notes/2026-08-28-disk-session-ingestion.md) — they need fields that live only
in the tools' own on-disk files and never cross the OTLP boundary.
