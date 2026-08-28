# Author and install a Tailapp

A Tailapp is a versioned source set, not a plugin loaded into the engine
process. Its DDL declares a private normalized event, materialized tables,
bounded reads, JSONata programs, and explicit query exports. The resident
compiles the complete source set before it can become active.

The first-install lifecycle is:

```text
complete source set -> validate -> create -> first activation -> query
```

CLI `apps install` and MCP `tailapp_install` perform that lifecycle in one
idempotent request. Existing applications use the deliberately explicit update
lifecycle: `get draft -> put/remove elements -> validate exact revision ->
activate`.

Draft editing is optimistic: every element mutation names the current draft
revision and returns a new one. Mutation idempotency keys are durably bound to
one exact request. Activation happens at an inbox delivery boundary, so a
single accepted OTLP record is never interpreted partly by two revisions.

## Minimal example

Create this directory outside `$TAILAPP_HOME`:

```text
event-counts/
├── application.sql
└── folds/
    ├── count.jsonata
    └── normalize.jsonata
```

`application.sql`:

```sql
CREATE EVENT otel_event (
  source TEXT NOT NULL,
  event_name TEXT NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE event_counts (
  source TEXT NOT NULL,
  event_name TEXT NOT NULL,
  event_count INTEGER NOT NULL,
  last_source_position INTEGER NOT NULL,
  PRIMARY KEY (source, event_name)
);

CREATE NORMALIZER normalize_log ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;

CREATE FOLD count_log ON otel_event
READ prior OPTIONAL ONE AS
  SELECT source, event_name, event_count, last_source_position
  FROM event_counts
  WHERE source = :event.source AND event_name = :event.event_name
USING 'folds/count.jsonata'
WRITES event_counts;

CREATE EXPORT event_counts AS
  SELECT source, event_name, event_count, last_source_position
  FROM event_counts;
```

`folds/normalize.jsonata`:

```jsonata
(
  event.signal = "log" ? {
    "decision": "effective",
    "facts": [],
    "events": {"otel_event": [{
      "source": event.source,
      "event_name": event.name,
      "source_position": meta.position
    }]},
    "tables": {}
  } : {
    "decision": "ineffective",
    "facts": [],
    "events": {},
    "tables": {}
  }
)
```

`folds/count.jsonata`:

```jsonata
(
  $old := rows.prior;
  {
    "decision": "effective",
    "facts": [],
    "tables": {"event_counts": {"upsert": [{
      "source": event.source,
      "event_name": event.event_name,
      "event_count": $old ? $old.event_count + 1 : 1,
      "last_source_position": event.source_position
    }]}}
  }
)
```

The normalizer sees every captured OTLP record and marks non-log records
ineffective. Effective log records produce a transaction-local `otel_event`.
The analytic fold reads its own prior row and upserts the next count. Neither
the source record nor the private event becomes queryable storage.

## Install with CLI in one request

Install and first-activate the complete directory:

```sh
tailapp apps install \
  --idempotency-key event-counts-install-v1 \
  event-counts event-counts
```

The command reads `application.sql` and every `folds/*.jsonata` file, validates
the complete source set before creating anything, and activates a fresh
projection at the current delivery boundary. Files such as `README.md` remain
author documentation and are not installed as executable source.

The result contains the installed app, compiled-profile identity and contract
digests, and the active frontier; pass `--full` when reviewing the complete
compiled profile. Installation is create-only: it refuses an existing name
rather than silently replacing source or resetting materialized state.

## Install with MCP in one request

An agent calls `tailapp_install` with:

- the new Tailapp `name`;
- `sources`, a map containing `application.sql` and every referenced
  `folds/*.jsonata` file as base64 values; and
- one stable `idempotency_key` for the complete request.

The returned app, compiled profile, and frontier provide immediate review and
query context. The same tool can install a shipped example by supplying
`bundle` instead of `sources`, but those examples have no privileged runtime
path.

The complete schema is in the [MCP reference](reference/mcp.md). Agents should
keep source files in a normal repository as the human-reviewable source of
truth; the Tailapp registry is deployment state, not an authoring workspace.

## Lower-level lifecycle

`apps create`, `apps put`, `apps validate`, and `apps activate`—and their MCP
counterparts—remain available for reviewed, incremental control. Every element
mutation names the prior draft revision and returns the next revision. This is
the appropriate path for updating an existing Tailapp, because an activation
must explicitly choose compatible continuation or acknowledged reset.

## Update safely

Read the latest draft with `apps get` or `tailapp_get`, then make each edit
against the revision just returned. Validate before activation.

Use `continue` only when every existing writable table retains its exact stored
shape. A continuation may add tables and replace programs, views, indexes, and
exports while preserving existing rows. Removing or changing an existing
table, column, constraint, primary key, or unique key requires `reset`, which
discards that Tailapp's prior materialized state and requires explicit
acknowledgement.

Draft edits are not live. A failed validation leaves the active revision and
projection untouched. A projection that fails deterministically on an input
records a local gap and detaches from later delivery; inspect
`tailapp_status`, fix the source, and reset-activate because Tailapp v1 has no
retained event log to replay the missing data.

## Share dependencies

Tailapps cannot consume another Tailapp inside a fold. This prevents a
dataflow graph and keeps execution flat. Cross-Tailapp composition is
query-time only: declare an explicit export in the provider, mount the provider
under an alias in a query, and join `alias.export` from the primary Tailapp.
See the [query SQL reference](reference/query-sql.md).

## Delete

`apps delete` or `tailapp_delete` removes the definition and detaches only that
projection. It does not delete another Tailapp or affect its state. Deletion is
idempotent only when retried with the same exact key and request.
