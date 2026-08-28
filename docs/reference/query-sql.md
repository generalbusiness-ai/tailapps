# Query SQL reference

CLI `tailapp query` and MCP `tailapp_query` run bounded read-only SQLite SQL
against an active projection snapshot. Querying never invokes JSONata and
cannot change Tailapp state.

## Visible relations

The primary Tailapp exposes all of its declared tables and application views.
Tailapp platform metadata, SQLite catalog relations, and internal export views
are denied.

Another Tailapp is visible only when the request mounts it under a SQL
identifier alias, and only its declared exports are readable:

```sh
tailapp query \
  --mount cost=session-cost \
  --sql 'SELECT p.session_id, c.input_tokens FROM session_progress p JOIN cost.session_cost c ON c.harness = p.harness AND c.session_id = p.session_id ORDER BY p.session_id' \
  agent-guard
```

Use mounted relations only in explicit `FROM alias.export` or
`JOIN alias.export` clauses. Give each relation a table alias and qualify its
columns through that alias. Comma-listed mounted relations are outside the
supported syntax.

Mounts are request-local. They do not create fold dependencies or a persistent
Tailapp graph. A mounted projection must be healthy and at the same interpreted
position as the primary projection, which gives the query one aligned
delivery snapshot.

## Allowed operations

The prepared statement must be read-only and the input must contain exactly
one statement. The SQLite authorizer permits selection and reads only from the
relations above. The only SQL functions admitted are:

```text
avg coalesce count ifnull max min sum total
```

Pragmas, attach/detach, schema changes, extension loading, and data mutation
are denied even if hidden inside otherwise valid SQL. The query sandbox uses a
read-only connection, defensive mode, untrusted schema, and disabled extension
loading.

## Parameters

Use positional `?` placeholders. Up to 64 parameters are accepted. MCP
parameters are JSON scalars; CLI `--param` values are individually parsed as
JSON. Supported bound values are null, string, boolean, integer, finite real,
and binary values supplied by an API client.

```sh
tailapp query \
  --sql 'SELECT finding_id, summary FROM policy_findings WHERE harness = ? ORDER BY source_position DESC' \
  --param '"claude-code"' \
  agent-guard
```

## Snapshot preconditions

Pass `expected_revision`/`--expected-revision` to require one active source
revision and `expected_position`/`--expected-position` to require one
interpreted frontier. A mismatch returns `frontier_changed` rather than
silently answering from a newer snapshot.

Every result identifies:

- primary Tailapp and active revision;
- current delivery head and interpreted position;
- mounted aliases, revisions, export-contract digests, and positions;
- whether the projection is complete;
- columns, rows, and truncation state.

The delivery head can be ahead of the interpreted position while accepted work
is pending. Use `expected_position` when a follow-up query must observe the
same materialization point.

## Limits and representation

- SQL text: 16 KiB
- positional parameters: 64
- row limit: default 256, maximum 1000
- encoded result rows: 1 MiB
- wall deadline: 5 seconds
- fixed SQLite progress budget: 2,048 checks, each covering 1,000 VM
  instructions plus statement-boundary checks

The sandbox also caps SQLite expression depth, columns, VM operations,
function arguments, attached databases, and individual SQLite values. A fixed
budget exhaustion returns `query_budget_exceeded`; the wall deadline is a
secondary safety net.

BOOLEAN columns return JSON booleans and JSON columns return decoded JSON.
BLOB values return `{"bytes_base64":"..."}`. Integers outside the exact JSON
safe range return `{"integer_decimal":"..."}` rather than losing precision.
`truncated = true` means the row or byte limit stopped the result; it is not a
complete answer.
