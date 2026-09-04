# jsonataddl

`jsonataddl` is a reusable JSONata-with-DDL application core. It compiles a
bounded declarative application and evaluates its programs behind a
host-neutral boundary. Hosts supply their own dialect, runtime identity,
source loading, storage, and transaction orchestration.

## Install

The module is published from this repository with module-path tags. The JSON
storage correction targets `jsonataddl/v0.1.2`; publish that tag only after
the source is merged and verified:

```sh
go get github.com/generalbusiness-ai/tailapps/jsonataddl@v0.1.2
```

Consumers resolve the versioned module directly, without a workspace or local
replacement. See
`ExampleLoadApplication` for a minimal compile-and-evaluate path.

Do not use v0.1.0. Its published module bytes conflict with the immutable
public checksum-database record for that version, so a normal Go consumer
correctly rejects it.

## Contents

- `dialect.go` — the complete host policy contract: identity, source
  layout, the typed host-event envelope, private event policy, topology
  cardinalities, read/write/emission authority, and evaluation limits,
  plus the Tailapp dialect value. Dialect values have value semantics, and
  the whole semantic configuration is mechanically bound to the composed
  identity through a canonical digest. Core code receives these; it never
  hard-codes a host name.
- `identity.go` — the composed runtime identity: independently versioned
  core, dialect, and host components with canonical ordered serialization,
  a human-readable descriptor, and a digest that changes when any semantic
  component changes. It replaces a single host-wide runtime string with an
  identity composed from every semantic participant: the core names its own
  components (`CoreComponents`), the host adds its dialect and host
  components, and the composed digest is the runtime identity new revisions
  and projections record.
- `values.go` — the sole logical value codec: JSON-safe values, exact
  integers, finite numbers, lossless integer and bytes wrappers, per-type
  validation with corpus-frozen diagnostics, and the conversions between
  logical values and SQLite values. A host uses this codec at its ingest,
  evaluation, projection, and query boundaries.
- `load.go`, `compile.go`, `confine.go`, `evaluate.go` — source loading
  through the configured layout, DDL compilation and topology/authority
  validation, JSONata admitted-subset confinement, bounded evaluation, and
  strict result and mutation-plan validation. Everything is parameterized by
  the supplied `Dialect`; the runtime identity string is a caller input,
  never a core constant.
- `application.go` — the immutable compiled `Application` handle:
  inspection accessors that return independent copies, evaluation, read
  plans, and continue-compatibility.
- `authorizer.go` — the default-deny SQLite read authorizer derived from a
  program's compiled read plan and the schema; the host installs it on the
  connection that executes the plan.
- `corpus/` — the host-neutral conformance corpus and its format
  (see `corpus/README.md`).

The module owns the complete compile-and-evaluate behavior. Host adapters may
retain compatibility resolvers or value conversions, but they consume this
same implementation rather than carrying a copy.

## Trust boundary

Logical `JSON` table columns compile to SQLite `JSON_TEXT` (TEXT affinity).
Hosts execute the core's `SchemaSQL`/`Table.SQL` unchanged and pass SQLite's
declared column type to `ReadRowValue` or `LogicalColumnValue`; both recognize
this physical spelling as logical JSON. Table and export metadata still say
`JSON`, and source DDL must use that logical spelling. JSON query numbers now
remain `json.Number`, including within objects and arrays, matching declared
read values and preserving their exact re-encoded representation.

`SQLiteLogicalType` maps a driver-declared type to the logical type for public
query metadata. `SQLiteJSONType` names the required physical JSON declaration
for hosts inspecting existing databases before continuation. Neither admits
the physical marker in application source DDL. A current-compiled prior
handle alone is insufficient evidence of the stored schema after recovery.

This correction versions `core.grammar` as `ddl/2` and `core.value-codec` as
`logical-values/2`. Old `JSON` storage has NUMERIC affinity and can lose scalar
data. Its stored shape and digest differ, so it cannot be continued into the
new shape. A host must honor the changed runtime identity and require an
explicit safe upgrade; recompiling sources is not permission to reuse an old
database. This module does not activate or migrate projections.

The module validates sources, confines JSONata, enforces deterministic input,
output, depth, range, event, fact, and row-change bounds, and supplies a
default-deny SQLite read authorizer derived from each compiled read plan. It
does not open host files or databases, own transactions or goroutines, expose
ambient clock, randomness, network, or process state, or execute mutation
plans. A host must install the authorizer on the connection used for reads and
must apply validated mutation plans inside its own transaction boundary.

## Develop

From the module directory, run without workspace influence:

```sh
GOWORK=off go mod tidy
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
```

The tests include the versioned `corpus/v1` compile, evaluation, logical-value,
SQLite-read, and default-deny authorization cases. The module's production and
test code has no dependency on packages outside this module.

## Release ordering

Repository maintainers release only a commit already merged to `main`. Before
pushing a module-path tag, run the public-record preflight for the intended
version:

```sh
scripts/check-jsonataddl-version-available.sh v0.1.2
```

Only a result that verifies the exact tag is absent from `origin`, the exact
version is absent from `proxy.golang.org`'s version list, and `sum.golang.org`
has no record permits the tag push. Do not probe the proxy's version-specific
`.info` endpoint before publication. The v0.1.1 preflight did so and its 404
remained cached for 30 minutes after the tag and release workflow succeeded.
Missing tools, network failures, malformed or ambiguous responses, and
existing records all refuse publication. The Tailapps root module requires
`jsonataddl` v0.1.2 behind a local replacement, so `jsonataddl/v0.1.2` must be
pushed before any later Tailapps root release tag. The protected
`jsonataddl/v*` tag namespace prevents published tags from being moved or
deleted.

The module is Apache-2.0 licensed; see `LICENSE` and `NOTICE` in this directory.
