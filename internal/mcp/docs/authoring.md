# Authoring a Tailapp

A Tailapp is one `application.sql` plus JSONata programs under `folds/`,
ending in `.jsonata`. The engine compiles the set into an immutable
profile; drafts are edited under optimistic revision control and go live
only at activation.

## application.sql

Five statement kinds, in any order:

- `CREATE EVENT otel_event (…)` — exactly one private event, always named
  `otel_event`, with typed columns (`TEXT`, `INTEGER`, `REAL`, `BOOLEAN`,
  `BLOB`, `JSON`; `NOT NULL` where required).
- `CREATE TABLE name (…, PRIMARY KEY (…))` — materialized state. Each
  table has exactly one writing program.
- `CREATE NORMALIZER name ON otlp_record USING 'folds/x.jsonata' EMITS
  otel_event;` — exactly one normalizer, consuming the host record and
  emitting only the private event. It may also `WRITES` its own tables.
- `CREATE FOLD name ON otel_event READ … USING 'folds/y.jsonata' WRITES
  table;` — one or more analytic folds. A fold may read tables it writes
  and tables the normalizer writes, never another fold's.
- `CREATE EXPORT name AS SELECT …;` — the explicit query surface.

## Programs

Each program is one JSONata expression receiving `{meta, event, rows}` and
returning one object: `{"decision": "effective"|"ineffective", "facts":
[…], "events": {…}, "tables": {…}}`. An ineffective decision may carry
facts but no events or row changes. Rows are validated against declared
columns; integers must be exactly representable JSON integers, blobs are
base64 text. Programs run confined: no clock, randomness, filesystem,
network, or ambient functions.

## The host record

The normalizer's input event is the canonical `otlp_record`: scalar
envelope fields `id`, `signal`, `name`, `source`, `time_unix_nano`,
`observed_unix_nano`, `trace_id`, `span_id`, `content_digest`, plus the
full `record` payload. See `tailapp://docs/otlp-records`.
