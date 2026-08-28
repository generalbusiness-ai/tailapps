# Codex

Codex can send native structured log events directly to Tailapp over
OTLP/HTTP. The three examples shipped with this release recognize its tool and
API-request event names.

## Send telemetry

Add the following to the user-level `~/.codex/config.toml`:

```toml
[otel]
environment = "dev"
log_user_prompt = false
exporter = { otlp-http = { endpoint = "http://127.0.0.1:4318/v1/logs", protocol = "binary" } }
```

Codex does not allow a project-local `.codex/config.toml` to override telemetry
routing, so this must be user-level configuration. `binary` means
OTLP/HTTP protobuf; `json` also works with Tailapp. Keep `log_user_prompt =
false` unless your data policy explicitly permits raw prompt export.

Restart Codex after changing the configuration. Codex batches exports and
flushes them on shutdown, so allow for a delay before treating an empty query
as a configuration failure.

## Give Codex access to Tailapp MCP

```sh
codex mcp add \
  --env TAILAPP_HOME="$HOME/.local/share/tailapp" \
  tailapp -- /absolute/path/to/tailapp mcp
codex mcp get tailapp
```

A useful first prompt is:

> Query Tailapp for this session's guard coverage and recent findings. Join
> session-cost if a matching row exists, and distinguish missing telemetry
> from an observed clean result.

## Current bundle fit

The three shipped Tailapps are examples, not a catalog. Users and agents can
fork, extend, replace, or supplement them; the
[authoring guide](../authoring.md) covers installation over CLI and MCP.

`agent-guard` recognizes `codex.tool_call`, `codex.tool_decision`, and
`codex.tool_result`. Its reference policy consumes only the canonical fields
listed in the [`agent-guard` input model](../../tailapps/agent-guard/README.md#input-model).
Codex CLI can export a Rust tracing callsite as the OTLP log-record name while
putting the semantic `codex.*` name in the `event.name` attribute. The shipped
normalizer prefers that semantic attribute and maps the native
`service.name = codex_cli_rs` and `service.name = codex_exec` sources to the
stable `codex` harness name. Query `telemetry_coverage` after a real call to
learn whether the current Codex version exported enough target and progress
detail for a rule. If the query's
`ineffective_records` count rises without coverage rows, inspect the current
memory-only sample with `tailapp ineffective agent-guard` or the MCP tool
`tailapp_ineffective` before changing the normalizer.

`session-cost` recognizes `codex.sse_event` records that carry at least one of
`input_token_count`, `output_token_count`, `cached_token_count`, or
`reasoning_token_count`. It also accepts the older cached and reasoning aliases
`cached_input_token_count` and `reasoning_output_token_count`. Codex documents
these counts on `response.completed`; current native records identify that
completion as `event.kind = response.completed`. Unrelated SSE records remain
ineffective rather than creating zero-valued usage rows. The bundle also
retains its generic `codex.api_request` mapping for adapters. Native Codex
events do not currently provide a cost attribute, so `cost_microusd` remains
zero unless an adapter adds one.

`activity-stats` additionally recognizes Codex prompt-length, TTFT,
latency/status-only API request, and websocket event families. Counterless
`codex.api_request` records remain ineffective for `session-cost` while
contributing request performance to `activity-stats`; ineffectiveness is
per-Tailapp, not a receiver-wide drop.

The checked-in compatibility fixture is a structurally representative,
scrubbed capture from Codex CLI 0.150.1. Vendor fields may change; the official
documentation describes the event families but is not the Tailapp engine
contract. After an upgrade, use `ineffective` and `telemetry_coverage` to check
the live shape. Updating an older installed `session-cost` to this schema adds
cached-input, reasoning-output, and last-event-time columns and therefore
requires reset activation.

Official references: [Codex configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference),
[advanced configuration and telemetry](https://learn.chatgpt.com/docs/config-file/config-advanced),
and [Codex MCP](https://learn.chatgpt.com/docs/extend/mcp).
