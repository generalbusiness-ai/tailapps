# jsonataddl

`jsonataddl` is a reusable JSONata-with-DDL application core. It compiles a
bounded declarative application and evaluates its programs behind a
host-neutral boundary. Hosts supply their own dialect, runtime identity,
source loading, storage, and transaction orchestration.

## Install

The module is published from this repository with module-path tags such as
`jsonataddl/v0.1.0`:

```sh
go get github.com/generalbusiness-ai/tailapps/jsonataddl@v0.1.0
```

Consumers resolve the versioned module directly, without a workspace or local
replacement. See
`ExampleLoadApplication` for a minimal compile-and-evaluate path.

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
  component changes. Since migration stage 5 it replaces the
  repository-global runtime string on the live path: the core names its own
  components (`CoreComponents`), the host adds its dialect and host
  components, and the composed digest is the runtime identity new revisions
  and projections record.
- `values.go` — the sole logical value codec: JSON-safe values, exact
  integers, finite numbers, lossless integer and bytes wrappers, per-type
  validation with corpus-frozen diagnostics, and the conversions between
  logical values and SQLite values. All four Tailapp boundaries (ingest,
  evaluation, projection, query) delegate to it since migration stage 3.
- `load.go`, `compile.go`, `confine.go`, `evaluate.go` — the extracted
  behavior of migration stage 4: source loading through the configured
  layout, DDL compilation and topology/authority validation, the JSONata
  admitted-subset confinement, bounded evaluation, and strict result and
  mutation-plan validation. Everything is parameterized by the supplied
  `Dialect`; the runtime identity string is a caller input, never a core
  constant.
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

Repository maintainers release only a commit already merged to `main`, using a
module-path tag such as `jsonataddl/v0.1.0`. The Tailapps root module requires
`jsonataddl` v0.1.0 behind a local replacement, so `jsonataddl/v0.1.0` must be
pushed before any later Tailapps root release tag.

The module is Apache-2.0 licensed; see `LICENSE` and `NOTICE` in this directory.
