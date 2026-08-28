# Harness setup

Tailapp has two independent connections to an agent harness:

1. The harness sends OTLP/HTTP telemetry to the resident Tailapp engine.
2. The harness starts `tailapp mcp` over stdio so its agent can inspect and
   manage Tailapps.

Telemetry does not travel through MCP. The MCP process connects to the already
running resident through `$TAILAPP_HOME/engine.sock`; it must inherit the same
`TAILAPP_HOME` as the resident.

## Start Tailapp

Build once and keep the absolute binary path for harness configuration:

```sh
go build -o tailapp ./cmd/tailapp
export TAILAPP_BIN="$PWD/tailapp"
export TAILAPP_HOME="$HOME/.local/share/tailapp"
"$TAILAPP_BIN" init
"$TAILAPP_BIN" serve --otlp-http 127.0.0.1:4318
```

Keep `serve` running. Tailapp requires an explicit loopback IP and refuses
`localhost`. The actual URL is recorded in `$TAILAPP_HOME/engine.json`.

In another terminal, export the same variables and install each shipped
example in one validated, first-activated request:

```sh
export TAILAPP_HOME="$HOME/.local/share/tailapp"
"$TAILAPP_BIN" apps install --bundle activity-stats \
  --idempotency-key setup-install-activity-stats-v1 activity-stats
"$TAILAPP_BIN" apps install --bundle agent-guard \
  --idempotency-key setup-install-agent-guard-v1 agent-guard
"$TAILAPP_BIN" apps install --bundle session-cost \
  --idempotency-key setup-install-session-cost-v1 session-cost
```

These commands install the three examples shipped with this release. They are a
starting kit, not a fixed application catalog. Users and agents are encouraged
to create additional Tailapps, fork or extend an example, or replace either
example with policy- and harness-specific analytics. Custom Tailapps use the
same CLI and MCP lifecycle; see [Author and install a Tailapp](../authoring.md).

The keys above are stable retry keys for these exact operations. If an app
already exists, inspect it with `tailapp apps get APP`; install never replaces
an existing Tailapp.

## Choose a harness

- [Claude Code](claude-code.md): native OTLP/HTTP logs and native MCP client
- [Codex](codex.md): native OTLP/HTTP logs and native MCP client
- [OpenCode](opencode.md): native MCP client; telemetry adapter required
- [Pi](pi.md): third-party extensions required for MCP and telemetry

## Verify data

After the harness has completed at least one tool call and model request, check
that the resident is healthy and its inbox has drained:

```sh
"$TAILAPP_BIN" health
"$TAILAPP_BIN" metrics --json
"$TAILAPP_BIN" ineffective agent-guard
"$TAILAPP_BIN" query \
  --sql 'SELECT harness, capability, state, reason FROM telemetry_coverage ORDER BY harness, capability' \
  agent-guard
"$TAILAPP_BIN" query \
  --sql 'SELECT harness, session_id, input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens, cost_microusd FROM session_cost ORDER BY harness, session_id' \
  session-cost
"$TAILAPP_BIN" query \
  --sql 'SELECT harness, event_family, SUM(event_count) AS events, SUM(duration_unknown_count) AS duration_unknown FROM event_inventory GROUP BY harness, event_family ORDER BY harness, event_family' \
  activity-stats
```

An increase in `intake.records_total` proves OTLP delivery independently of
MCP. Query results include the projection's durable `ineffective_records`
count, and `ineffective` exposes up to 16 recent rejected records from resident
memory for shape diagnosis. An empty table means no recognized event reached
that bundle. An `unknown` coverage row means the event was recognized but
lacked a field needed by the guard. See the
[`activity-stats`](../../tailapps/activity-stats/README.md),
[`agent-guard`](../../tailapps/agent-guard/README.md), and
[`session-cost`](../../tailapps/session-cost/README.md) models before treating
results as policy or billing evidence.

## Security and operating limits

Tailapp v1 is a local, same-OS-user service. Its OTLP receiver has no TLS or
authentication, the MCP interface can mutate Tailapp definitions, and
projections may contain commands, paths, identities, or prompts. Do not expose
the receiver beyond loopback or give untrusted processes access to the MCP
command.

The bundled guard is detective, not preventive. Export batching adds latency,
and a silent or terminated harness cannot emit the event needed to diagnose
it. Use harness-native permission and sandbox controls for inline enforcement.

Once connected, an agent can [author and install its own
Tailapps](../authoring.md) through the same MCP interface. For OpenCode and Pi,
a custom normalizer is also the natural place to support an adapter's native
event vocabulary instead of translating it to one of the shipped examples.
