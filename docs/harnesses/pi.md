# Pi

Pi intentionally keeps both MCP and observability integrations out of its
core. Its extension system can provide them, and third-party packages exist,
but the Tailapps project does not ship or verify a turnkey Pi adapter.

## MCP through a third-party adapter

One example is the third-party `pi-mcp-adapter` package:

```sh
pi install npm:pi-mcp-adapter
```

Its documented shared project format is `.mcp.json`. A `tailapp` MCP entry is:

```json
{
  "mcpServers": {
    "tailapp": {
      "command": "/absolute/path/to/tailapp",
      "args": ["mcp"],
      "env": {
        "TAILAPP_HOME": "/absolute/path/to/.local/share/tailapp"
      }
    }
  }
}
```

Restart Pi, then use the adapter's `/mcp` setup/status interface. This package
is not part of Pi or Tailapps; review its source and release before installing
an extension that runs with your user privileges. `pi-mcp-extension` is
another package listed in Pi's package catalog.

## Telemetry through a third-party extension

Examples currently listed in the ecosystem include:

- `@desek/pi-opentelemetry`, which advertises logs, metrics, and traces over
  OTLP and uses `pi.*` event names; and
- `@mobrienv/pi-otlp`, which advertises OTLP metrics.

These are useful starting points, not plug-compatible Tailapps guarantees. The
`activity-stats`, `agent-guard`, and `session-cost` do not currently recognize
`pi.*` event names, and metrics alone do not drive them. Users and agents can
install a Pi-native Tailapp, fork an included bundle with a `pi.*`
normalizer, or replace the examples with analytics suited to their own adapter
and policy. Translating Pi lifecycle data to the input contracts documented by
[`agent-guard`](../../tailapps/agent-guard/README.md#input-model) and
[`session-cost`](../../tailapps/session-cost/README.md#input-model) is only one
available path.

Point an HTTP/protobuf or HTTP/JSON logs exporter at
`http://127.0.0.1:4318/v1/logs`. The `tailapp` receiver does not accept OTLP
gRPC. Preserve a stable Pi session ID, choose one result event per tool action,
report whether the action succeeded, and distinguish redacted target/progress
data from an observed value.

Until a Pi-aware Tailapp or mapping exists, the Pi MCP adapter can query and
manage Tailapps, but Pi's own activity will not appear in the shipped examples'
tables. See [Author and install a Tailapp](../authoring.md) for the CLI and MCP
lifecycle.

References: [Pi's extension model and intentionally minimal core](https://pi.dev/),
the third-party [Pi MCP Adapter](https://www.npmjs.com/package/pi-mcp-adapter),
[Pi OpenTelemetry extension](https://www.npmjs.com/package/@desek/pi-opentelemetry),
and [Pi OTLP metrics extension](https://www.npmjs.com/package/@mobrienv/pi-otlp).
