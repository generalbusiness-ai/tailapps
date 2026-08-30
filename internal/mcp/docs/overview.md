# Tailapp

Tailapp turns local coding-agent OTLP telemetry into queryable SQLite
analytics. One resident engine hosts any number of Tailapps: small
declarative applications, each with its own continuously materialized
projection. Observation is detective, never inline prevention — nothing
here can block a tool call or stop an agent.

This server: version {{version}}{{source_line}}

## Read first

- `tailapps_list` — what is installed.
- `tailapp_query` — read-only SQL over one Tailapp's exported tables.
- `tailapp_status` — engine readiness and projection frontiers, the first
  stop when telemetry seems missing.
- `tailapp_ineffective` — recent records a Tailapp's normalizer rejected,
  for adapter-shape diagnosis.

## Changing things

A Tailapp is defined by a small source set: one `application.sql` (events,
tables, programs, exports) and JSONata programs under `folds/`. The draft
loop is `tailapp_create` → `tailapp_put_element` → `tailapp_validate` →
`tailapp_activate`; `tailapp_install` does all of it in one validated
request from a built-in bundle or a complete source map. Every mutating
tool takes an `idempotency_key`: an identical retry replays the original
outcome. `tailapp_delete` and reset-mode activation discard materialized
state and are the two destructive operations.

See `tailapp://docs/authoring` for the source format,
`tailapp://docs/query-sql` for the query subset, and
`tailapp://docs/tools/NAME` for any tool.

## Data sensitivity

Query, schema, and ineffective results derive from local telemetry and may
contain session identifiers and file paths. Treat them as sensitive, and
prefer aggregate queries when sharing output.
