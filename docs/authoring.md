# Author and install a Tailapp

A Tailapp is a versioned source set, not a plugin loaded into the engine
process. Its DDL declares a private normalized event, materialized tables,
bounded reads, JSONata programs, and explicit query exports. The resident
compiles the complete source set before it can become active.

The lifecycle is:

```text
create draft -> put/remove elements -> validate exact revision -> activate -> query
```

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

## Install with CLI

Start from an empty draft. The examples below use `jq` only to extract the
revision returned by each JSON response:

```sh
revision=$(tailapp apps create \
  --idempotency-key event-counts-create-v1 \
  event-counts | jq -r .draft_revision)

revision=$(tailapp apps put \
  --expected "$revision" \
  --idempotency-key event-counts-put-application-v1 \
  event-counts application.sql event-counts/application.sql |
  jq -r .draft_revision)

revision=$(tailapp apps put \
  --expected "$revision" \
  --idempotency-key event-counts-put-normalize-v1 \
  event-counts folds/normalize.jsonata event-counts/folds/normalize.jsonata |
  jq -r .draft_revision)

revision=$(tailapp apps put \
  --expected "$revision" \
  --idempotency-key event-counts-put-count-v1 \
  event-counts folds/count.jsonata event-counts/folds/count.jsonata |
  jq -r .draft_revision)

tailapp apps validate --expected "$revision" event-counts
tailapp apps activate \
  --expected "$revision" \
  --mode reset \
  --ack-reset \
  --idempotency-key event-counts-activate-v1 \
  event-counts
```

There is no custom directory-import command in v1. `apps create --bundle`
selects only a source set embedded in the Tailapp binary. A custom "bundle"
is installed by creating an empty app and putting every source element as
shown above.

## Install with MCP

An agent follows the same sequence:

1. Call `tailapp_create` with a name and idempotency key, omitting `bundle`.
2. For each source file, call `tailapp_put_element` with the latest
   `expected_revision`, a new idempotency key, and base64-encoded content.
3. Call `tailapp_validate` with the final exact revision and inspect the
   compiled schema, writers, reads, exports, and contract digests.
4. Call `tailapp_activate`. Use `reset` with `acknowledge_reset = true` for the
   first activation.
5. Call `tailapp_schema` and `tailapp_query` to verify live behavior.

The complete schemas are in the [MCP reference](reference/mcp.md). Agents
should keep source files in a normal repository as the human-reviewable source
of truth; the Tailapp registry is deployment state, not an authoring workspace.

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
