# Claude Code

Claude Code can send its native structured log events directly to Tailapp over
OTLP/HTTP. The three examples shipped with this release recognize its
tool-result, tool-decision, and API-request event names.

## Send telemetry

### Persistent settings

For normal use, put the exporter variables in a Claude Code settings file.
Choose one scope:

- `~/.claude/settings.json`: your user settings, applied in every project on
  this machine;
- `.claude/settings.json`: shared project settings, normally committed for the
  team;
- `.claude/settings.local.json`: your settings for one project, kept out of
  version control; or
- managed settings: organization policy delivered by the claude.ai admin
  console, MDM, or `managed-settings.json`. File-based managed settings live at
  `/Library/Application Support/ClaudeCode/managed-settings.json` on macOS,
  `/etc/claude-code/managed-settings.json` on Linux and WSL, and
  `C:\Program Files\ClaudeCode\managed-settings.json` on Windows.

Add this `env` object to the selected file, preserving any other keys already
there. Settings files are strict JSON, and every `env` value is a string:

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "env": {
    "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
    "OTEL_LOGS_EXPORTER": "otlp",
    "OTEL_EXPORTER_OTLP_LOGS_PROTOCOL": "http/protobuf",
    "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT": "http://127.0.0.1:4318/v1/logs"
  }
}
```

Claude Code applies an `env` value over the same variable inherited from your
shell. Between files, managed settings have highest precedence, followed by
command-line `--settings`, project-local, shared-project, and user settings.
Managed OTLP destination settings can also remove conflicting developer-set
per-signal endpoints and protocols. Run `/status` inside Claude Code to confirm
which settings sources loaded, and `claude doctor` to find invalid or rejected
entries.

Run `/status` after saving to confirm the file loaded, then start a new Claude
Code session for a clean exporter lifecycle. Values exported directly by your
shell are read when the next `claude` process starts.

### One-session shell configuration

For a temporary setup or troubleshooting, start Claude Code from a shell with
the same values:

```sh
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_LOGS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_LOGS_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_LOGS_ENDPOINT=http://127.0.0.1:4318/v1/logs
claude
```

Use the signal-specific endpoint exactly as shown, including `/v1/logs`.
Tailapp accepts both `http/protobuf` and `http/json`; it does not accept OTLP
gRPC.

The shipped analytics do not require prompt text, assistant-response text, or
tool content. Leave `OTEL_LOG_USER_PROMPTS`,
`OTEL_LOG_ASSISTANT_RESPONSES`, and `OTEL_LOG_TOOL_CONTENT` unset to keep
that content redacted; prompt and response length analytics still work.
`OTEL_LOG_TOOL_DETAILS=1` additionally exposes tool parameters and inputs,
including paths and full shell commands. The shipped guard deliberately does
not parse those raw structures, so enable detailed logging only for a trusted
custom Tailapp or downstream collector with an appropriate data policy.

Claude Code emits logs in batches. For a setup check, perform a tool call, wait
for the default five-second log export interval, and run:

```sh
./tailapp health
./tailapp metrics --json
./tailapp query \
  --sql "SELECT harness, event_family, SUM(event_count) AS events FROM event_inventory WHERE harness = 'claude-code' GROUP BY harness, event_family ORDER BY event_family" \
  activity-stats
./tailapp query \
  --sql "SELECT harness, capability, state, reason FROM telemetry_coverage WHERE harness = 'claude-code' ORDER BY capability" \
  agent-guard
```

An increase in the metrics intake counters proves transport independently of
whether a shipped Tailapp recognizes the record. Rows in `event_inventory`
prove the activity bundle recognized Claude Code's event family; the coverage
query distinguishes observed policy fields from fields Claude Code did not
provide. If no records arrive, run `claude --debug` and inspect its
OpenTelemetry exporter errors. If intake rises but no rows appear, inspect
`./tailapp ineffective activity-stats` for an adapter-shape mismatch.

## Give Claude access to Tailapp MCP

Use absolute values so Claude's spawned stdio process does not depend on its
working directory:

```sh
claude mcp add --transport stdio \
  --scope user \
  tailapp \
  --env TAILAPP_HOME="$HOME/.local/share/tailapp" \
  -- /absolute/path/to/tailapp mcp
claude mcp get tailapp
```

Inside Claude Code, `/mcp` shows connection status. A useful first prompt is:

> Use Tailapp to show telemetry coverage, recent policy findings, and session
> cost for this harness. Explain any unknown coverage before drawing a policy
> conclusion.

## Current bundle fit

The three shipped Tailapps are examples, not a catalog. Users and agents can
fork, extend, replace, or supplement them; the
[authoring guide](../authoring.md) covers installation over CLI and MCP.

`agent-guard` recognizes `claude_code.tool_result` and
`claude_code.tool_decision`. Current Claude Code records carry a short
`event.name` (`tool_result` or `tool_decision`) and the namespaced name in the
log body; the normalizer uses the namespaced body for this source. Claude
supplies `tool_name` and `success` on tool results. Detailed logging exposes
raw `tool_input` or `tool_parameters`; the shipped normalizer deliberately
does not interpret those sensitive structures, so target coverage remains
unknown unless an adapter promotes a safe `file_path`, `full_command`, or
`target` attribute. The reference policy still needs customization for your
allowed roots and operation classes.

`session-cost` recognizes `claude_code.api_request` and its token attributes.
It maps Claude Code's native `cache_read_tokens` and `cost_usd_micros` fields
into the example's `cached_input_tokens` and `cost_microusd` columns. See the
[`session-cost` input model](../../tailapps/session-cost/README.md#input-model).

`activity-stats` consumes the same tool and API-request families plus
`claude_code.assistant_response` length metadata. It retains the reported
length only, never response content; see its
[`input and privacy model`](../../tailapps/activity-stats/README.md#input-and-privacy-model).

The checked-in compatibility fixture is a structurally representative,
scrubbed capture from Claude Code 2.1.250. Vendor fields may change; after an
upgrade, use `ineffective` and `telemetry_coverage` to check the live shape.

Official references: [Claude Code monitoring](https://code.claude.com/docs/en/monitoring-usage)
and [Claude Code MCP setup](https://code.claude.com/docs/en/mcp).
