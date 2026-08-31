# VS Code Copilot Chat

VS Code's built-in GitHub Copilot Chat extension can export local OpenTelemetry
to Tailapp over OTLP/HTTP. The included bundles recognize its `copilot-chat`
default service identity and the optional `vscode-copilot-chat` identity below,
using one `vscode-copilot-chat` harness label for both.

## Send telemetry

Open **Preferences: Open User Settings (JSON)** and add these settings,
preserving any existing settings:

```json
{
  "github.copilot.chat.otel.enabled": true,
  "github.copilot.chat.otel.exporterType": "otlp-http",
  "github.copilot.chat.otel.otlpEndpoint": "http://127.0.0.1:4318",
  "github.copilot.chat.otel.captureContent": false,
  "github.copilot.chat.otel.serviceName": "vscode-copilot-chat"
}
```

The endpoint is the receiver base URL; VS Code sends the OTLP signal paths.
`serviceName` is optional: use it to make Copilot’s source name unambiguous;
`OTEL_SERVICE_NAME` takes precedence when it is set in VS Code’s environment.
Restart VS Code after changing telemetry settings, then make one Copilot Chat
request and one agent tool call. Confirm delivery and interpretation with:

```sh
./tailapp health
./tailapp query \
  --sql "SELECT source, signal, event_name, event_count FROM signal_counts WHERE source = 'vscode-copilot-chat' ORDER BY signal, event_name" \
  signal-counts
./tailapp query \
  --sql "SELECT harness, event_family, tool_bucket, SUM(event_count) AS events FROM event_inventory WHERE harness = 'vscode-copilot-chat' GROUP BY harness, event_family, tool_bucket ORDER BY event_family, tool_bucket" \
  activity-stats
```

The first query proves VS Code reached the local receiver even when a bundled
Tailapp does not yet recognize a newer event. The second reports the currently
recognized inference and tool families.

## What the included Tailapps retain

Copilot emits the same interaction through logs, traces, and metrics. The
bundles use the log record `gen_ai.client.inference.operation.details` as the
single source for token totals, avoiding duplicate token counts from its trace
and metric companions. It maps `gen_ai.usage.input_tokens`,
`gen_ai.usage.output_tokens`, and cache-read tokens when supplied. Copilot's
telemetry does not report a currency cost, so `session-cost.cost_microusd`
remains zero rather than estimating a price.

`agent-guard` recognizes `copilot_chat.tool.call`, with
`gen_ai.tool.name`, `duration_ms`, and `success`. Copilot tool names fall into
the generic `other` tool bucket. It records target coverage as unknown unless
a producer supplies a safe `target`, `file_path`, or `full_command` attribute;
Copilot’s full `gen_ai.tool.arguments` are never promoted into policy evidence.

For inference logs that lack a conversation identifier, the bundles use the
resource `session.id`. That identifies the VS Code instrumentation session,
not necessarily one chat conversation, so do not treat its aggregate row as a
per-chat-session ledger. Spans may contain a more precise
`gen_ai.conversation.id`; the shipped bundles do not use them for token totals
because that would duplicate the logs. Only those two log families are
interpreted; their trace and metric companions remain visible through
`signal-counts` for diagnosis but do not add to bundle results.

The content setting above asks VS Code not to include prompt, response, and
tool payloads. The included Tailapps do not promote such raw fields into their
materialized rows; they retain only the structured fields described here.

The checked-in compatibility fixture is a structurally representative,
scrubbed local capture. After updating VS Code or Copilot Chat, use
`tailapp ineffective APP` to inspect a bounded sample before changing a
normalizer.

Official reference: [VS Code monitoring agents](https://code.visualstudio.com/docs/agents/guides/monitoring-agents).
