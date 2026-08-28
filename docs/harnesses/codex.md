# Codex

Codex can send native structured log events directly to Tailapp over
OTLP/HTTP. The two examples shipped with this release recognize its tool and
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

`agent-guard` and `session-cost` are shipped examples, not the available set of
Tailapp applications. Users and agents are encouraged to fork, extend, replace,
or supplement them with analytics and policy specific to their environment;
the [authoring guide](../authoring.md) covers installation over CLI and MCP.

`agent-guard` recognizes `codex.tool_call`, `codex.tool_decision`, and
`codex.tool_result`. Its reference policy consumes only the canonical fields
listed in the [`agent-guard` input model](../../tailapps/agent-guard/README.md#input-model).
Query `telemetry_coverage` after a real call to learn whether the current Codex
version exported enough target and progress detail for a rule.

`session-cost` recognizes `codex.api_request`, but Codex documentation places
token counts on completed stream events and does not promise them on that API
request event. The bundled revision does not consume `codex.sse_event` or
`codex.websocket_event`, so native token and cost totals may remain zero until
the normalizer is expanded.

Official references: [Codex configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference),
[advanced configuration and telemetry](https://learn.chatgpt.com/docs/config-file/config-advanced),
and [Codex MCP](https://learn.chatgpt.com/docs/extend/mcp).
