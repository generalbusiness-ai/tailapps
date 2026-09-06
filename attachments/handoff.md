# I7 adoption handoff

Candidate: [notes/2026-09-05-capability-context-contract.md@b20296dce4555a3bec5fa868f5fbbc9913f31266](notes/2026-09-05-capability-context-contract.md:1), from main7ecffd53013fd6ca45693a1a8e28b7c8d52432e8, under request3926. This is a decision refresh, with no production source, module or live-service change.

The current-source table links the implemented input contracts, validation before reads and again during evaluation, the existing nine-component identity, and physical stored-runtime continuation guards. It explicitly retains pre-allocation metering as future work because current input validation marshals first. Caller context remains rejected; the new reserved slot belongs to compiled per-program metadata.

Current exact-source reconciliation is recorded at #4012/8e44dc03. The ten comparison paths are byte-identical to their older live artifacts; none of those historical staleness flags or live predecessors was changed. Eleven exact current source pointers (including the first authorizer pointer) now support the decision, alongside existing live confinement, loader, value-codec, module-pin and upgrade-guide artifacts. External Chess/host/evaluator sources are linked at immutable full Git commits; their exact inspected source hashes and line checks are attached in doc-validation.json.

Adoption is the next step: Hugh files and ratifies an ordinary proposal for this exact candidate. Only after that will Builder request independent Architecture/Security/Simplification review resting on the fresh request, adopted proposal and candidate artifact. The old stale commission grants no implementation authority for this revision.

## Full-scope implementation handoff after decision delivery

Later implementation requires a separate request on the then-current adopted and delivered decision. Preserve all four stages:

1. Meter/core: a reviewed evaluator patch and immutable final pin; codecs and a small fixture provider; deterministic work/allocation/depth/call budgets with physical memory proof; immutable registry, DDL/AST confinement, bounded shapes, event sessions, one-use Prepare/Evaluate, ten-component identity and fixed diagnostics. Reuse the implemented input rules while preserving both checks. No timer-only or self-reported-cost admission.
2. Host/release: event context preparation before reads; rollback and frontier atomicity; inner/outer/persistence sanitation; bounded telemetry; explicit identity migration and protected reset. Separately review and publish the nested module only after its gates and exact public resolution/host conformance pass.
3. Providers/exact-record context: separately admit metered domain primitives and bounded Gitseq identity snapshots, including position, token, scope, strength, expiry, revocation and parent-chain comparisons. Current unmetered notnil/chess and whole-log Resolve are not admitted.
4. Chess adoption: after admitted providers and I9, refresh native design and differentially replay full inputs, decisions, refusals, seats, move chains, FEN, outcome/method and materializations. Preserve custody/forge/seat boundaries; new exclusions require adoption before binding changes.

All eight admission gates and their positive/omission controls are byte-for-byte unchanged from the delivered prior note: program allowlist; SQL confinement; failure atomicity/bounds; domain refusal; equivalent implementations when supported; no secret diagnostics; exact-record context; provider change. No gate is replaced with a timing-only check or a claimed cost.

Validation: 33 exact source links resolve to inspected files and valid lines; only the note changed; four numbered stages retained; eight-gate table unchanged. Existing six focused core input/identity tests and three host runtime/upgrade tests pass. The pinned JSONata control was reproduced: exactly one scoped call, next unbound call refused, synthetic raw error canary exposed. A fresh CLI build and diff checks pass. These are current-baseline checks, not claims that future I7 admission gates have been implemented or passed.


Correction to requester4030 and review4029: local source links use the owning artifact head, preserving its immutable context. The source-provenance attachment retains exact earlier revisions and byte comparisons. All19 local source links, including the newly precise activation check, are to be verified through this reader. The three normative tables and all four stages remain byte-identical. Prior33-link evidence was an offline Git/module check, not a claim of in-app preview acceptance. No old approval covers this changed head.
