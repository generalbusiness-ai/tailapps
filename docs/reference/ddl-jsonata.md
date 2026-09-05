# DDL and JSONata reference

The Tailapps v1 runtime compiles a deliberately small language. It is SQL-shaped
for schema and bounded reads, with JSONata programs for deterministic
normalization and fold logic. The restrictions are part of the runtime profile
and are enforced at validation, not conventions.

For a working source set and installation sequence, start with the
[authoring guide](../authoring.md).

## Source set

A complete Tailapp contains:

- exactly one `application.sql`;
- one or more referenced `folds/*.jsonata` files; and
- no other paths.

Paths are relative, clean, slash-separated UTF-8 paths with no empty, `.` or
`..` segment. Each element is 1 to 64 KiB; the source set is at most 512 KiB.
Every JSONata source in the set must be bound by a program declaration.

Tailapp names match `^[a-z][a-z0-9-]{0,62}$`. DDL identifiers match
`^[A-Za-z_][A-Za-z0-9_]*$` and are unquoted.

## Required topology

Every valid application declares:

- exactly one private event named `otel_event`;
- exactly one normalizer consuming the built-in `otlp_record`;
- at least one analytic fold consuming `otel_event`;
- one or more writable tables, each with exactly one declared writer; and
- at least one explicit query export.

The topology is flat:

```text
OTLP record -> normalizer -> private otel_event -> analytic folds -> tables
```

The normalizer may also write its own tables. A normalizer read can reach only
normalizer-owned tables. An analytic fold can read normalizer-owned tables and
its own tables, but not a table owned by another analytic fold. Analytic folds
cannot emit events. These ownership rules exclude fold graphs, event chains,
and stream joins while allowing shared normalized context.

## DDL statements

Every statement ends with `;`. SQL line comments and block comments are
accepted in `application.sql`.

### Event

```sql
CREATE EVENT otel_event (
  session_id TEXT NOT NULL,
  value INTEGER NOT NULL,
  detail JSON
);
```

Only one event is allowed and it must be named `otel_event`. Event declarations
have columns but no keys or table constraints.

### Table

```sql
CREATE TABLE totals (
  session_id TEXT NOT NULL,
  value INTEGER NOT NULL CHECK (value >= 0),
  PRIMARY KEY (session_id)
);
```

Supported logical types are `TEXT`, `INTEGER`, `REAL`, `BLOB`, `BOOLEAN`, and
`JSON`. Every table requires an explicit inline or table-level primary key.
Table-level `UNIQUE (...)`, `NOT NULL`, and SQLite `CHECK` constraints are
usable. `DEFAULT`, generated columns, `AUTOINCREMENT`, `COLLATE`, and
`REFERENCES` are outside the profile.

### Index

```sql
CREATE INDEX totals_value ON totals(value);
CREATE UNIQUE INDEX totals_unique_value ON totals(value);
```

Indexes are derived schema: a continue activation may replace them without
changing the stored table shape.

### View

```sql
CREATE VIEW positive_totals AS
  SELECT session_id, value
  FROM totals
  WHERE value > 0;
```

Views use the application-query profile described below. View dependency
cycles are rejected.

### Export

```sql
CREATE EXPORT totals AS
  SELECT session_id, value
  FROM totals;
```

Exports are the only relations visible when another Tailapp mounts this one.
An export must select directly from base tables, not application views, and
must produce unique named columns with fixed logical types.

### Normalizer

```sql
CREATE NORMALIZER normalize ON otlp_record
READ context OPTIONAL ONE AS
  SELECT source, label
  FROM source_context
  WHERE source = :event.source
USING 'folds/normalize.jsonata'
WRITES source_context
EMITS otel_event;
```

`READ` clauses and `WRITES` are optional. `EMITS otel_event` is required and
must be last.

### Analytic fold

```sql
CREATE FOLD accumulate ON otel_event
READ prior OPTIONAL ONE AS
  SELECT session_id, value
  FROM totals
  WHERE session_id = :event.session_id
USING 'folds/accumulate.jsonata'
WRITES totals;
```

At least one `WRITES` table is required.

## Program reads

A program can declare multiple named reads before `USING`:

```text
READ name ONE AS SELECT ...
READ name OPTIONAL ONE AS SELECT ...
READ name MANY LIMIT N AS SELECT ... ORDER BY ...
```

Reads are simple named-column `SELECT` statements over one table or view.
`SELECT *`, SQL functions, qualified names, subqueries, compound queries, and
joins are rejected. `WHERE` terms, when present, are only
`column = :event.scalar_field` joined by `AND`.

`ONE` and `OPTIONAL ONE` require an event-key `WHERE`. At runtime `ONE` must
return exactly one row and `OPTIONAL ONE` zero or one. `MANY LIMIT N` supports
1 to 1024 rows, requires `ORDER BY`, and must read a table directly with an
ordering whose final columns are a declared primary or unique key. This makes
the bounded result order total and stable.

Read results appear in JSONata as `rows.<name>`:

- `ONE`: object
- `OPTIONAL ONE`: object or `null`
- `MANY`: ordered array of zero through the declared limit of objects; empty is `[]`

## JSONata input

A normalizer with no declared reads receives this complete input shape:

```json
{
  "meta": {
    "position": 57,
    "event_id": "local:57",
    "event_type": "otlp_record"
  },
  "event": {
    "id": "local:57", "signal": "log", "name": "example", "source": "demo",
    "time_unix_nano": null, "observed_unix_nano": null,
    "trace_id": null, "span_id": null, "content_digest": "example-digest",
    "record": {"attributes": {}}
  },
  "rows": {}
}
```

All three metadata keys and every envelope key below are required. Unknown
metadata or event members refuse; absent and null are distinct. The time and
trace/span fields permit null. The other envelope scalars are non-null TEXT.
`meta.position` is an exact-range INTEGER; event IDs and types are non-null TEXT.
For a normalizer, `event` has these scalar envelope fields and one object:

- `id`, `signal`, `name`, `source`
- `time_unix_nano`, `observed_unix_nano`
- `trace_id`, `span_id`, `content_digest`
- `record`, the canonical signal-specific OTLP object

`record.attributes`, `record.resource.attributes`, and `record.scope` provide
normalized attribute maps and instrumentation identity. `record.otel` retains
the decoded original OTLP record. The source is the first nonempty resource
attribute among `service.name`, `gen_ai.agent.name`, and `telemetry.sdk.name`,
or `unknown`. See [Canonical OTLP records](otel-records.md) for the complete
log, span, and metric-point shapes and value rules.

For an analytic fold, `event` is one object emitted against the declared
`otel_event` schema. `meta.event_id` adds an emission ordinal, while
`meta.position` remains the source inbox position. The ordinal is part of the
ID (for example `local:57#0`), never a separate metadata member. Every private
event column must be present with its declared type and nullability.

`rows` is a non-null closed object with exactly one member per declared read,
or `{}` for no reads. Each row has exactly the selected columns, using the
spelling in the read declaration. SQL column matching remains case-insensitive;
selecting `BALANCE` still enforces the type and nullability of a declared
`balance` column. Hosts map SQL result columns to those selected names. Table reads
use their declared logical types and nullability; view reads preserve the
selected-column shape with JSON-valued members, including null. Missing or
extra reads/columns, `MANY: null`, or wrong cardinality refuse before JSONata.

The core validates the same encoded input it passes to JSONata, preserving
JSON numbers without coercion. The host also calls `ValidateProgramInput`
before binding event values to read SQL. Opaque `record` contents retain their
host-defined canonicalization and existing evaluator behavior; scalar INTEGER
and REAL restrictions do not apply recursively inside opaque JSON.

## JSONata output

Every program returns exactly one object with `decision`, `facts`, and
`tables`; a normalizer may also return `events`:

```json
{
  "decision": "effective",
  "facts": [],
  "events": {"otel_event": []},
  "tables": {
    "totals": {
      "insert": [],
      "upsert": [],
      "delete": []
    }
  }
}
```

`decision` is `effective` or `ineffective`. An ineffective result cannot emit
events or change tables. `facts` is required and accepts up to 64 objects, but
v1 does not persist or expose facts; use `[]` unless developing against a later
runtime contract.

A normalizer can emit up to 256 `otel_event` objects. All emitted and inserted
or upserted rows must provide exactly the declared column names and values of
the correct logical type; insert/upsert rows are complete even for nullable
columns. Delete rows contain the complete primary key. A program can change at
most 1024 rows per evaluation.

All normalizer writes, emitted events, analytic reads/writes, and the frontier
advance for one OTLP record occur in one SQLite transaction. Private events
disappear after that transaction.

## Bounded JSONata profile

The evaluator admits JSONata expressions but rejects:

- `$now`, `$millis`, `$random`, `$shuffle`, and `$eval`;
- user-defined `function(...)` lambdas;
- unquoted `*` (wildcards and multiplication);
- unquoted `..` generated ranges; and
- every function call except the allowlist below.

Allowed functions:

```text
$abs $boolean $ceil $contains $count $exists $floor $length $lookup
$lowercase $max $min $not $number $round $string $substring $sum
$uppercase
```

Because unquoted `*` is forbidden, JSONata block comments are unavailable.
Use descriptive variable names and formatting instead. Programs are limited to
64 KiB, input/output are each limited to 256 KiB, encoded input container depth
is limited to 1024 (root depth 1), evaluator execution depth remains 64, and a
250 ms wall deadline is retained as a secondary process-safety limit.

JSON integers written to `INTEGER` must be exactly representable in the
JavaScript-safe range `-(2^53-1)` through `2^53-1`. Reals must be finite. BLOB
values are standard base64 strings; JSON values can be any decoded JSON value.

## Application-query restrictions

Fold reads, views, and exports reject ambient clock/random functions, mutation
and schema statements, pragmas, attach/detach, compound queries, CTEs,
subqueries, `SELECT *`, and SQL functions. Views and exports may use explicit
`JOIN`; fold reads may not. Comma-separated joins are not admitted.

This is stricter than the separate operator/agent query interface documented
in the [query SQL reference](query-sql.md).
