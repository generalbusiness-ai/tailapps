# OpenCode

OpenCode has a native MCP client, but its current documentation does not
describe a native OTLP exporter for harness events. The Tailapps project
therefore has a split setup: MCP is configured directly, while telemetry comes
from a plugin. Plugins are independent producers and can emit different names
and attributes; the project documents compatibility per named plugin profile
rather than treating "OpenCode telemetry" as one wire contract.

## Give OpenCode access to Tailapps over MCP

Add a local server to `opencode.json` or the applicable user configuration,
using absolute paths:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "tailapp": {
      "type": "local",
      "command": ["/absolute/path/to/tailapp", "mcp"],
      "enabled": true,
      "environment": {
        "TAILAPP_HOME": "/absolute/path/to/.local/share/tailapp"
      }
    }
  }
}
```

Run `opencode mcp list` to check the connection. OpenCode prefixes tools with
the MCP server name, so ask the agent to use the `tailapp` tools explicitly.

## Send telemetry with the DEVtheOPS plugin

The checked-in compatibility fixture was validated against
[`@devtheops/opencode-plugin-otel`](https://github.com/DEVtheOPS/opencode-plugin-otel)
1.5.1. Add this entry to the existing `plugin` array in `opencode.json` or the
applicable user configuration:

```json
[
  "@devtheops/opencode-plugin-otel",
  {
    "enabled": true,
    "endpoint": "http://127.0.0.1:4318",
    "protocol": "http/protobuf",
    "capturePromptInLogs": false,
    "disabledTraces": ["all"]
  }
]
```

The endpoint is the receiver base URL, not `/v1/logs`; the plugin appends the
signal paths. Restart OpenCode after changing plugin configuration. Its first
startup may install the package.

The trace exclusion is intentional. In this plugin, LLM, session, and tool
spans can contain complete prompts, model outputs, tool parameters, and tool
results even when `capturePromptInLogs` is false. The logs and metrics needed
for the shipped privacy-preserving analytics continue to be exported with all
traces disabled. Enable traces only when every local consumer is trusted to
receive that content.

The DEVtheOPS profile sets `service.name = opencode` and emits unprefixed log
event names. The `tailapp` engine consumes logs as the single counting source:

- `tool_result`, with `session.id`, `tool_name`, `success`, and `duration_ms`;
- `api_request`, with token counters, `cost_usd`, and `duration_ms`.

`session-cost` maps `reasoning_tokens` to `reasoning_output_tokens`,
`cache_read_tokens` to `cached_input_tokens`, and rounded US dollars to integer
microdollars. The plugin also emits `opencode.tool.*` spans with dotted
`tool.name` and `tool.success` attributes. The shipped bundles deliberately do
not count those duplicate, content-bearing spans.

For a different plugin, first inspect `tailapp ineffective APP`, then adapt or
fork the normalizer for that producer's signal, service/scope identity, event
names, units, and attributes. The older generic adapter vocabulary remains
accepted for custom adapters:

- `opencode.tool.execute.after` for a completed tool call;
- `opencode.tool.execute.before` for an attempted call with no result; and
- `opencode.api_request` for usage.

Supply the canonical fields described by
[`agent-guard`](../../tailapps/agent-guard/README.md#input-model) and
[`session-cost`](../../tailapps/session-cost/README.md#input-model). Avoid
emitting both before and after as independently counted actions for one
completed call. The Tailapps project does not ship or install this plugin in
the current release.

`activity-stats` accepts the DEVtheOPS logs and the generic adapter records,
reduces tool and endpoint values to bounded categories, and retains only
aggregates. Its optional prompt/response/TTFT/websocket names are generic
adapter contracts, not claims that this plugin emits those records.

After one model response and tool call, `tailapp metrics --json` should show
log and metric intake and the projection frontiers should drain. Query
`session-cost` and `agent-guard` for `harness = 'opencode'`; use `ineffective`
to diagnose a plugin-version or profile mismatch. Until an adapter is active,
MCP still works but no OpenCode rows are expected.

These examples are not a closed catalog: users and agents are encouraged to
install an OpenCode-native Tailapp, fork their normalizers or policies, or
substitute entirely different analytics through the
[CLI or MCP lifecycle](../authoring.md).

Official references: [OpenCode plugins](https://opencode.ai/docs/plugins/)
and [OpenCode MCP servers](https://opencode.ai/docs/mcp-servers/).
