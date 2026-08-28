# Claude Code

Claude Code can send its native structured log events directly to Tailapp over
OTLP/HTTP. The two examples shipped with this release recognize its
tool-result, tool-decision, and API-request event names.

## Send telemetry

Start Claude Code with this environment, or put equivalent values in the
Claude Code settings used by your organization:

```sh
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_LOGS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_LOGS_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_LOGS_ENDPOINT=http://127.0.0.1:4318/v1/logs
export OTEL_LOG_TOOL_DETAILS=1
claude
```

Use the signal-specific endpoint exactly as shown, including `/v1/logs`.
Tailapp accepts both `http/protobuf` and `http/json`; it does not accept OTLP
gRPC. `OTEL_LOG_TOOL_DETAILS=1` exposes details such as file paths, full shell
commands, and tool inputs. Those fields improve guard coverage but may contain
sensitive code, paths, commands, or identifiers, so enable them only under an
appropriate data policy.

Claude Code emits logs in batches. For a setup check, perform a tool call, wait
for the export interval, and query `telemetry_coverage`. If no events arrive,
run `claude --debug` and inspect its OpenTelemetry exporter errors.

## Give Claude access to Tailapp MCP

Use absolute values so Claude's spawned stdio process does not depend on its
working directory:

```sh
claude mcp add --transport stdio \
  --scope user \
  --env TAILAPP_HOME="$HOME/.local/share/tailapp" \
  tailapp -- /absolute/path/to/tailapp mcp
claude mcp get tailapp
```

Inside Claude Code, `/mcp` shows connection status. A useful first prompt is:

> Use Tailapp to show telemetry coverage, recent policy findings, and session
> cost for this harness. Explain any unknown coverage before drawing a policy
> conclusion.

## Current bundle fit

`agent-guard` and `session-cost` are shipped examples, not the available set of
Tailapp applications. Users and agents are encouraged to fork, extend, replace,
or supplement them with analytics and policy specific to their environment;
the [authoring guide](../authoring.md) covers installation over CLI and MCP.

`agent-guard` recognizes `claude_code.tool_result` and
`claude_code.tool_decision`. Claude supplies `tool_name` and `success` on tool
results; detailed logging supplies `file_path` or `full_command` for relevant
tools. The reference policy still needs customization for your allowed roots
and operation classes.

`session-cost` recognizes `claude_code.api_request` and its token attributes.
It maps Claude Code's native `cost_usd_micros` field into the example's
`cost_microusd` column. See the
[`session-cost` input model](../../tailapps/session-cost/README.md#input-model).

Official references: [Claude Code monitoring](https://code.claude.com/docs/en/monitoring-usage)
and [Claude Code MCP setup](https://code.claude.com/docs/en/mcp).
