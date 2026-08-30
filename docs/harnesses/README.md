# Harness setup

Complete [Install and connect](../../README.md#install-and-connect) first. Keep
the resident running, use an absolute binary path in harness configuration, and
use the same `TAILAPP_HOME` everywhere.

A harness has two independent connections:

1. It sends OTLP/HTTP telemetry to the `tailapp` engine's loopback receiver.
2. Its MCP client starts `tailapp mcp` over stdio. That adapter connects to the
   resident through `$TAILAPP_HOME/engine.sock`.

Telemetry does not travel through MCP, and `tailapp mcp` does not start the
resident.

## Choose a harness

| Harness | Telemetry | MCP |
| --- | --- | --- |
| [Claude Code](claude-code.md) | Native OTLP/HTTP logs | Native client |
| [Codex](codex.md) | Native OTLP/HTTP logs | Native client |
| [OpenCode](opencode.md) | Third-party plugin | Native client |
| [Pi](pi.md) | Third-party extension | Third-party adapter |

The OpenCode and Pi producers are not uniform wire contracts. Follow the named
compatibility profile, then use ineffective-record samples to inspect any
different producer's shape.

## Verify transport, then interpretation

After one model response and tool call:

```sh
export TAILAPP_HOME="$HOME/.local/share/tailapp"
./tailapp health
./tailapp metrics --json
./tailapp query \
  --sql 'SELECT harness, event_family, SUM(event_count) AS events FROM event_inventory GROUP BY harness, event_family ORDER BY harness, event_family' \
  activity-stats
./tailapp query \
  --sql 'SELECT harness, capability, state, reason FROM telemetry_coverage ORDER BY harness, capability' \
  agent-guard
./tailapp query \
  --sql 'SELECT harness, session_id, input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens, cost_microusd FROM session_cost ORDER BY harness, session_id' \
  session-cost
```

Interpret the checks in order:

- A rising `intake.records_total` proves OTLP transport, even if no Tailapp
  recognizes the record.
- Rows prove that the named Tailapp recognized useful fields. An empty result
  is not evidence of zero activity.
- An `unknown` coverage state means the guard recognized an event but lacked a
  required field.
- A rising query-result `ineffective_records` count means that Tailapp rejected
  some canonical records. Run `./tailapp ineffective APP` for its bounded,
  memory-only sample.

Read each shipped Tailapp's model before treating results as policy or billing
evidence: [activity stats](../../tailapps/activity-stats/README.md),
[agent guard](../../tailapps/agent-guard/README.md),
[daily review](../../tailapps/daily-review/README.md),
[session cost](../../tailapps/session-cost/README.md), and
[signal counts](../../tailapps/signal-counts/README.md).

## Security

The Tailapps v1 engine is a local, same-OS-user service. Its receiver has no
TLS or authentication, MCP can mutate Tailapp definitions, and telemetry or
projections may contain sensitive content. Do not expose the receiver beyond
loopback or give untrusted processes access to the MCP command.

The shipped guard is detective, not preventive. Export batching adds latency,
and a silent or terminated harness cannot emit the event needed to diagnose
it. Keep harness-native permission and sandbox controls in place.

The five shipped Tailapps are a starting kit. Agents and operators can
[author and install their own](../authoring.md), including a normalizer for a
different adapter vocabulary.
