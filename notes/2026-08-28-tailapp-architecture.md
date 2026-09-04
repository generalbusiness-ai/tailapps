---
date: 2026-08-28
status: initial architecture design
companion: notes/2026-08-28-tailapp-initial-implementation.md
rests_on:
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:a02e8da110b93948746edf9adf13e6dd00633271
---

# Tailapps architecture

> **Status:** Implemented baseline. This note records the architectural
> decisions; the [documentation map](../docs/README.md) and references describe
> the current user-facing interfaces.

Tailapps is a local analytics engine for agent-harness telemetry. It accepts
OpenTelemetry (OTel) streams and continuously materializes user-defined
relational projections. It does not retain the telemetry stream as an event
log.
Agents manage those projections as **tailapps**: small packages of event DDL,
table DDL, fold bindings, and deterministic JSONata programs. They inspect the
results with bounded, read-only SQL, primarily through MCP and equivalently
through a CLI.

This design applies the JSONata-with-DDL model developed in Gitseq without
making Gitseq itself a runtime dependency. The important contract is retained:
typed events, declared reads from materialized state, deterministic JSONata
transitions, validated typed row changes out, and SQL over materialized state.
Unlike Gitseq, Tailapps does not initially retain an authoritative event
history, so its materialized tables are durable state rather than disposable
replay caches.

## Architectural decisions

1. **One local engine owns ingestion and all writes.** CLI and MCP are clients
   of that engine; neither opens writable databases directly.
2. **Standard OTLP is the ingestion boundary.** Harness-specific configuration
   points exporters at Tailapp. The core does not parse Claude Code, Codex, or
   OpenCode transcript files.
3. **The event stream is not stored.** A bounded durable inbox exists only to
   carry unconsumed records across backpressure and process crashes. A record
   is deleted after every captured consumer has committed or detached. Each
   tailapp may retain a tiny, fixed-size memory-only diagnostic sample of its
   most recent ineffective inputs; samples disappear on restart or activation
   and are not replayable history.
4. **Each active tailapp has an isolated durable SQLite projection.** Its
   materialized tables are the tailapp's memory and locally authoritative
   analytic state. Ordinary SQL names need no global prefix, and a broken fold
   cannot stop other tailapps.
5. **Editing and activation are separate.** Agents manipulate draft elements
   with optimistic revision checks. A storage-compatible revision can continue
   over existing tables; an incompatible writable-table change requires an
   explicit reset and begins with future events.
6. **MCP is the primary product interface; the CLI has parity.** Both adapt one
   internal application service and therefore share validation, bounds, and
   error semantics.
7. **The first trust boundary is the local user account.** The engine binds to
   loopback and a user-owned local control socket. Remote collectors, shared
   tenancy, and network authentication are deferred.
8. **Tailapps are isolated fold namespaces.** Tables are private by default.
   Explicit read-only exports are visible only to multi-namespace SQL queries,
   never to another fold.
9. **Every tailapp has one fixed two-stage pipeline.** Its normalizer consumes
   the engine's canonical `otlp_record` and may update normalizer-owned tables
   while producing zero or more typed, app-private `otel_event` values.
   Analytics folds consume only those values and produce only table changes.
   Analytics cannot emit events, no stage can import another tailapp, and no
   stream join or arbitrary fold graph exists.

## System shape

```text
Claude Code ─┐
Codex ───────┼─ OTLP/HTTP ─> receiver ─> bounded durable inbox
OpenCode ────┘                                  │ consume then delete
                                                v
tailapp sources ─> registry ─> compiler ─> projection supervisor
      ^               │                         │
      │ draft CRUD    │ active revision         ├─> app A SQLite
      │               v                         ├─> app B SQLite
CLI ──┼──────────> local application service <──└─> app N SQLite
MCP ──┘                    │
                          └─ bounded read-only SQL + schema/frontiers
```

The diagram has three distinct planes:

- the **data plane** receives and orders OTel records;
- the **definition plane** compiles and activates tailapp revisions; and
- the **query plane** exposes projections and their exact frontiers.

They share one engine process so there is one owner for ordering, activation,
and SQLite writers.

### Reusable application-semantics module

The deterministic compiler and evaluator are a second Go module at
`github.com/generalbusiness-ai/tailapps/jsonataddl`. That module owns source
validation, DDL compilation, JSONata confinement and evaluation, logical-value
conversion, immutable application and read/mutation plans, deterministic
bounds, and the default-deny SQLite read authorizer. It contains no dependency
on Tailapps internal packages.

Tailapps remains the host. Its internal packages own OTLP canonicalization,
the dialect and composed host identity, SQLite connection and transaction
lifetime, mutation execution, projection frontiers, activation, registry,
query, CLI, MCP, and resident lifecycle. Producer and dialect consistency tests
stay on this side of the module boundary. The versioned core corpus stays with
the reusable module.

The root module replacement binds the host to the same local source for
development. Independent consumers resolve module-path tags such as
`jsonataddl/v0.1.0` without a workspace or replacement. Packaging the existing
boundary this way does not change its runtime contract or composed identity;
a later semantic change must still change the relevant identity component.

## Core concepts

### Delivery record

One OTel log record, span, or metric data point becomes one `otlp_record` at
a monotonically increasing local position. Its canonical form records the OTel
signal, source timestamps and identifiers, resource and instrumentation scope,
attributes, body or value, and the original signal-specific structure.

Receiver order, not source timestamp, is fold order. OTLP does not provide a
universal delivery identifier, so the engine does not silently deduplicate
equal records. It preserves at-least-once delivery and exposes content digests
and native trace, span, session, and request IDs so tailapps can define
domain-appropriate deduplication. `otlp_record` is a fixed engine input, not
the tailapp's event DDL.

The canonical record remains in the inbox only while an active tailapp still
owes consumption. Once all obligations are complete, the engine deletes it.
It is not queryable through MCP or SQL and is not a source for historical
replay.

### Application memory

JSONata itself is stateless. Useful behavior comes from declared reads of the
tailapp's materialized tables. The normalizer begins from committed state at
the previous source-record boundary. Its changes then become visible to the
analytic stage within the same transaction. Each analytic fold sees its own
earlier changes while consuming repeated `otel_event` values from that source
record. A tailapp can therefore store counters, sessions, correlation keys,
outstanding tool calls, bounded event-time windows, or selected history
without forcing the engine to retain every input record.

Time-based expiry can happen when a later event supplies a newer event time.
Actions that must fire during a completely idle stream would require explicit
platform tick events or another later scheduling design; folds never read the
wall clock.

A tailapp processes one delivery record in a single transaction. First, one
normalizer evaluates `otlp_record`. It may update its own lookup or
normalization tables and produce a bounded ordered list of values conforming
to that tailapp's `CREATE EVENT otel_event` declaration. Then analytic folds
consume those `otel_event` values and update their own tables. The values live
only for that transaction; they are neither an outbox nor a queryable or
replayable stream.

Each table has one declared writer. The normalizer reads only its own tables.
Analytic folds may read normalizer-owned tables and their own tables, but not
tables owned by another analytic fold. This permits parsing, enrichment,
rollups, counters, tear-off into normalized child tables, denormalized query
shapes, and sessionization without creating forward, backward, or implicit
fold chains. All changes for the root event commit or roll back together.

### Namespace and query visibility

Every tailapp is a logical database schema. Its tables and views are private by
default. It may expose a named read-only relation with an explicit export
declaration. An export fixes the relation's columns, logical types,
nullability, and defining query into an export-contract digest.

An MCP or CLI query may explicitly mount several active tailapps under local
SQL aliases and read only their exported relations as `alias.relation`. This is
query-time composition, not a durable dependency between tailapp definitions.
No fold, fold read, or application view may reference another namespace.
Tailapps can therefore be installed, updated, reset, and deleted independently.

The engine runs queries between flat delivery waves and returns a frontier
vector for every mounted namespace. There is no writable cross-namespace
surface and no implicit global catalog.

### Tailapp element

An element is a named source file belonging to one tailapp. The first format
has only:

- exactly one `application.sql`; and
- one or more `folds/*.jsonata` programs named by `CREATE FOLD` statements.

The `application.sql` names one normalizer and one or more analytic folds. The
tailapp name comes from the registry. A content digest supplies immutable
revision identity, so a manifest is not necessary initially. Documentation
may accompany a package, but it is not interpreted and does not enter the
runtime profile unless a later design says so.

### Draft and active revision

Every tailapp has at most one mutable draft and one active immutable revision.
Element changes advance the draft revision. Validation compiles a snapshot of
that draft. Activation changes behavior at a precise delivery boundary.
Continue compatibility is based on stored writable-table shape, not the whole
DDL text: every existing writable table must retain its columns, types,
constraints, and primary key, while new empty tables may be added. Event,
normalizer, fold, index, view, and export changes are continue-safe; indexes
and views are rebuilt at the boundary. Changing or removing an existing
writable table requires explicit `reset`, which creates empty materialized
tables and begins at the next event. Failed editing or compilation leaves the
old active revision queryable.

Deletion follows the same rule. Deleting an element changes only the draft.
Deleting a tailapp retires its active projection from the query surface. It
does not affect the inbox obligations or materialized state of other tailapps.

An engine upgrade never silently reinterprets an active revision under a new
runtime profile. If the binary no longer supplies the active profile, control
and query remain available but OTLP readiness is held closed. The operator or
agent revalidates each source set under the new profile and continue-activates
it when its writable tables are compatible, or explicitly resets/deactivates
it. Ingestion resumes only after every active projection is runnable.

### Projection frontier and gap

A projection result always names:

- the active revision digest;
- the current engine delivery position known when the result was read;
- the last interpreted position; and
- whether interpretation is complete.

Invalid input, evaluator failure, an exceeded deterministic limit, invalid
fold output, or a DDL constraint failure rolls back that event transaction and
opens a **gap** at its position. The affected tailapp is detached from live
delivery and reports the reason so the bounded inbox can continue draining.
Other tailapps and ingestion continue. Repair requires a schema-compatible
reactivation that resumes with future events, an explicit reset, or a future
external replay facility. A business-level fold decision of `ineffective` is
not a gap.

Because the inbox is not a history store, records accepted after detachment and
before repair are permanently absent from that tailapp. This fail-stop policy
is deliberate in version one. A declared skip-and-count error policy may be
designed later for analytics where continuity is preferable to exactness.

## Components

### 1. OTLP receiver

The receiver accepts OTLP/HTTP on loopback and translates logs, traces, and
metrics into the canonical delivery envelope. It supports protobuf and JSON
encodings, validates request and record limits before committing, flattens a
batch in wire order, and acknowledges only after its inbox transaction is
durable.

The receiver is deliberately vendor-neutral. Current Claude Code can export
metrics, log events, and optional traces through OTLP; current Codex exposes
separate OTLP log, trace, and metric exporter settings. OpenCode can use its
OTel path where available or a small plugin that subscribes to its public event
stream and emits OTLP. Those are configuration or edge adapters, not fold
semantics.

### 2. Durable inbox

The inbox is a bounded queue in the engine's control SQLite database. It
assigns delivery positions, holds canonical JSON, and records which active
tailapps owed consumption when the event arrived. Projection workers consume
ordered pages. A consumer transaction and its completion marker are committed
before the inbox can delete the record.

The inbox is operational state, not analytic storage. Installing a tailapp
does not enroll it in records already accepted. Deleting or detaching a
tailapp settles its outstanding obligations. Capacity is finite; when a slow
consumer fills it, the receiver returns OTLP backpressure rather than growing
an accidental event store.

After an ineffective normalizer decision, the engine may copy that tailapp's
record into a fixed-size, memory-only diagnostic ring exposed over owner-only
CLI and MCP. The ring has both record-count and per-record byte bounds, clears
on resident restart or activation, and never participates in evaluation or
replay. Its payload exposure is explicit because ineffective-shape diagnosis
otherwise requires an external OTLP tap.

### 3. Tailapp registry and compiler

The registry stores drafts, immutable compiled revisions, active pointers,
read-only query exports, and diagnostics. The compiler:

1. separates SQL statements safely;
2. recognizes `CREATE EVENT`, `CREATE NORMALIZER`, and `CREATE FOLD`
   extensions;
3. asks SQLite to prepare the allowed relational DDL in a scratch database;
4. prepares declared fold reads as read-only statements;
5. compiles each referenced JSONata program under a pinned language profile;
6. derives read-only export contracts;
7. verifies the fixed `otlp_record` -> private `otel_event` -> tables topology,
   single-writer table ownership, and private event schema;
8. rejects cross-namespace and analytic-fold-to-fold references;
9. verifies declared read/write authority and resource limits; and
10. hashes the canonical source set and complete runtime profile.

Unsupported or unsafe syntax is refused rather than guessed.

### 4. Projection supervisor and workers

The supervisor observes new inbox positions and active-revision changes. Each
tailapp has one logical worker and one SQLite writer. For delivery *n*, every
active tailapp runs its normalizer and the resulting analytic folds in one
transaction, beginning from its own committed state at *n - 1* and committing
its complete state at *n*. Transaction-local `otel_event` values disappear at
commit.

The first engine treats that fan-out as one flat wave and does not begin
delivery *n + 1* until every active consumer has committed or detached.
Independent tailapps may run concurrently because there are no fold-time edges
between them. This gives aligned query frontiers without a dataflow scheduler,
but accepts head-of-line blocking: one slow tailapp can fill the bounded inbox,
backpressure all harnesses, and eventually expose them to exporter-side drops.

### 5. Query service

The query service opens read-only connections to active projection databases.
It permits only bounded `SELECT` statements over application tables, views,
and public platform relations. Every result includes typed columns, bounded
rows, truncation state, and the exact revision/frontier. Callers can provide an
expected revision and frontier to prevent pagination across changing state.

A query names a primary tailapp and may explicitly mount exports from other
tailapps under request-local aliases. Arbitrary private access is refused. The
engine takes query snapshots between flat delivery waves, so every mounted
namespace names the same completed delivery position or is reported as
detached/unavailable.

### 6. Local application service

This service is the sole control API. It covers tailapp CRUD, element CRUD,
validation, activation, reset, status, schema discovery, and SQL query. The
resident engine exposes it over a user-owned local socket. The Go API, local
wire protocol, CLI, and MCP adapter are separate layers even if they initially
ship in one repository and binary.

### 7. MCP and CLI adapters

The MCP server runs over stdio for an agent and forwards calls to the local
engine. It offers focused tools for manipulating definitions and querying
results; it never exposes a generic filesystem or writable SQL tool.

The CLI maps the same operations into scripts and human diagnostics. JSON
output is available for every read and mutation. The CLI also owns engine
lifecycle commands and a fixture-ingestion command useful for tests.

## Interfaces

### Ingestion

- OTLP/HTTP endpoints: `/v1/logs`, `/v1/traces`, `/v1/metrics`.
- Loopback only by default.
- Protobuf and JSON request bodies.
- Success means every accepted record is durably queued for the active
  consumers; it may already have been consumed by the response.
- Malformed or over-limit requests change nothing and return an OTLP error.
- Valid records are never rejected because no active tailapp understands them.

### Definition management

- Names identify tailapps; revision digests identify immutable source sets.
- Every draft mutation takes `expected_revision` and returns a new revision.
- Validation returns structured diagnostics without activation.
- Export contracts cover read-only relations for explicit query-time mounting.
- Continuing activation takes an expected draft revision, requires compatible
  stored writable tables, and changes behavior at one delivery boundary.
- Reset activation is explicit, discards that tailapp's old materialized state,
  and begins with future events.
- The active source and compiled metadata are inspectable through MCP/CLI.

### Query

- A query names one active tailapp, SQL, positional parameters, and optional
  expected revision/frontier.
- The response names schema, rows, truncation, active revision, delivery head,
  interpreted position, completeness, and any gap.
- SQL is read-only and resource-bounded. It is never an ingestion or tailapp
  mutation interface.

### Engine control

- Start, stop, health, configuration, and reset are CLI operations.
- MCP exposes health and tailapp status but does not stop the engine hosting
  the calling agent's analytics.
- Engine configuration is machine-local, not part of a tailapp revision.

## Capabilities

The architecture supports:

- local OTLP collection from multiple concurrent harness sessions;
- schema-checked, deterministic normalization and analytics defined without
  recompiling Tailapp;
- materialized tables, indexes, views, joins, aggregates, rollups, normalized
  tear-offs, denormalized query shapes, and application-defined lineage;
- explicit query-time joins across exported read-only namespace relations;
- continuous agent-authored install, inspection, compatible update, reset, and
  delete;
- atomic fold-profile activation at a delivery boundary;
- bounded SQL and schema discovery through MCP;
- exact frontier reporting and deterministic table transitions; and
- isolation of one tailapp's schema and runtime failure from others.

Immediate value comes from a bundled `agent-guard` tailapp, not from the engine
alone. Its normalizer maps current Claude Code, Codex, and OpenCode telemetry
into one private event vocabulary. Its tables and exports cover telemetry
coverage, session progress, tool and operation policy findings, repeated
failures, repeated-action fingerprints, and bounded no-progress/loop signals.
Missing or redacted fields produce an explicit `unknown` coverage result,
never a claim of compliance. A periodic MCP or CLI query with a
caller-supplied event-time cutoff identifies sessions whose last observed
progress is stale; folds do not invent an idle-time event or read the clock.

These are detective controls. OTLP observation happens after or alongside
harness activity and cannot prevent a tool call, terminate an agent, or prove
that an unobserved operation did not happen. Inline enforcement requires a
later harness control adapter. Version one nevertheless gives agents and
operators immediately queryable, cross-harness evidence for policy and
behavior review.

It intentionally does not make arbitrary code, network calls, filesystem
access, clocks, randomness, dynamic evaluation, SQLite extensions, or ambient
process state available to JSONata folds.

## Trust, privacy, and failure boundaries

Agent telemetry can contain prompts, responses, commands, paths, tool inputs,
account identifiers, and model metadata. Tailapps therefore treats inbox and
projection files as private to the local user, creates state directories
with restrictive permissions, binds listeners to loopback, disables SQLite
extension loading, and avoids logging record bodies in its own operational
logs.

The first release trusts processes running as the same operating-system user.
It does not claim protection from a malicious local peer with access to the
socket or data directory. Tailapp source is untrusted declarative input:
compilation, the JSONata profile, SQLite authorizers, deterministic limits,
transaction boundaries, and result bounds confine it.

Failure is localized:

- an invalid OTLP request does not enter the inbox;
- an invalid draft does not affect an active revision;
- a projection gap stops only that tailapp and leaves all others running;
- a failed activation does not displace the old revision; and
- a cancelled or over-budget SQL query does not stop ingestion or folding.

## Initial scope

The first product is one local Go binary, one loopback OTLP/HTTP receiver, one
user-owned local control socket, one bounded durable SQLite inbox/registry, and
one durable SQLite projection per tailapp. It implements the minimal
JSONata-with-DDL profile, private fold namespaces, explicit query exports, MCP
over stdio, a parity CLI, bounded read-only SQL, draft/validate/continue-or-reset
activation, a bundled cross-harness `agent-guard` tailapp, and an independent
sample analytic that can be joined with it at query time.

The initial release targets macOS and Linux. It accepts OTel logs, spans, and
metric points but does not promise a stable semantic mapping for every vendor
attribute. Tailapps operate on the canonical envelope and preserved record.

## Deferred scope

The following require later designs:

- non-loopback OTLP, authentication, authorization, TLS termination, and
  multi-user tenancy;
- collectors for Bedrock, LiteLLM, hosted gateways, or other server streams;
- OTLP/gRPC if real harness compatibility cannot be achieved through HTTP;
- distributed ordering, replication, remote database services, and horizontal
  projection workers;
- retained event history, archival, upstream replay, and privacy redaction;
- inline policy enforcement, agent termination, alerts, and subscriptions;
- arbitrary fold-to-fold event emission, pipelines deeper than the fixed
  normalizer-to-analytics boundary, tailapp imports, dataflow graphs, stream
  joins, and fold-time cross-namespace reads;
- more than one private normalized event type per tailapp; the single
  `otel_event` constraint is an intentional version-one simplification, not a
  requirement of the two-stage architecture;
- declared skip-and-count execution-error policies;
- persistent federated catalogs, dashboards, alerts, and SQL subscriptions;
- native fold helpers or other extension capabilities;
- in-place projection schema migration; and
- arbitrary package dependencies or executable plugins in the core engine.

Remote scaling must preserve the semantic boundaries here: ordered delivery,
content-addressed tailapp revisions, deterministic folds, one logical writer
per projection, exact frontiers, and read-only bounded queries. A future
retained source may add replay, but the fold contract cannot depend on its
existence.

## Design basis

- Gitseq's [JSONata-with-DDL application interface](https://github.com/generalbusiness-ai/gitseq/blob/3a5d952c8e1d94ff4d07ce666ca35085571ef857/notes/2026-08-26-jsonata-ddl-application-interface.md)
- Gitseq's [first implementation design](https://github.com/generalbusiness-ai/gitseq/blob/3a5d952c8e1d94ff4d07ce666ca35085571ef857/notes/2026-08-26-jsonata-ddl-application-implementation.md)
- Gitseq's [working JSONata/DDL spike](https://github.com/generalbusiness-ai/gitseq/tree/3a5d952c8e1d94ff4d07ce666ca35085571ef857/spike/jsonataddl)
- [Claude Code OTel monitoring](https://code.claude.com/docs/en/monitoring-usage)
- [Codex configuration reference](https://developers.openai.com/codex/config-reference)
- [OpenCode plugin event interface](https://opencode.ai/docs/plugins/)
