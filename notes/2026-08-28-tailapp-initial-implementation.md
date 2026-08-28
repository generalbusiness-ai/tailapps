---
date: 2026-08-28
status: initial implementation specification
companion: notes/2026-08-28-tailapp-architecture.md
rests_on:
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:a02e8da110b93948746edf9adf13e6dd00633271
---

# Tailapp initial implementation specification

This specification turns the companion architecture into the first buildable
local product. It defines observable contracts and acceptance tests; package
names and private Go APIs may change without changing the design.

## 1. Deliverable

Build one `tailapp` binary that can:

1. initialize a private local data directory;
2. run a resident engine with an OTLP/HTTP receiver and local control socket;
3. receive OTel logs, spans, and metric points through a bounded durable inbox;
4. create and edit tailapp source elements through CLI or MCP;
5. compile, validate, and atomically activate or reset a tailapp revision;
6. keep active projections caught up as new telemetry arrives;
7. expose private schemas plus explicit read-only relation exports for bounded
   multi-namespace SQL queries; and
8. recover unconsumed inbox records and durable materialized state after a
   restart without retaining consumed telemetry.

The implementation is complete when the end-to-end acceptance tests in
section 15 pass. It is not required to connect to Bedrock, LiteLLM, or a
remote collector.

## 2. Technology profile

- **Language:** Go, using the version selected by `go.mod` and CI.
- **SQLite:** `ncruces/go-sqlite3`, pinned exactly, through `database/sql`.
- **JSONata:** `jsonata-go/jsonata/v206`, pinned exactly and wrapped by a
  Tailapp profile.
- **OTLP:** official OpenTelemetry protobuf definitions; no collector sidecar.
- **MCP:** JSON-RPC over stdio using a maintained Go MCP library if its
  dependency surface is acceptable, otherwise a small protocol adapter.
- **Control transport:** HTTP-over-Unix-domain-socket on macOS and Linux.
- **Serialization:** canonical JSON at the inbox and evaluator boundaries.

The JSONata runtime, SQLite version and controls, DDL grammar, numeric rules,
canonicalization rules, and all deterministic limits form a runtime profile
identifier. A revision digest includes that identifier so an engine upgrade
cannot silently reinterpret an existing active projection.

## 3. Repository layout

Start with this package shape:

```text
cmd/tailapp/                 command dispatch and process entrypoint
internal/control/            local application service and socket transport
internal/engine/             lifecycle, scheduling, and health
internal/ingest/             OTLP/HTTP receiver and canonicalization
internal/inbox/              bounded delivery queue and consumer obligations
internal/definition/         drafts, revisions, source registry, compiler
internal/projection/         flat event waves and SQLite materialization
internal/query/              authorizer-limited read-only SQL
internal/mcp/                MCP stdio adapter
internal/cli/                CLI commands and JSON rendering
internal/profile/            pinned DDL/JSONata language profile
tailapps/tool-activity/      sample rollup and normalized tool-use tables
tailapps/session-cost/       independent sample with a query export
testdata/otlp/               deterministic OTLP request fixtures
notes/                       architecture and implementation notes
```

Packages depend inward on interfaces owned by their consumer. Protocol
adapters do not contain business rules. The engine service is directly
testable without starting a subprocess.

## 4. Process and storage layout

`TAILAPP_HOME` overrides the state root. Otherwise use the operating system's
per-user application-data directory and append `tailapp`. Create the root and
all private subdirectories with owner-only permissions where the platform
supports them.

```text
TAILAPP_HOME/
  control.sqlite             inbox, registry, revision and activation state
  control.sqlite-wal
  engine.sock                local control socket while resident is running
  engine.json                PID, start time, OTLP address, profile, version
  projections/
    <escaped-name>/
      state.sqlite           durable materialized state
      state.sqlite-wal
  tmp/                       scratch databases used by compilation/reset
```

Only the resident opens writable databases. It takes a process lock before
opening `control.sqlite`; a second resident exits with a typed
`engine_already_running` error. CLI and MCP connect through `engine.sock`.

The first engine binds OTLP to `127.0.0.1:4318` by default. A loopback override
is allowed. A non-loopback address is refused in this version.

## 5. Control database

The control database uses WAL, foreign keys, defensive mode, disabled trusted
schema behavior, and disabled extension loading. Its logical relations are:

### `inbox_events`

| column | contract |
|---|---|
| `position` | `INTEGER PRIMARY KEY`, global delivery order |
| `event_id` | unique local delivery ID derived from the position |
| `signal` | `log`, `span`, or `metric` |
| `name` | best stable signal name, possibly empty |
| `source` | service/harness name derived from resource attributes |
| `time_unix_nano` | decimal text; never coerced through JSON float |
| `observed_unix_nano` | decimal text when supplied |
| `trace_id` / `span_id` | lowercase hexadecimal when supplied |
| `content_digest` | SHA-256 of canonical signal content |
| `record_json` | canonical complete signal record, deleted after consumption |
| `received_at` | engine wall time for operations only, not fold semantics |

The queue also has `inbox_obligations(position, tailapp, revision, state,
error_code)`. The active consumer set is captured in the same transaction that
inserts an event. A record is deleted when every captured obligation is
`consumed`, `skipped`, or `detached`. Installing a tailapp does not add an
obligation to already accepted records.

Indexes cover outstanding obligations and delivery order. Correlation fields
are not indexed for user queries because the inbox is not an analytic surface.
The engine never uses `received_at` as a fold input.

The inbox has hard byte and record caps. A deletion transaction runs after
consumer completion; ordinary successful operation therefore keeps only a
small working set. Consumed event content is not retained in any control table
or operational log.

### Definition relations

- `tailapps(name, draft_revision, active_revision, created_at, updated_at)`
- `draft_elements(tailapp, path, content, digest)`
- `revisions(digest, tailapp, runtime_profile, source_json, diagnostics_json)`
- `exports(revision, name, query_sql, contract_json, contract_digest)`
- `activations(tailapp, revision, schema_digest, mode, boundary_position,
  activated_at, retired_at)`
- `projection_status(tailapp, revision, interpreted_position, complete,
  gap_position, gap_reason)`

Draft mutation and revision advancement are one transaction. Immutable
revision rows are insert-only. Activation is a compare-and-swap against the
expected draft, old active revision, and a delivery boundary.

Operational timestamps support diagnostics but never enter JSONata input or
revision identity.

## 6. OTLP/HTTP ingestion

Implement:

- `POST /v1/logs`
- `POST /v1/traces`
- `POST /v1/metrics`

Accept `application/x-protobuf` and `application/json`. Decode with the official
OTLP messages and `protojson`; reject other content types. Support optional
gzip content encoding with a decompressed-size limit.

### Transaction and ordering

One HTTP request is one inbox transaction. Walk resource groups, scope groups,
and records in their encoded order. Assign consecutive positions, capture the
active consumer obligations, and commit all records. Then return the normal
empty OTLP success response. If decoding, validation, limits,
canonicalization, or persistence fails, roll back the whole request and return
a non-success status; partial acceptance is not in the first implementation.

After commit, notify projection workers. A successful response means Tailapp
has durably accepted responsibility for delivery, not that it will retain the
event. A slow consumer uses bounded inbox capacity. A gapped consumer is
detached so it cannot turn the temporary queue into a history store. When the
inbox is full, return a retryable backpressure response.

OTLP can deliver at least once. Tailapp does not infer equality as identity and
does not deduplicate records. `content_digest` and native correlation IDs are
available to application folds.

### Canonical record

Convert protobuf values into canonical JSON with sorted object keys, finite
numbers, base64 bytes, decimal-string nanosecond timestamps, lowercase hex
trace/span IDs, and explicit nulls only where the OTel value is null. Preserve:

- resource attributes and schema URL;
- instrumentation scope name, version, attributes, and schema URL;
- all signal attributes;
- log body, severity, flags, trace/span IDs, and timestamps;
- span name, kind, parent, times, status, events, links, flags, and trace state;
- metric identity, temporality and monotonicity, point type, point attributes,
  values or buckets, exemplars, flags, and times.

Unknown protobuf fields are not available through ordinary generated decoding;
the engine records its own version and the runtime profile so this limitation
is explicit.

### Receiver limits

Initial defaults, configurable only in machine-local engine configuration:

- compressed request: 4 MiB;
- decompressed request: 16 MiB;
- records per request: 10,000;
- one canonical record: 256 KiB;
- concurrent OTLP requests: 16;
- inbox records: 100,000;
- inbox canonical bytes: 256 MiB;
- decode and commit deadline: 10 seconds.

Limit errors include a stable code and do not include telemetry bodies.

## 7. Tailapp source format

A tailapp source set contains exactly one `application.sql` and one or more
files matching `folds/*.jsonata`. Paths are slash-separated relative paths,
must be normalized UTF-8, and cannot contain empty, dot, or parent segments.
Each element is at most 64 KiB; the complete source set is at most 512 KiB.

Tailapp names match `[a-z][a-z0-9-]{0,62}`. Fold, event, table, view, export,
query-alias, read, and column identifiers match
`[A-Za-z_][A-Za-z0-9_]*` in the first profile.

There is no required manifest. The registry supplies the name; the revision
digest is SHA-256 over:

1. the runtime profile identifier;
2. every normalized path in lexical order; and
3. the exact bytes of each element.

Compilation also produces an export-contract digest over normalized exported
relation definitions and schemas. Query responses name that digest so callers
can detect a changed query-time contract without creating a dependency between
tailapp definitions.

## 8. Source record and event DDL

### Fixed `otlp_record` source

The engine presents every inbox delivery to each tailapp normalizer as a
built-in `otlp_record`. It has the fixed fields `id`, `signal`, `name`,
`source`, `time_unix_nano`, `observed_unix_nano`, `trace_id`, `span_id`,
`content_digest`, and `record`. The last field is decoded structured JSON, not
a JSON string. This source is part of the runtime profile and is not declared
or redefined by a tailapp.

The normalizer evaluator input is:

```json
{
  "meta": {
    "position": 42,
    "event_id": "local:42",
    "event_type": "otlp_record"
  },
  "event": {
    "id": "local:42",
    "signal": "log",
    "name": "claude_code.tool_result",
    "source": "claude-code",
    "time_unix_nano": "1787869200000000000",
    "observed_unix_nano": "1787869200001000000",
    "trace_id": null,
    "span_id": null,
    "content_digest": "...",
    "record": {}
  },
  "rows": {}
}
```

No engine receive time, environment variable, filesystem value, or current
delivery head is present.

### Tailapp-private `otel_event`

A tailapp declares exactly one internal event type produced by its normalizer
and consumed by its analytic folds. Its schema is application-specific:

```sql
CREATE EVENT otel_event (
  kind        TEXT NOT NULL,
  session_id  TEXT NOT NULL,
  tool        TEXT NOT NULL,
  success     BOOLEAN NOT NULL,
  duration_ms INTEGER
);
```

`otel_event` is private to the tailapp. It cannot be exported, queried,
retained, or consumed by another tailapp. A normalizer may produce zero or
more values per `otlp_record`; each payload is schema-checked before any
analytic fold runs. The host adds fixed metadata naming the root delivery
position and emission ordinal. The payload itself contains only declared
fields.

## 9. DDL and JSONata profile

### Allowed SQL declarations

The statement splitter handles quoted strings, quoted identifiers, line
comments, and block comments. The first profile accepts:

- exactly one `CREATE EVENT`, named `otel_event`;
- `CREATE TABLE` with an explicit primary key on every writable table;
- `CREATE INDEX`;
- `CREATE VIEW`;
- `CREATE EXPORT`;
- exactly one `CREATE NORMALIZER`; and
- one or more `CREATE FOLD` declarations.

Exports use a focused declaration:

```sql
CREATE EXPORT activity AS
  SELECT tool, calls, failures FROM tool_activity;
```

`CREATE EXPORT` defines a read-only relation contract and is the only way to
make application data visible outside its namespace at query time. Its query
is prepared against the tailapp's schema and must have unique named columns
with fixed logical types. No fold or application view may reference another
tailapp, including one of its exports.

It rejects `ALTER`, `DROP`, triggers, virtual tables, generated or
database-default values, PRAGMAs, `ATTACH`, `DETACH`, extension loading, and
DDL containing ambient date/time/random functions. A revision whose relational
DDL digest is unchanged may continue over the current database. Changed
relational DDL requires explicit reset activation; this version does not
migrate a live database in place.

Logical column types are `TEXT`, `INTEGER`, `REAL`, `BLOB`, `BOOLEAN`, and
`JSON`. Integers crossing JSONata are restricted to the exactly representable
range. Nanosecond timestamps therefore remain decimal text unless a tailapp
explicitly derives a smaller unit.

### Normalizer and fold declarations

The normalizer consumes only the built-in `otlp_record`, may use zero or more
declared reads, must declare `EMITS otel_event`, and may additionally write
tables. Emission is zero-or-more at runtime:

```sql
CREATE NORMALIZER normalize_otel ON otlp_record
READ catalog OPTIONAL ONE AS
  SELECT tool, category FROM tool_catalog WHERE tool = :event.name
USING 'folds/normalize.jsonata'
WRITES tool_catalog
EMITS otel_event;
```

Analytic folds consume only private `otel_event` values and write tables:

```sql
CREATE FOLD observe_tools ON otel_event
READ prior OPTIONAL ONE AS
  SELECT tool, calls, failures FROM tool_activity WHERE tool = :event.tool
USING 'folds/activity.jsonata'
WRITES tool_activity, sessions;
```

Only the normalizer may use `EMITS`, and its only event target is `otel_event`.
Analytic folds cannot consume `otlp_record` or emit events. Every table has
exactly one writer: either the normalizer or one analytic fold. An analytic
fold may read normalizer-owned tables and its own tables, but not a table owned
by another analytic fold; the normalizer may read only its own tables. These
rules apply through views as well as direct table references. This makes the
topology a fixed root and independent analytic leaves rather than a general
graph.

Read cardinalities are `ONE`, `OPTIONAL ONE`, and `MANY LIMIT <positive-int>`.
Every `MANY` query must have a deterministic `ORDER BY`. Reads are parameterized
`SELECT` statements prepared against the completed scratch schema. Parameters
come only from declared event scalar fields. The compiler records selected
columns and refuses `SELECT *`, duplicate output names, writes, PRAGMAs,
subqueries outside the admitted profile, cross-namespace relations,
undeclared tables/columns/functions, or a plan whose deterministic work bound
cannot be enforced.

The first implementation may begin with complete-primary-key equality reads,
as proven by the Gitseq spike, then add the bounded `MANY` profile before the
acceptance example depends on it. Historical-position reads are deferred.

### JSONata input and output

Every program evaluates against `{meta,event,rows}`. A normalizer returns:

```json
{
  "decision": "effective",
  "facts": [{"kind": "tool-failed", "tool": "shell"}],
  "events": {
    "otel_event": [
      {"kind": "tool-result", "session_id": "s1", "tool": "shell", "success": false}
    ]
  },
  "tables": {
    "tool_catalog": {
      "upsert": [{"tool": "shell", "category": "command"}]
    }
  }
}
```

An analytic fold has the same shape without `events`:

```json
{
  "decision": "effective",
  "facts": [],
  "tables": {
    "tool_activity": {
      "upsert": [{"tool": "shell", "calls": 8, "failures": 1}]
    }
  }
}
```

`decision` is `effective` or `ineffective`. `facts` is a bounded array of JSON
objects. In normalizer output, `events.otel_event` is an ordered array of
payloads and no other event key is valid. Each declared writable table may
contain `insert`, `upsert`, or `delete` arrays. Inserts and upserts provide
complete rows; deletes provide the complete primary key. Undeclared events,
tables, operations, columns, emissions, or writes are errors. An `ineffective`
decision emits no events and applies no table changes.

The normalizer may emit events with an empty `tables` object, update tables
without emitting events, or do both. One evaluation may change many rows in
many normalizer-owned tables. Each analytic fold likewise may change many rows
in many tables it owns. This covers filtering, normalization, enrichment,
rollups, tear-off, and denormalization without exposing a general event graph.

The declared application tables are JSONata's memory across events. A fold can
read and update counters, session state, correlation records, bounded windows,
or selected history. No general input-event buffer is exposed to the evaluator.
If an application wants raw or normalized history, it must declare a table and
write the chosen rows itself. Those rows then have explicit schema, retention
behavior expressed by later folds, and ordinary SQL visibility.

Remove `$now`, `$millis`, `$random`, `$shuffle`, `$eval`, dynamic code, and all
ambient or non-total functions. Canonicalize object iteration where it becomes
observable. The wrapper enforces input, depth, range, step, allocation, output,
fact, and row-change bounds. If the upstream evaluator cannot enforce
deterministic step and allocation bounds, maintain a narrow fork or reduce the
admitted language; wall-clock cancellation alone is not sufficient for active
production revisions.

Initial per-evaluation maxima:

- encoded input: 256 KiB;
- program source: 64 KiB;
- depth: 64;
- generated range: 4,096;
- output: 256 KiB;
- normalized events: 256;
- facts: 64;
- total row changes: 1,024;
- rows in one declared `MANY` read: 1,024.

Machine configuration may lower but not raise limits for an existing runtime
profile. The normalized-event limit is per tailapp and source record; there is
no recursive emission or wave-wide event expansion.

## 10. Projection database and folding

Create one durable SQLite database per tailapp. Enable WAL, foreign keys,
defensive mode, disabled trusted schema, and disabled extension loading. One
writer connection owns live folding. The database is materialized application
state, not a disposable cache: without an external replay source, deleting or
corrupting it loses observations already consumed.

The database contains:

- declared application tables, indexes, and views;
- `tailapp_projection_identity` with tailapp, revision, runtime profile, and
  activation mode;
- `tailapp_frontier` with activation boundary, last consumed position and
  source event ID, completeness, and gap; and
- `tailapp_stats` with bounded aggregate operational counters.

The engine does not persist a platform event, decision, fact, or derivation
row per input. A tailapp that needs history declares and maintains an
application table for it.

For each `otlp_record` in delivery wave *n*, one tailapp transaction:

1. loads the canonical source record;
2. executes the normalizer's declared reads against committed state at
   position *n - 1*;
3. evaluates the normalizer once and validates all event payloads and table
   operations;
4. applies normalizer-owned table changes inside the transaction;
5. presents each private `otel_event`, in emission order, to every analytic
   fold bound to it;
6. executes each analytic fold's declared reads against its own evolving state
   and the normalizer-owned tables, evaluates it, and validates its changes;
7. applies each fold's changes to its exclusively owned tables;
8. updates bounded frontier/stat counters and the source event ID; and
9. commits every normalization and analytics change atomically.

Analytic folds are ordered by their declaration position for reproducible
diagnostics, but they cannot observe one another's tables, so that order is
not application semantics. Repeated `otel_event` values are processed in
emission order and a fold can observe its own earlier changes within the same
root transaction.

If the normalizer is ineffective, it changes no tables and no analytic fold
runs. If it is effective but emits no `otel_event`, its table changes apply and
no analytic fold runs. A valid ineffective analytic result also advances
normally. Facts are discarded unless the producing program writes the desired
fact to an application table. After the projection transaction commits, mark
the inbox obligation consumed. If the engine crashes between those commits,
the projection's last source event ID proves consumption and recovery settles
the obligation without evaluating twice.

Private `otel_event` values exist only within this transaction. They need no
outbox because the inbox record remains durable until the complete transaction
commits and its obligation is settled. The engine completes the flat wave for
all active tailapps before starting source position *n + 1*.

Any execution failure rolls back, records gap metadata in a separate small
transaction, detaches only that tailapp from live delivery, and settles its
current and queued obligations as detached. Other tailapps continue. The
durable materialized state remains at its last successful source record.

### Activation

Activation compiles the expected draft revision and chooses one of two explicit
modes. It has no cross-tailapp validation or coordination because definitions
are independent.

**Continue** is permitted only when the relational DDL digest exactly matches
the active revision. The scheduler stops assigning new obligations at boundary
*b*, drains that tailapp through *b*, updates the active normalization/fold
profile and projection identity under a recoverable activation journal, then
resumes with records after *b*. Existing materialized tables remain intact. A
failure before the pointer switch leaves the old revision active.

**Reset** is required for first activation or changed relational DDL. Build an
empty candidate database from the compiled DDL, stop assigning obligations at
boundary *b*, detach any old outstanding obligations, atomically switch queries
and future delivery to the candidate, and retire the old database. The new
revision observes only events accepted after *b*. The API requires
`mode=reset` and an acknowledgement that prior materialized state will be
discarded.

Activating an earlier revision is allowed under the same rules: continue when
its relational digest matches, otherwise reset. It does not replay events that
arrived while another revision was active.

On engine restart, validate projection identity and frontier, finish or roll
back an activation journal, and resume pending inbox obligations. A missing,
corrupt, or profile-incompatible projection is `unavailable`; Tailapp cannot
rebuild it without retained input and requires explicit reset or a later
external replay operation.

## 11. Query contract

Queries name one primary tailapp and its active revision. Use a small read-only
pool per active projection. Connections use SQLite `query_only`, a
default-deny authorizer, runtime limits, progress interruption, and context
cancellation.

Permit `SELECT` and read-only common table expressions over the primary
tailapp's application tables and views. A request may also supply a bounded
map of request-local SQL aliases to other active tailapps. The engine mounts
those databases read-only and authorizes only their named exports as
`alias.export_name`, including the private reads needed to evaluate that
export. This mounting is query input, not stored application metadata. Deny
other private relations, writes, DDL, PRAGMAs, caller-controlled attachment,
extension loading, internal SQLite relations, private identity/frontier
relations, and every function not explicitly allowed.

Execute queries at a barrier between flat delivery waves. Every available
mounted tailapp therefore exposes the same completed source position. The
result includes each namespace's revision and export-contract digest so a
caller can reproduce the namespace view it queried.

Request defaults and hard maxima:

- SQL bytes: 16 KiB;
- parameters: 64;
- returned rows: default 256, maximum 1,000;
- encoded result: 1 MiB;
- concurrent queries: 8 globally, 4 per tailapp;
- deterministic SQLite opcode budget: fixed by runtime profile;
- wall safety deadline: 5 seconds.

The result is:

```json
{
  "tailapp": "tool-activity",
  "revision": "sha256:...",
  "delivery_head": 1280,
  "interpreted_position": 1280,
  "schemas": [
    {"alias": "cost", "tailapp": "session-cost", "revision": "sha256:...", "contract": "sha256:...", "interpreted_position": 1280}
  ],
  "complete": true,
  "gap": null,
  "columns": [{"name": "tool", "type": "TEXT"}],
  "rows": [["shell"]],
  "truncated": false
}
```

JSON values return structured JSON, blobs return tagged base64, SQL null returns
JSON null, and integers outside the safe JSON range return tagged decimal
strings. If `expected_revision` or `expected_position` does not match, refuse
with `frontier_changed` rather than mixing pages from different states.

## 12. Local control API, MCP, and CLI

### Application service operations

Implement one typed service with:

- `Health`
- `TailappsList`, `TailappGet`, `TailappCreate`, `TailappDelete`
- `ElementPut`, `ElementDelete`
- `Validate`, `Activate` (with `continue` or `reset` mode)
- `Status`, `Schema`
- `Query`

All mutations accept an idempotency key. Draft mutations and activation also
accept expected revisions. Errors have a stable code, human message, and
optional field/path/line/column details.

`Schema` reports the private local relations, public query exports and their
contract digests, table-writer ownership, the internal `otel_event` schema,
and normalizer/fold bindings. It reports no cross-tailapp dependencies because
none exist.

### MCP tools

Expose these stdio tools with concise JSON schemas:

- `tailapps_list`
- `tailapp_get`
- `tailapp_create`
- `tailapp_delete`
- `tailapp_put_element`
- `tailapp_delete_element`
- `tailapp_validate`
- `tailapp_activate`
- `tailapp_status`
- `tailapp_schema`
- `tailapp_query`

Source-returning tools include revision digests. Mutation tools require
`expected_revision` after creation. SQL and source results are bounded; a tool
returns a continuation or explicit truncation rather than silently dropping
data. Tool descriptions tell agents that draft editing is non-live until
activation.

`tailapp mcp` starts only the stdio adapter and connects to the resident. It
does not start a second engine or write SQLite directly.

### CLI

Provide:

```text
tailapp init
tailapp serve [--otlp-http 127.0.0.1:4318]
tailapp health [--json]
tailapp apps list|create|get|delete
tailapp apps put|rm|validate|activate|status|schema
tailapp query APP --sql SQL [--param JSON] [--json]
tailapp ingest-fixture FILE [--json]
tailapp mcp
```

Commands use the control socket except `init`, `serve`, and `mcp` process
setup. Human output is concise; `--json` emits the same response objects used
by MCP and the service.

## 13. Operational and security requirements

- Never log prompt, response, tool input, attribute, body, SQL parameter, or
  fold input/output content at the normal operational log level.
- Log request IDs, positions, tailapp names, revision digests, durations,
  counts, stable error codes, and gap locations.
- Refuse non-loopback OTLP binding.
- Create the local socket and data files for owner access only.
- Refuse symlink traversal when resolving state or element paths.
- Never load SQLite extensions or arbitrary native/plugin code.
- Treat tailapp DDL, JSONata, and SQL queries as untrusted input.
- Bound every request body, source element, fold input/output, query, result,
  queue, and concurrency pool.
- Shut down in order: stop accepting OTLP, finish or cancel bounded commits,
  stop workers, checkpoint/close databases, remove the socket.

The local-user trust model means there is no application authentication in
version one. The documentation must state this plainly.

## 14. Tests and milestones

### Milestone A: profile compiler

- Port the narrow, proven Gitseq spike behavior into Tailapp-owned packages.
- Compile a sample `application.sql` and JSONata fold.
- Reject malformed statement splitting, escaping paths, duplicate names,
  ambient functions, undeclared reads/writes, invalid output, and limits.
- Derive stable relation export contracts and validate the private
  `otlp_record` -> `otel_event` -> tables topology.
- Reject cross-tailapp references, multiple normalizers, analytic emission,
  multiple table writers, and reads from another analytic fold's tables.
- Pin compatibility fixtures for the admitted JSONata subset.

### Milestone B: inbox and OTLP receiver

- Decode protobuf and JSON logs, spans, and metrics fixtures.
- Prove canonical JSON and flattening order are byte-stable.
- Prove a malformed or over-limit batch commits nothing.
- Prove concurrent requests produce unique contiguous positions.
- Prove consumed records are deleted, pending records survive restart, and a
  full inbox returns retryable backpressure.

### Milestone C: projection, normalization, and query

- Feed canonical records through a normalizer and its analytic folds; check
  exact private events, normalizer-owned lookup changes, rollups, tear-offs,
  denormalized rows, and the aligned frontier.
- Run two independent tailapps over the same source records and join their
  explicit exports using request-local query aliases.
- Prove event transactions roll back and report gaps.
- Prove declared tables can express counters, correlation, and bounded history
  without access to prior inbox records.
- Adversarially test the SQL authorizer and all query/result bounds.
- Prove a bounded reader can run while the writer advances WAL state.
- Crash after a projection commit but before obligation completion and prove
  recovery does not evaluate the event twice.
- Crash during normalization or analytics and prove the whole tailapp
  transaction rolls back while the durable inbox record remains retryable.

### Milestone D: revision lifecycle

- Exercise create, element put/delete, validate, continue activation, refused
  incompatible continuation, explicit reset, failed activation, and deletion.
- Race expected-revision mutations and prove only one wins.
- Prove compatible activation preserves materialized rows and changes behavior
  only after its boundary.
- Prove reset discards only the named tailapp's old materialized state and
  begins with future events.
- Prove one tailapp can be activated, reset, or deleted without coordinating
  another tailapp that its exports are sometimes queried alongside.
- Crash at each activation boundary and prove restart chooses one complete,
  identity-matching state, never a hybrid.

### Milestone E: interfaces and harness smoke tests

- Run every service operation through CLI and MCP conformance tests.
- Configure current Codex to export OTLP logs to the loopback receiver and
  observe at least one queryable session/tool event.
- Configure current Claude Code to export OTLP log events and observe at least
  one queryable prompt/tool event with content gates left at their defaults.
- For OpenCode, use native OTLP when its tested release supports it; otherwise
  provide a thin example adapter from the documented plugin event stream to
  OTLP. The engine contract must not vary between those paths.
- Record harness versions in fixtures and treat vendor field mappings as
  compatibility tests, not stable Tailapp engine schema.

## 15. End-to-end acceptance

The initial implementation is accepted when an automated test or scripted
demo proves all of the following:

1. A fresh `TAILAPP_HOME` initializes and one resident owns it.
2. Independent `tool-activity` and `session-cost` tailapps are created through
   the public service.
3. Each declares one normalizer from built-in `otlp_record` to its own private
   `otel_event`, one or more analytic folds, owned tables, and a read-only
   query export.
4. Reset activation publishes each tailapp independently at a precise delivery
   boundary.
5. Protobuf and JSON OTLP fixtures containing logs and spans are accepted at
   the standard endpoints and receive consecutive delivery positions.
6. For each tailapp, the normalizer optionally updates its lookup tables,
   emits schema-valid private events, and its analytics folds materialize
   expected rollup and tear-off rows in one transaction.
7. Inbox content is deleted after obligations settle. The same bounded SQL
   query joins both tailapps' explicit exports under request-local aliases and
   returns identical typed rows and aligned namespace frontiers through CLI
   and MCP.
8. An irrelevant OTel event is consumed as ineffective or unhandled without
   changing application tables or leaving a platform event-history row.
9. A second normalizer, wrong internal event schema, analytic event emission,
   duplicate table writer, cross-tailapp fold read, and analytic-to-analytic
   table read are each refused before activation.
10. A schema-compatible fold update activates at one boundary, preserves prior
   materialized rows, and affects only later events.
11. A changed table schema is refused in continue mode and succeeds only with
    explicit reset acknowledgement; the reset tailapp then sees future events
    only.
12. A deliberately failing fold opens a gap and detaches only its tailapp while
    another tailapp and the inbox continue progressing.
13. A write, PRAGMA, attachment, unsafe function, undeclared private read,
    over-budget query, stale
    expected frontier, and oversized result are each refused with the expected
    stable error code.
14. Either tailapp can be reset or deleted without changing the other's active
    revision or materialized state; a later query naming the absent export is
    refused as a query-resolution error.
15. Restarting the engine recovers inbox delivery, active revisions, and
    materialized state without duplicate interpretation, including a crash
    between projection and obligation commits.
16. `go test ./...`, static analysis, and the scripted demo pass on clean macOS
    and Linux environments.

## 16. Explicit non-goals

Do not add these to the first implementation to make a demo look complete:

- remote authentication or non-loopback listeners;
- Bedrock, LiteLLM, hosted gateway, or cloud database connectors;
- OTLP/gRPC unless a named harness cannot be exercised over HTTP;
- dashboards, alerting, or a browser UI;
- native fold helpers, WASM, SQLite extensions, or arbitrary plugins;
- a persistent event log, historical replay, archival, or automatic redaction;
- historical-position fold reads;
- normalizer chains, analytics-to-analytics event or table dependencies,
  fold-time cross-tailapp access, stream joins, or shared writable tables;
- online schema migration; or
- SQL writes from CLI or MCP.

Any one of these can be designed later without weakening the initial delivery,
revision, isolation, or frontier contracts.

## 17. Design basis

- [Tailapp architecture](2026-08-28-tailapp-architecture.md)
- Gitseq's [JSONata-with-DDL application interface](https://github.com/generalbusiness-ai/gitseq/blob/3a5d952c8e1d94ff4d07ce666ca35085571ef857/notes/2026-08-26-jsonata-ddl-application-interface.md)
- Gitseq's [implementation design](https://github.com/generalbusiness-ai/gitseq/blob/3a5d952c8e1d94ff4d07ce666ca35085571ef857/notes/2026-08-26-jsonata-ddl-application-implementation.md)
- Gitseq's [spike README and code](https://github.com/generalbusiness-ai/gitseq/tree/3a5d952c8e1d94ff4d07ce666ca35085571ef857/spike/jsonataddl)
- [OpenTelemetry Protocol specification](https://opentelemetry.io/docs/specs/otlp/)
- [Claude Code monitoring and event reference](https://code.claude.com/docs/en/monitoring-usage)
- [Codex configuration reference](https://developers.openai.com/codex/config-reference)
- [OpenCode plugin events](https://opencode.ai/docs/plugins/)
