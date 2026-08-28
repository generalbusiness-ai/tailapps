# Tailapp documentation

Start with the root [README](../README.md#run-locally). It is the canonical
build, startup, install, and first-query path. The pages below add detail
without changing that sequence.

## Use Tailapp

- [Harness overview and verification](harnesses/README.md)
- Harness setup: [Claude Code](harnesses/claude-code.md),
  [Codex](harnesses/codex.md), [OpenCode](harnesses/opencode.md), and
  [Pi](harnesses/pi.md)
- Shipped analytics: [activity stats](../tailapps/activity-stats/README.md),
  [agent guard](../tailapps/agent-guard/README.md), and
  [session cost](../tailapps/session-cost/README.md)

## Build Tailapps

- [Author, install, and update a Tailapp](authoring.md)
- [Five-minute `signal-counts` example](../examples/signal-counts/README.md)
- [Canonical OTLP record shapes](reference/otel-records.md)
- [DDL and JSONata](reference/ddl-jsonata.md)
- [Query SQL](reference/query-sql.md)

The shipped analytics are examples, not a catalog. Custom Tailapps use the
same compiler, lifecycle, and runtime. Users and agents can create, validate,
activate, query, update, and delete them through either public interface.

## Operate and integrate

- [CLI reference](reference/cli.md)
- [MCP reference](reference/mcp.md)
- [Runtime metrics](reference/metrics.md)

The CLI and MCP adapter are clients of the same resident control service.
Draft changes made through either interface are immediately visible through
the other, but do not affect a live projection until activation.

## Design

- [Architecture](../notes/2026-08-28-tailapp-architecture.md)
- [Initial implementation specification](../notes/2026-08-28-tailapp-initial-implementation.md)
- [Proposed disk session ingestion](../notes/2026-08-28-disk-session-ingestion.md)
