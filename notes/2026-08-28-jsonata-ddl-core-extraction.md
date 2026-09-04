---
date: 2026-08-28
status: proposed extraction design; implementation is out of scope
companion: notes/2026-08-28-tailapp-architecture.md
rests_on:
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:fffc1c9006c8261bd84c4e94f8ce0010ac81d8ab
---

# Extracting `jsonata-ddl-core`

The Tailapps project's `internal/profile` package is now a production-quality implementation
of the small JSONata-with-DDL model. Gitseq has the platform-neutral interface
note and an intentionally incomplete spike. The useful direction is therefore
to extract the Tailapps project's mature implementation behind a host-neutral contract, not
to merge two peers or make either product depend on the other's runtime.

This note defines that extraction. It deliberately does not decide the
library's repository or whether Gitseq will adopt it; those are operator
decisions in [Open questions](#open-questions). No implementation is included.

## Contract and scope

The shared semantic contract is Gitseq's
`notes/2026-08-26-jsonata-ddl-application-interface.md`: a typed event enters,
declared reads produce `{meta,event,rows}`, a confined JSONata transition emits
validated typed row changes, and SQL exposes the materialized projection. The
core adopts that platform-neutral interface and makes its implementation
choices explicit through a runtime identity. It does not adopt the private
`spike/jsonataddl` API or the spike's two-file and inventory-specific limits.

The extraction has four goals:

1. one reusable compiler and evaluator for the shared interface;
2. host-supplied dialect and source-layout policy instead of Tailapp names;
3. one value model at JSON, logical-type, and SQLite boundaries; and
4. a composed identity that detects every semantic component change.

It is not a general stream processor. It adds no event store, arbitrary fold
graph, stream join, scheduler, cross-application fold read, native-helper ABI,
remote service, or new Tailapp source syntax. Tailapp retains its fixed
normalizer-to-private-event-to-analytics topology.

## Boundary

The library owns the deterministic application semantics:

- source-set validation through a configured layout;
- DDL parsing, schema compilation, topology validation, and compatibility
  comparison over logical application definitions;
- JSONata compilation, admitted-subset checks, evaluator bounds, evaluation,
  and strict result decoding;
- logical types, JSON-safe values, exact-integer and finite-number rules, blob
  representation, and conversion between logical values and SQLite values;
- fold-read grammar, parameter binding plans, cardinality and ordering checks,
  and the default-deny SQLite authorizer policy used when those reads execute;
- event, fact, emitted-event, and row-change validation, including declared
  read/write/emission authority and per-evaluation limits; and
- canonical source digesting and the core portion of runtime identity.

Tailapp remains the host and owns:

- OTLP/HTTP reception and OTLP-to-canonical-record conversion in
  `internal/ingest`, including resource, scope, signal, and envelope meaning;
- inbox positions, delivery obligations, deletion, backpressure, and gaps;
- the exact two-stage transaction: run one normalizer, apply its changes, feed
  its private events to analytic folds, then commit statistics and frontier;
- SQLite database creation, connection and transaction lifetime, migrations,
  projection activation, continue/reset policy, and crash handling;
- registry, drafts, exports, multi-namespace queries, CLI, MCP, and metrics;
- external query representation and Tailapp-specific diagnostics; and
- the Tailapp dialect configuration described below.

The core may validate and prepare a deterministic mutation plan, but the host
executes that plan in its transaction. The plan contains only declared table,
operation, column, and converted logical values; it cannot update platform
metadata or choose transaction boundaries. Likewise, the core supplies a
default-deny authorizer callback and an immutable read plan, while the host
supplies the already-open SQLite transaction. This keeps replay semantics in
the core and database ownership in the host.

The public core API must not expose the concrete JSONata AST or the Tailapps project's
projection types. Compiled applications are immutable handles with inspection
data, evaluation methods, runtime identity, compatibility information, and
read/mutation plans.

## Dialect and source layout

Today `internal/profile` hard-codes `otlp_record`, `otel_event`, the
`otlpScalarFields` envelope, and `application.sql` plus `folds/*.jsonata`.
These become an immutable `Dialect` supplied to the compiler. In conceptual
form it contains:

```text
Dialect
  identity                 stable name and version
  source layout            definition path; program root and suffix
  host event               name and typed scalar envelope
  private event            required name and declaration policy
  topology                 admitted program kinds and event flow
  authority                read/write/emission ownership rules
  limits                   source, input, output, depth, range, rows, events
```

The source loader receives a `SourceLayout`; it never tests literal paths.
The compiler receives event and topology declarations; it never tests literal
event names. Parameter validation uses the configured host-event fields or the
compiled private-event schema. Diagnostics may include configured names, but
no Tailapp name is a special case in core code.

Tailapp supplies a versioned dialect with:

- definition `application.sql` and programs `folds/*.jsonata`;
- host event `otlp_record` with `id`, `signal`, `name`, `source`,
  `time_unix_nano`, `observed_unix_nano`, `trace_id`, `span_id`, and
  `content_digest` as readable scalar envelope fields;
- exactly one declared private event named `otel_event`;
- exactly one normalizer from `otlp_record` that may emit only `otel_event`;
- one or more analytic folds over `otel_event` that cannot emit events; and
- the Tailapps project's single-writer and read-visibility rules.

This configuration preserves the Tailapps project's deliberately bounded two-stage
pipeline. Parameterization is not permission to build an arbitrary dataflow
graph: the core admits only topology policies it implements, and the Tailapp
host selects this closed policy.

## One value model

Value rules currently appear independently at three product boundaries:
OTLP canonicalization in `internal/ingest/canonical.go`, fold validation and
JSON decoding in `internal/profile/evaluate.go`, and SQL result conversion in
`internal/query/query.go`. Projection read/write conversion adds a fourth
implementation of parts of the same rules. They agree in intent but can drift
on exact integers, non-finite numbers, JSON columns, booleans, and blobs.

The core will contain the sole logical value codec. It defines:

- JSON `null`, boolean, string, array, object, finite number, and integers in
  the exactly representable JSON range;
- lossless wrappers for values outside JSON's direct scalar model:
  `{ "integer_decimal": "..." }` and `{ "bytes_base64": "..." }`;
- validation against `TEXT`, `INTEGER`, `REAL`, `BOOLEAN`, `BLOB`, and `JSON`;
- canonical JSON encode/decode using number-preserving decoding; and
- logical-to-SQLite and SQLite-to-logical conversion by declared type.

Hosts still decide what their native input means. The Tailapps OTLP adapter, for
example, decides that an OTLP `int64` outside the exact JSON range uses the
integer wrapper. It must construct that value through the shared codec rather
than duplicating its bounds or wrapper spelling. The Tailapps query layer still
chooses its response envelope, but obtains each typed result value from that
same codec. Fold inputs, row changes, projection reads/writes, and query rows
therefore share one tested implementation.

## Composed runtime identity

A single repository-global string cannot describe a reusable core plus a host.
Every compiled application instead carries a canonical, ordered
`RuntimeIdentity` composed from independently versioned components:

```text
jsonata-ddl runtime identity
  core
    interface contract revision
    DDL and fold-read grammar revision
    JSONata implementation, version, admitted subset, and enforced bounds
    logical value codec revision
    SQLite adapter, authorizer policy, and relevant engine/driver versions
  dialect
    source layout, host/private event schemas, topology, authority, and limits
  host
    input canonicalization and envelope revision
    orchestration and transaction-visibility revision
    externally observable projection/query value revision
```

Each component has a stable key and value. Canonical serialization sorts keys,
rejects duplicates and unknown required components, and produces both a
human-readable descriptor and a digest. The full digest, not merely the core
version, enters the application revision digest and is stored with each active
projection. Changing any semantic component changes the effective revision.

This replaces `profile.RuntimeID`; it is not a string assembled ad hoc by the
host. The core constructs and validates the composite from its own component,
the selected dialect, the SQLite adapter, and explicit host components. Tests
prove that order does not matter and that every component change alters the
digest.

## Conformance corpus

The shared contract needs executable examples independent of either host. A
versioned corpus should contain directories with:

- a manifest naming the interface revision, dialect fixture, runtime
  components, and expected compile or evaluation outcome;
- the application source set;
- evaluation inputs and prior read rows;
- expected normalized logical output, mutation plan, or stable diagnostic
  code; and
- when relevant, a small initial SQLite state and expected final state.

Coverage must include accepted DDL and fold reads; rejected syntax and ambient
functions; exact-integer, floating-point, JSON, boolean, blob, and null edges;
read cardinality and total ordering; authorizer denial of undeclared tables,
columns, functions, schemas, PRAGMAs, and writes; insert/upsert/delete
validation; decision/fact/event limits; source and runtime digest fixtures;
and deterministic repeated evaluation.

Tailapp first runs the corpus against both its existing implementation and the
new core adapter. Gitseq may later run the same host-neutral cases plus its own
dialect cases. Host-specific OTLP, frontier, storage, and query tests remain in
Tailapp and are not conformance requirements for the core.

## Tailapp migration

Migration is incremental and preserves the ability to run existing active
projections:

1. Freeze current behavior as corpus cases and differential tests, including
   stable diagnostics where callers depend on them.
2. Introduce the Tailapp dialect, source-layout adapter, and composed identity
   types behind `internal/profile` without moving behavior.
3. Introduce the shared value codec and replace the copies in ingest,
   evaluation, projection, and query one boundary at a time.
4. Move the compiler, JSONata confinement and evaluation, fold-read policy,
   SQLite authorizer, and result/mutation validation into the core. Keep a
   narrow `internal/profile` adapter until all callers use core handles.
5. Switch compilation and projection identity to the composed runtime. This
   is a deliberate runtime-profile version change, even if differential tests
   show identical results.
6. Remove the adapter and duplicate rules only after corpus, differential,
   activation, upgrade, race, and end-to-end tests pass.

The upgrade must not reinterpret an active projection silently. The binary
retains a resolver for the current legacy `RuntimeID` so an old active
projection remains queryable and recognizable, while ingestion readiness is
held according to the existing upgrade rule. Revalidation under the composed
runtime creates a new revision. The operator then explicitly
continue-activates it when stored table shape is compatible, or resets it.
Continue preserves tables but begins the new runtime at the drained activation
boundary; no old delivery is replayed under new semantics.

The legacy resolver can be removed only under an explicit compatibility and
support policy. A cosmetic package move alone must not force data loss, and a
claimed behavior-preserving extraction must still change runtime identity
because implementation identity participates in deterministic interpretation.

## Security and failure properties

Extraction must preserve or strengthen the current confinement boundary:

- fold reads execute with a default-deny authorizer, not only a textual SQL
  check; the allowlist derives from the compiled read plan and schema;
- JSONata receives no ambient clock, randomness, filesystem, network, process
  state, dynamic evaluation, or undeclared helper;
- all encoded inputs, outputs, reads, emissions, facts, mutation counts, depth,
  ranges, and wall-time safety nets remain bounded and identified;
- compiled configuration and application handles are immutable and safe for
  concurrent callers; and
- core failures return stable categories so Tailapp can distinguish an
  ineffective decision from a projection gap without parsing prose.

The core does not open arbitrary files or databases, own goroutines, commit
transactions, or expose a general SQL execution method. Those constraints keep
the extracted library smaller than a resident application platform.

## Settled module boundary

The physical home is the independently versioned
`github.com/generalbusiness-ai/tailapps/jsonataddl` module in this repository.
Its module-path tags, CI, security checks, license, dependency manifests, and
conformance corpus travel with that module. Tailapps consumes the same local
source during development; external hosts resolve published versions. A
second host still supplies its own dialect, host adapter, runtime components,
storage and frontier semantics, and production hardening; it does not create a
second implementation of this core.

## Completion criteria for the later implementation

The extraction is complete only when Tailapp has no hard-coded source or event
names in core code, no repository-global runtime string, and no duplicated
logical value conversions; the exact current Tailapps topology is expressed by
its dialect; every fold read is enforced by the default-deny runtime
authorizer; old projections follow the explicit runtime-upgrade path; and the
shared corpus plus Tailapps' existing tests pass against the extracted core.
