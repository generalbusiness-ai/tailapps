# jsonata-ddl-core

The Tailapps-local home of the JSONata-with-DDL application core being
extracted from `internal/profile` per
`notes/2026-08-28-jsonata-ddl-core-extraction.md`. The shared semantic
contract is Gitseq's `notes/2026-08-26-jsonata-ddl-application-interface.md`;
this library adopts it behind a host-neutral boundary, with Tailapp as the
first host and Gitseq adoption deferred.

## Module layout decision

The library lives as a **package tree inside the Tailapps module**
(`github.com/generalbusiness-ai/tailapps/jsonataddl`), not a nested module,
for the duration of the migration: one module keeps the differential
corpus, the `internal/profile` adapter, and the core in a single gated
build while behavior moves boundary by boundary. A dedicated module (or
repository) with its own release lifecycle is the intended end state *when
a second host adopts the core*; carving it out then is a mechanical module
split, and doing it now would only add version-skew risk between the two
halves of a migration that must stay lock-stepped. Revisit at migration
stage 6 or on Gitseq adoption, whichever comes first.

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
  component changes. Replaces the repository-global runtime string at
  migration stage 5.
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

At migration stage 4 the core carries the complete compile-and-evaluate
behavior and the conformance corpus runs against both implementations
differentially through the narrow `internal/profile` adapter
(`LoadViaCore`); live compilation and projection identity still use
`internal/profile` until the stage-5 switchover.
