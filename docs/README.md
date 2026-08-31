# Tailapps documentation

Start with the root [README](../README.md#install-and-connect). It is the canonical
build, startup, install, and first-query path. The pages below add detail
without changing that sequence.

## Use Tailapps

- [Harness overview and verification](harnesses/README.md)
- Harness setup: [Claude Code](harnesses/claude-code.md),
  [Codex](harnesses/codex.md), [OpenCode](harnesses/opencode.md), and
  [Pi](harnesses/pi.md)
- Shipped Tailapps: [activity stats](../tailapps/activity-stats/README.md),
  [agent guard](../tailapps/agent-guard/README.md),
  [daily review](../tailapps/daily-review/README.md),
  [session cost](../tailapps/session-cost/README.md), and
  [signal counts](../tailapps/signal-counts/README.md)

## Build Tailapps

- [Author, install, and update a Tailapp](authoring.md)
- [Five-minute walkthrough with the shipped `signal-counts` Tailapp](../tailapps/signal-counts/README.md)
- [Canonical OTLP record shapes](reference/otel-records.md)
- [DDL and JSONata](reference/ddl-jsonata.md)
- [Query SQL](reference/query-sql.md)

The shipped Tailapps are a starting kit, not a fixed catalog. Custom Tailapps use the
same compiler, lifecycle, and runtime. Users and agents can create, validate,
activate, query, update, and delete them through either public interface.

## Operate and integrate

- [CLI reference](reference/cli.md)
- [MCP reference](reference/mcp.md)
- [Runtime metrics](reference/metrics.md)
- [Dependency and vulnerability checks](reference/dependency-security.md)
- [Verified GitHub releases](reference/releases.md)
- [First-time resident setup](reference/first-time-setup.md)

The CLI and MCP adapter are clients of the same resident control service.
Draft changes made through either interface are immediately visible through
the other, but do not affect a live projection until activation.

## Design

- [Architecture](../notes/2026-08-28-tailapp-architecture.md)
- [Initial implementation specification](../notes/2026-08-28-tailapp-initial-implementation.md)
- [Proposed disk session ingestion](../notes/2026-08-28-disk-session-ingestion.md)
