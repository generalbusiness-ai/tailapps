# OpenCode

OpenCode has a native MCP client, but its current documentation does not
describe a native OTLP exporter for harness events. Tailapp therefore has a
split setup: MCP is ready to configure; telemetry requires an OpenCode plugin
or another adapter that you supply.

## Give OpenCode access to Tailapp MCP

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

## Telemetry adapter contract

OpenCode plugins can observe `tool.execute.before` and `tool.execute.after`
hooks. A compatible adapter sends OTLP/HTTP logs to
`http://127.0.0.1:4318/v1/logs` using JSON or protobuf and sets
`service.name = opencode`.

The two examples shipped with this release recognize:

- `opencode.tool.execute.after` for a completed tool call;
- `opencode.tool.execute.before` for an attempted call with no result; and
- `opencode.api_request` for usage.

Supply the canonical fields described by
[`agent-guard`](../../tailapps/agent-guard/README.md#input-model) and
[`session-cost`](../../tailapps/session-cost/README.md#input-model). Avoid
emitting both before and after as independently counted actions for one
completed call. Tailapp does not ship or install this plugin in the current
release.

Until an adapter is active, MCP queries and Tailapp management work, but the
two shipped examples' tables receive no OpenCode rows. This is expected rather
than a receiver failure. Those examples are not a closed catalog: users and
agents are encouraged to install an OpenCode-native Tailapp, fork their
normalizers or policies, or substitute entirely different analytics through
the [CLI or MCP lifecycle](../authoring.md).

Official references: [OpenCode plugins](https://opencode.ai/docs/plugins/)
and [OpenCode MCP servers](https://dev.opencode.ai/docs/mcp-servers/).
