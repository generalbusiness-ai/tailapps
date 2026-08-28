# Tailapp documentation

## Run and connect

- [Harness setup overview](harnesses/README.md)
- [Claude Code](harnesses/claude-code.md)
- [Codex](harnesses/codex.md)
- [OpenCode](harnesses/opencode.md)
- [Pi](harnesses/pi.md)

## Extend

- [Author and install a Tailapp](authoring.md)
- [DDL and JSONata reference](reference/ddl-jsonata.md)
- [Query SQL reference](reference/query-sql.md)

Tailapp is not limited to the two examples compiled into this release. A
custom Tailapp is an ordinary source set containing `application.sql` and one
or more `folds/*.jsonata` programs. Users and agents are encouraged to create
new applications, fork or extend the examples, substitute different policies,
and install ongoing revisions. They can create, edit, validate, activate,
query, and delete every custom Tailapp through the public CLI or MCP lifecycle.

## Interfaces

- [CLI reference](reference/cli.md)
- [MCP reference](reference/mcp.md)

The CLI and MCP server are clients of the same resident control service. Draft
changes made through either interface are immediately visible through the
other, but never alter live projections until activation.

## Examples shipped with this release

- [`agent-guard`](../tailapps/agent-guard/README.md)
- [`session-cost`](../tailapps/session-cost/README.md)

These are useful initial analytics and complete authoring examples, not a
catalog or preferred limit. Their source files use the same DDL/JSONata and
lifecycle as custom Tailapps; they have no privileged execution path.

## Design

- [Architecture](../notes/2026-08-28-tailapp-architecture.md)
- [Initial implementation](../notes/2026-08-28-tailapp-initial-implementation.md)
