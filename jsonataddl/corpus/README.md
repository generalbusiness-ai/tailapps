# jsonata-ddl conformance corpus

This corpus freezes the JSONata-with-DDL application semantics defined by the
shared interface contract (Gitseq's
`notes/2026-08-26-jsonata-ddl-application-interface.md`) as executable,
host-neutral cases. It exists so the `jsonata-ddl-core` extraction
(`notes/2026-08-28-jsonata-ddl-core-extraction.md`) can prove behavior is
preserved: the corpus runs against both Tailapps' `internal/profile` and
the extracted core's adapter (`LoadViaCore`) through the same runner seam,
differentially, since migration stage 4.

Any deliberate semantic change regenerates goldens with
`go test ./internal/profile -run TestConformanceCorpus -update-corpus`;
the resulting diff is the review surface. A golden that changes without a
deliberate decision is a regression.

## Layout

Each directory under `v1/` is one case:

```
v1/<case>/
  manifest.json      what to compile and evaluate, and what to expect
  app/               the application source set (application.sql, folds/)
  inputs/            evaluation inputs ({meta, event, rows} JSON)
  expected/          golden outcomes, written by -update-corpus
```

`manifest.json` fields:

- `interface`, `dialect` — the contract revision and dialect the case
  targets. Every current case uses the Tailapp dialect (`application.sql`
  plus `folds/*.jsonata`, host event `otlp_record`, private event
  `otel_event`).
- `application` — the source-set directory, relative to the case.
- `compile.outcome` — `ok` or `error`. `ok` pins the compiled identity
  (`expected/identity.json`: revision, runtime profile, storage-schema and
  export-contract digests) and the runner asserts recompilation determinism
  over all four fields. `error` pins the exact diagnostic text
  (`expected/diagnostic.txt`) — diagnostics are part of the frozen surface.
- `evaluations[]` — each names a program, an input file, and an `expect`
  golden. One golden mechanism covers success and failure: a successful
  evaluation freezes its `EvaluationResult` JSON, a failed one freezes the
  exact diagnostic as an `ERROR:` line, and `-update-corpus` regenerates
  both, so any divergence in values or diagnostic text fails the freeze.
  `repeat: N` asserts N identical outcomes (determinism).
- `projection[]` — core-only cases over real SQLite state (the runtime read
  authorizer is enforcement the extraction adds, so there is no prior
  implementation to run them against). Each names a program and either a
  `sql` statement to attempt under that program's default-deny read
  authorizer (freezing the denial diagnostic or the returned rows), or an
  `event` plus `meta` to run the full host cycle: execute the compiled read
  plan under the authorizer over the `state` fixture, evaluate, freeze the
  result (`expect`), apply the validated mutation plan, and freeze the
  final table state (`final_state`).

## Current coverage

- `basic` — a well-formed two-stage application: compile identity fixture;
  effective and ineffective normalization; value edges (the exactly
  representable JSON integer bound, floats, booleans, base64 blobs, JSON
  columns, nulls in optional columns); fold accumulation with and without a
  prior row; deterministic repetition; unknown program.
- `misbehavior` — compile-clean programs whose outputs violate runtime
  authority and shape rules: undeclared event emission, fold event emission,
  undeclared table writes, ineffective decisions carrying outputs, the facts
  limit, unknown result fields, undeclared/missing columns, wrong and
  unrepresentable values, and key-only deletes (accepted).
- `reject-*` — source sets the compiler must refuse: ambient/dynamic
  JSONata functions, syntax errors, unsupported logical types, multiple
  writers for one table.
- `projection-state` — fold-read execution over initial SQLite state
  (OPTIONAL ONE with and without a prior row, MANY with total ordering,
  a read through a declared view), mutation-plan application (insert,
  upsert, primary-key delete) with frozen final state, and default-deny
  authorizer denials: a sibling fold's table, an undeclared host table,
  SQL functions, PRAGMAs, and writes.

## Planned coverage

These categories join the corpus at the migration stage that moves their
enforcement point, per the extraction note:

- composed runtime-identity component fixtures, replacing the single
  runtime-profile string pinned today.
