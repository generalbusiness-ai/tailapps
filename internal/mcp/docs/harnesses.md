# Connecting coding-agent harnesses

Telemetry and MCP are separate connections: the harness sends OTLP/HTTP to
the resident receiver, while its MCP client runs `tailapp mcp` against the
same engine home.

## Telemetry

Point the harness's OTLP logs exporter at the resident (default
`http://127.0.0.1:4318/v1/logs`, http/protobuf or JSON). Claude Code,
Codex, and OpenCode each have OTLP telemetry settings; the resident's
actual address is written to `engine.json` in the engine home.

Content gating stays with the harness. The shipped analytics need no
prompt text, response text, or tool content; leaving those disabled keeps
them redacted at the source while activity, cost, and guard analytics
still work. Detail gates (file paths, commands, URLs) widen what the
telemetry carries — enable them only with a data policy that accepts it.

## Verifying the path

After a model request and a tool call: `tailapp_status` shows rising
intake and per-Tailapp frontiers; a query against `activity-stats` or
`agent-guard` shows recognized rows. Intake rising while a query stays
empty means records arrive but a normalizer rejects them —
`tailapp_ineffective` holds the recent rejects for diagnosis.
