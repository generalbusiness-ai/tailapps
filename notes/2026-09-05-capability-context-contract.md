---
date: 2026-09-05
status: Commissioned I7 decision candidate; no provider or runtime change is delivered by this note.
author: builder
rests_on:
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:8395e89beb21198ff629fb36b5770524eb72736c
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:007036be14f46653e931b2f75cc0434ca82d0cc8
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:ae3801cfd041232f9a87aa7d9e3cfb05f9066798
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:0bc9a5dcda410094a1c9b838086217ce0b2746b0
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:9a4c25edc70290e72a41fe7ce1c3774cd4debc44
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:0cf7f9765b5e7bf4a7a9cc50aa12095a30055c2a
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:130607b9b4598cba6618faf2290e914577a62a2b
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:7f8eea0b09e7e4e7a6725faf3701668b3e8b8db8
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:52e5fe63af0d3708040d02f925b73d8870f74af8
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:b0088c7108efe0e1e91ebe31ada8651a80f5b4b0
---

# Bounded capabilities and exact-record context

Add an optional, explicitly declared extension boundary to the shared core.
A host installs reviewed providers; an application selects their semantic
contracts and narrower limits. Programs receive only their declared functions
and context. Missing or unenforceable providers make the application
uninterpretable before replay. JSONata alone returns decisions and mutations;
the host alone owns the transaction and frontier.

The decision is commissioned by Hugh's request `8395e89b`. Its governing
requirements are sections 3 and 5 of the adoption note and the six rules and
eight gates of the stable-extensions note, both at immutable Gitseq source
`860ee61a07aa753dcbc2d50e74da2b7b6547625b`. Chess is the first consumer.
No Chess type, identity scheme, rules library or host import enters this core.

| Adopted rule | Enforcing part of this decision |
|---|---|
| Explicit | DDL declarations and the per-program AST allowlist |
| Identified by meaning | Immutable contracts and canonical registry digest |
| No ambient authority | Reviewed argument-only providers and separate verified context |
| Total within bounds | Pre-operation meters and admission proofs |
| Data, not effects | Validated results; JSONata output and host-owned transaction |
| Failures retain meaning | Typed domain refusals, fixed interpretation errors and rollback |

## Current evidence and the admission limit

Source is Tailapps `1eef3c1964642f691fff59d2a8ac2de10170d7ab`; runtime files
are unchanged from the request's `9280be8b`. I5's corrected input-contract
decision is now delivered in that main, but its proposed API is not implemented.
I7 design does not depend on I5 implementation. Integration must reconcile the
two decisions in one reviewed API, without silently treating either note as code.

Chess observations use source `e7a65612a8270b12f676a35e97c66aaf2a705a30`
and its pinned Gitseq host `7152e79a741e` and notnil/chess v1.10.0. The core
directory artifact at `96712c14` describes the unchanged application/evaluation
files; the newer exact-file artifacts describe compiler and identity changes.
The live exact `application.go` artifact `ca9066c5` names older `8c6d5e21`
bytes without the mutex, so it cannot support the serialization claim. The
directory basis is intentional. `evaluate.go` has no exact artifact. The
exact `dialect.go` and MCP-tools bases are historically stale but live and
match the inspected bytes; neither describes a superseded world. The missing
current exact application/evaluation pointers are provenance gaps: the directory
basis names their inspected bytes, but a carried wider pointer alone does not
guarantee that later exact-file succession makes this note flare. The heads
implementing these changes must publish their exact-file artifacts.

`jsonataddl/confine.go` admits nineteen fixed functions, refuses lambdas and
dynamic calls, and has no program extension allowlist. `compile.go` sets depth,
range and a 2,000 ms safety deadline. `application.go` serializes each compiled
expression; `identity.go` requires exactly nine components. `load.go` accepts a
runtime digest string without verifying its relation to installed providers.
These are the boundaries to extend.

The pinned JSONata revision `599f35f32e5f` has expression-local function
registration and per-evaluation bindings. A control against that exact module
called a bound function once, then refused it on a second invocation without
bindings. A second control returned an error containing a synthetic invitation
canary: the evaluator exposed that string. Provider errors therefore require
sanitization inside the adapter, before evaluator error formatting.
Both checks are in one executable attached to workroom artifact
`292dcd9949d624910a56f59cac0172a9960064a9`, with its JSON output. In a
Tailapps checkout containing the workroom history, retrieve them with:

```sh
git show 292dcd9949d624910a56f59cac0172a9960064a9:attachments/tail-i7-provider-control.go > /tmp/tail-i7-provider-control.go
git show 292dcd9949d624910a56f59cac0172a9960064a9:attachments/tail-i7-provider-control.json
cd jsonataddl
GOWORK=off go run /tmp/tail-i7-provider-control.go
```

The expected output records one successful call, refusal of the next unbound
call, and `raw_callback_error_contains_canary: true`. The canary is synthetic.

The evaluator's entry callback can count AST visits. It does not intercept
allocations or all work inside built-ins. Its existing depth/range/time limits
are not deterministic work and allocation meters. Neither a Go callback that
reports its own cost nor a timer around it supplies that guarantee.

Consequently, no production extension is admitted on the current evaluator.
The implementation first needs a narrowly maintained instrumentation patch to
this evaluator, covering its admitted subset and codecs. Preserve its language
and SQLite pin. A new exact evaluator pin is an essential, corpus-gated change
only after that patch proves the bounds below; no unreviewed replacement,
`replace` directive or timer-only fallback qualifies. If this cannot be done,
leave extensions unavailable and report the blocker. This is also the adoption
plan's existing production evaluation limit, not a new claim about old hosts.
Develop the patch in an isolated evaluator checkout with a temporary workspace
or module override for proof runs. Record that override in the evidence; it
must not enter the delivered module. Publish a reviewed immutable evaluator
revision and use its ordinary exact dependency pin for final admission tests.

## Declaration and immutable loading

Extend the existing focused DDL recognizers with `CREATE FUNCTION` and
`CREATE CONTEXT`, and program clauses `FUNCTIONS` and `CONTEXT`. Keep one
normalizer, one private event and the present fold ownership rules. Clauses
follow `USING`, precede `WRITES`/`EMITS`, and may occur once each in that order.
Existing declarations without these clauses remain syntactically valid.

```sql
CREATE FUNCTION digest_matches USING 'example.digest-match@1'
  MAX CALLS 1 MAX INPUT BYTES 8192 MAX OUTPUT BYTES 32
  MAX WORK 16384 MAX MEMORY BYTES 65536 MAX DEPTH 1;
CREATE CONTEXT actor_identity USING 'example.identity-at-record@1'
  MAX OUTPUT BYTES 1024 MAX WORK 4096 MAX MEMORY BYTES 65536 MAX DEPTH 17;

CREATE NORMALIZER normalize ON host_record
USING 'folds/normalize.jsonata'
FUNCTIONS digest_matches CONTEXT actor_identity
WRITES accepted EMITS private_event;
```

The identifiers and limits above illustrate grammar; they admit no provider.
All numbers are positive decimal integers, checked for overflow. Limits may
narrow the installed contract, never exceed it; memory and depth are mandatory
too. Context is invoked once per declared alias per program, before reads.
There are no provider-to-provider calls in this version. Bound declaration and
alias counts in the dialect; reject duplicates, unknown clauses, aliases over
64 ASCII identifier bytes and case-insensitive collisions with any built-in,
SQL function or application/platform name. No source may name code, a package
range, library, network endpoint or installer.

Add immutable `ExtensionRegistry` construction from host-owned entries and
`LoadApplicationWithExtensions(..., RuntimeIdentity, ExtensionRegistry)`.
The typed identity must contain exactly the registry component computed by the
core. The old loader admits only applications without extensions; it cannot
assert extension support through an arbitrary string. The extended loader
validates every declared provider, even if a branch never calls it, before
returning an application handle. Capture immutable entries and defensive
copies in that handle. No mutable global registration or hot swapping.

Each contract fixes kind (function/context), semantic identifier, argument
order and null rules, result shape, canonical codec, domain-result vocabulary,
all limits, work schedule, diagnostic version and conformance-corpus digest.
Its digest excludes Go symbols and source hashes. The host's reviewed admission
record separately binds the implementation build to that contract and its
enforcement proof. Claiming the digest is not the proof. An unsupported or
unreviewed implementation is absent from the admitted registry.

Use the existing logical scalars and a small closed shape description for
structured values: bounded arrays, closed objects and explicitly tagged unions.
Every node has a byte/element bound and explicit nullability; no recursive
schema references, executable validators or generic JSON Schema engine.
INTEGER retains the safe-integer rule, REAL is finite, BLOB uses the existing
tagged codec, and text is UTF-8. Structured values cannot contain functions,
undefined values, channels or host pointers. Share I5's scalar/closed-object
validation machinery when integrated, extending it only with bounded array and
tagged-union forms needed by contracts; keep opaque host JSON separate.

## Invocation, accounting and confinement

The host opens one event-scoped evaluation session on a verified record and
its exact immutable context snapshot. The session spans the normalizer and all
fold evaluations for its emissions. It owns total work/allocation counters and
fixed bounds; it cannot reset them between programs. An evaluation failure
latches the session failed. No later call can obtain a successful result from
it, and closing it releases all per-event data.

The proposed flow is `BeginEvent(token, snapshot)`, then
`session.Prepare(programName, meta, event)` before host reads, then
`session.Evaluate(prepared, rows)`. Preparation returns an opaque handle bound
to that session, program and immutable input; it validates base inputs and
resolves context. Evaluation validates the complete rows and consumes the
prepared handle once. A failed preparation also poisons the session. The
existing `Evaluate` remains available for extension-free applications; it
refuses an extension-enabled application rather than bypassing preparation.
I5's proposed `Application.ValidateProgramInput` also refuses that application:
`Prepare` subsumes its pre-read base-input validation, rejects caller context,
then inserts and validates the core-owned context before returning the handle.
The host verifies record/snapshot authenticity; the core checks their matching
token and handle ownership. The core does not pretend to verify Git signatures.

The core passes function bindings only for the selected program into the
existing expression evaluation, under its existing mutex. Bindings close over
that session, not a database, log or process context. Recheck the alias at each
call and accept only direct calls in the AST. For extensions reject partial
application, variable reassignment/shadowing, function-value references and
indirect apply forms; retain all current refusals for the rest of JSONata.
Return fresh owned values, never mutable provider state. Simultaneous sessions
must not share counters, arguments, results or context.
Use per-evaluation bindings for entry instrumentation too; `Assign` mutates the
cached expression's environment and would share a meter between sessions.
The pinned library also exposes package-global registration. The compile-time
allowlist keeps names outside the nineteen built-ins and selected aliases
unreachable; admission must separately prohibit replacement of those allowed
built-ins. No host may register an extension globally or alter that frame.

Before calling a provider, validate arity, complete shape, nulls, canonical
encoding, byte/collection/depth limits and the remaining budgets. Charge
encoding, decoding and validation too. The provider sees only an immutable
argument buffer, bounded scratch space and the trusted meter. Validate and
canonicalize its complete result before exposing it to JSONata. A result above
the bound is refused before an unbounded output allocation. No invocation
memoization is implemented: every executed call consumes a call allowance and
its work. This uses the stable note's optional-cache policy without introducing
cache-dependent accounting.
This requires deterministic dispatch from the exact admitted evaluator: no
common-subexpression elimination or speculative/repeated evaluation of a
logical call is permitted in its admitted subset. Gate 3 pins that behavior
with repeated identical call sites, conditional branches and bounded loops,
checking exact call/work counts and boundary judgments. Remove a dispatch or
repeat it and the corpus must fail. A future optimizing adapter must preserve
the adopted logical-invocation accounting through event-local deduplication;
it cannot inherit admission merely because its returned values match.

The enforcement contract is explicit:

| Resource | Check before doing the work |
|---|---|
| Calls | Decrement the alias allowance and the session's total call allowance before invocation; overflow or zero refuses. |
| Work | Charge one tick per AST dispatch, collection element visited and byte examined/copied by codec/string operations. Provider contracts add a fixed primitive schedule, such as one tick per examined board square or hash compression round. Charge before each primitive; include all loops and built-in work. The schedule and any weights are versioned. |
| Memory | Reserve cumulative allocation units before allocation, with fixed costs for scalar/node/container slots and bytes. Charge temporary buffers, growth copies, provider scratch, input/output codecs, frames and returned values. Freed storage does not refund the event allowance. Reuse of a bounded scratch arena is permitted only under its fixed reservation. |
| Recursion | Charge entry before a recursive call, release depth on every return/error, and refuse before exceeding the per-provider and evaluator limits. The event work counter bounds repeated shallow calls. |
| Aggregate limits | Every charge checks both the narrowed local bound and the event total. Integer counters use checked subtraction. A local failure cannot leave a usable session. |

Allocation units are a deterministic logical measure, not Go heap sampling.
The admitted implementation must also prove a finite physical working-memory
bound from those units: inventory every allocation path, require bounded
constructors/arenas, and fix conservative slot costs for supported builds.
An uncovered allocation, loop, callback or recursion path fails admission.
Static, finite non-recursive primitives may reserve their proven worst-case
cost once before entry; they cannot return a claimed cost after doing work.
GC behavior and process-wide allocator statistics are not semantic inputs.

Providers are reviewed trusted code, not arbitrary sandboxed Go functions.
The restricted interface alone cannot stop a malicious callback from using
globals, allocating or doing I/O. Admission includes source/dependency review
and instrumentation coverage; no native implementation is admitted on a
self-certified `Bounded` flag. Catch ordinary provider panics to a fixed error;
fatal process faults remain a limitation of trusted in-process code. The
transaction must remain recoverable after process death. WASM, subprocess
loading and third-party plugins are outside this decision.

Register no extension in SQLite in this version. The existing default-deny
read authorizer, writer seating/release and query pool remain unchanged.
SQL reads, query SQL, views, checks, defaults, indexes and triggers cannot call
an extension. A future internal SQLite adapter needs its own demonstrated
need and review; SQLite's extension ABI never becomes the public contract.

## Context is a verified host input

Use a separate host-owned context resolver, never a scalar function that hides
a history read. The evaluation session binds a token containing the verified
genesis, exact record ID, actor, position and signed timestamp. The host supplies
an immutable snapshot identified by the same record. Verify that the record is
present and all fields agree before resolution; an unknown record is an error,
even though the present identity library returns an empty result for it.
Application data cannot construct or override this token.

Resolve only the selected program's declared context aliases. The core inserts
their validated values under a reserved `meta.context` member before declared
reads and validation/evaluation. Reject a caller-supplied `meta.context`; a
program with no context receives none. Extend the compiled per-program metadata
shape, not the host's arbitrary input map. I5's declared base metadata remains
closed and excludes this reserved slot. Context does not become a SQL parameter
in the present `:event.scalar` read grammar.

All programs for one host record use the same record token. A private emission
does not become a new authority instant. Hosts keep existing base metadata
spellings: Inventory uses its frozen decimal strings, Tailapp its declared
fields. Future Chess base metadata must explicitly declare actor, timestamp
and ordered `rests_on` through its dialect; neither the core nor this context
API quietly adds them to other hosts.

The first identity provider belongs to Gitseq's host, not Tailapps. Its result
is the closed union `{anchored:false}` or the stable identity, scope, anchor
record and strength fields named by the stable-extensions note. Include
self-signed Nostr and witnessed identities already supported by the pinned
host. Identity equality is scheme plus stable subject; handle is display only.
Applications still enforce their own scope and strength policy. For current
Chess that means scope `chess` or `chess:<game>`, vouching Witnessed/SelfSigned
and verification LiveLookup/InLog; an unknown value grants no seat authority.

Resolve at position n with signed timestamp t: an anchor at n is visible at n;
a revocation at n removes authority at n; later records never alter the result
for n. Delegation inherits only the authority available at its own admission
position, and every ancestor must still stand at the queried position. Equal
timestamps do not collapse positions. Expiry matches the pinned host exactly:
`NotAfter == 0` is unlimited; otherwise `t > NotAfter` expires, equality does
not. Root withdrawal invalidates replayed copies of the same Nostr grant.
These rules must be differentially checked against the actual pinned host.

The current `identity.Resolve(log)` scans history, and `LookupAt` may scan an
actor's anchors. Wrapping that call does not prove bounded context work.
The host needs a metered incremental resolver or bounded indexed snapshot,
including decode, signature checks, anchor scans and parent traversal. Snapshot
preparation is accounted host work, not hidden outside the event budget.
Stage changes in the host transaction or an immutable discardable snapshot;
on failure leave the committed application/context frontier unchanged. A scan
budget exceeded is an interpretation failure, never `{anchored:false}` or a
truncated set of endorsements. Preserve the current parent-chain boundary
(the pinned code refuses when depth is greater than 16), including its exact
boundary tests; changing that semantic limit needs a new context identifier.

## Failure, diagnostics and identity

Publish fixed extension codes: `extension_unavailable`, `extension_contract`,
`extension_argument`, `extension_result`, `extension_calls`, `extension_work`,
`extension_memory`, `extension_depth`, `extension_context`,
`extension_provider`, and `extension_interrupted`. No raw provider error,
panic value, stack, argument or result is interpolated. Sanitize within the
callback and again at the extended evaluation boundary, including evaluator
errors that may quote a provider result. A machine deadline maps only to the
retryable interruption class; it is not deterministic exhaustion.

A valid domain refusal is ordinary typed data. JSONata decides whether to
turn it into an ineffective decision/facts. Any extension interpretation
failure returns no mutation plan, rolls back all normalizer/fold changes and
keeps the interpreted frontier before the record. The host may separately
record gap diagnostics; it must not publish a partial decision or advance
delivery. For extension failures, those diagnostics contain only the fixed
extension code. Today `internal/projection/projection.go`'s `recordGap` writes
`processErr.Error()` into `gap_reason`, and `internal/mcp/tools.go` exposes it
through `tailapp_status`. The host integration must sanitize this persistence
boundary as well as returned errors; retaining the raw-error path is forbidden.
Keep the existing timeout retry distinction using the typed error class.

The disposable invocation relation stores only event/program/alias, semantic
identifier, argument/result digests, encoded sizes, consumed work and fixed
code. It is host-owned, excluded from business output and separately bounded
by event call limits. Failure telemetry is published after rollback; it cannot
commit business writes. No raw values enter logs, HTTP/MCP errors or this
relation. Digests are not a claim of secrecy against guessing low-entropy input;
retain the host's existing access policy. Do not add exported metrics labels
containing argument digests.

Add exactly one required identity component, `extensions`, computed by the
core over the entire admitted immutable registry (functions and contexts),
its contract digests and enforcement/codec version. The explicit empty registry
has a stable digest; there is no absent-key default. Installing even an unused
provider changes this conservative identity. Provider source/build attestations
remain evidence, while a changed result, cost schedule, bound or context meaning
requires a new semantic identifier. A second implementation can share an
identifier only after proving the same outputs and cost semantics.

Canonical registry encoding is versioned JSON with fixed field order and
escaping, sorted semantic identifiers and sorted object members; argument
order and tagged-union alternatives have explicitly fixed order. Include
all limits, shape/null rules, corpus digest, work schedule and fixed failure
vocabulary. Reject duplicate identifiers, malformed digests and cyclic shapes.
The component value is the scheme-prefixed SHA-256 digest of this canonical
encoding, following `DialectComponent`, never the JSON itself; it must satisfy
`ComposeIdentity`'s existing delimiter restrictions.
The application source revision already binds aliases, narrowed limits and
program allowlists; the dialect canonical form additionally binds extension
policy and event totals. The loader verifies all three agree.

The component set becomes ten. Update composed descriptor/digest fixtures and
all host constructors explicitly. Bump `core.interface`, `core.grammar` and
`core.jsonata` for the new API, syntax and metered evaluator, plus the dialect
and host orchestration components where their behavior changes. Keep unchanged
SQLite and value-codec components unless the implementation changes them.
Crossing runtime identities requires a fresh acknowledged projection reset,
with I5's stored-identity guard also protecting continuation. No module release
may activate extensions on old unguarded hosts. Preserve old stored identity
recognition without restoring a historical evaluator.
Adding extension policy and event totals to the dialect canonical form changes
every existing host's dialect digest, even with an empty registry. Together
with the tenth component this requires the same acknowledged reset; preserving
extension-free program behavior does not preserve its old runtime identity.

## Delivery and admission gates

Builder owns core and Tailapp integration; Checker independently reviews each
exact head. Gitseq's host maintainer owns identity snapshot enforcement, and
the native Chess implementer owns its providers and adoption. These are later
bounded requests, not assignments made by this note.

1. **Meter proof and core:** prove the evaluator patch, codecs and one small
   non-domain fixture provider under the tables above; then implement immutable
   loading, DDL/AST allowlists, session accounting, shapes, identity and fixed
   diagnostics. I5 validation may integrate here if available; otherwise build
   the equivalent reserved metadata validation explicitly. Update core README,
   DDL reference and corpus contract in the same head. No production provider
   is enabled by the fixture. The patch, source pin, licenses and complete
   baseline/misbehavior corpora need independent review.
2. **Host and release:** wire event sessions, context preparation, rollback,
   telemetry, ten-component identity and protected reset into Tailapp's host;
   update activation/upgrade docs, including `resident-upgrade.md`. Retain empty
registry behavior for existing apps. After all gates pass, separately review
   and publish the nested module, verifying immutable public resolution and
   both hosts' conformance at its exact pin.
3. **Providers and host context:** separately design/admit bounded primitives
   and Gitseq's exact-record resolver. The current notnil/chess v1.10.0 engine
   and whole-log identity resolver are **not admitted**. The rules engine
   allocates move/history slices without this meter; history can grow. Prove
   bounded metered operations and state limits, stable outcome/method mapping,
   legal-move order and refusal values before assigning a contract identifier.
   Do not label library `Method().String()` a stable vocabulary. Input/history
   exhaustion must not masquerade as an illegal move. No second implementation
   is currently evidenced; gate 5 is conditional, not waived for future ones.
4. **Chess refresh/adoption:** after I7 providers pass and I9 is delivered,
   refresh the native Chess design against its current source, including forge
   confirmation, custody and seat authority. Preserve those host boundaries.
   Freeze complete normalizer inputs and differentially replay decisions,
   refusals, seats, move chains, FEN, outcome/method, materializations and tied
   identity histories. Set row and state bounds from demonstrated worst cases.
   Any new refusal/exclusion needs an adopted decision before binding changes.
   Leave the current native application running until this separate gate passes.

The eight adopted gates have concrete checks, each with a positive control:

| Gate | Evidence and an omission that must turn it red |
|---|---|
| 1. Program allowlist | Two programs share an application, only one declares an alias. The other cannot call, reference, shadow or indirectly apply it; a new session cannot inherit it. Remove per-program filtering and observe the unauthorized invocation. |
| 2. SQL confinement | Attempt calls from declared reads, query connections, views, checks and schema DDL. All refuse while the allowed JSONata call succeeds. Mutate query registration/authorizer isolation and demonstrate the forbidden call becomes executable. |
| 3. Failure atomicity and bounds | Missing/wrong-signature providers, malformed args/results, panic and each local/aggregate budget crossing leave all tables and frontier unchanged after a staged normalizer write. At-bound succeeds, one-over refuses before the next operation/allocation. Remove each charge or rollback guard and observe actual extra work/allocation or leaked rows/frontier. Include builtin loops/codecs, context preparation, overflow and error-then-next-session reset. |
| 4. Domain refusal | A typed refusal reaches JSONata and produces the expected ineffective result, without converting provider failures into decisions. Mutate failure-to-refusal mapping and compare exact decision-row presence/absence. |
| 5. Equivalent implementations | When a second implementation is supported, run both over identical conformance vectors and boundary inputs; require identical canonical outputs, fixed failures and deterministic cost accounting. Different provider implementation with the old contract but changed meaning must fail admission. |
| 6. No secret diagnostics | Canary in args, malformed results, nested fields, provider errors/panics and evaluator errors is absent from returned errors, logs, stored gap_reason, HTTP/MCP (including tailapp_status after reopen) and invocation rows. Remove inner, outer or persistence-boundary sanitation and reproduce the leak; the pinned raw callback control already demonstrates one path. |
| 7. Exact-record context | Anchor/delegate/revoke and root-grant replay at tied timestamps; scope mismatch, unknown strength, expiry equality and one-second-over, chain boundary, unknown/mismatched record. Compare pinned host results, then remove position/token/ancestor checks and observe unauthorized seat authority or false unanchored data. |
| 8. Provider change | Remove or change a declared provider and reject loading before any replay write; an unrelated unchanged registry still loads. Mutate registry/digest verification and demonstrate silent partial replay would otherwise occur. |

Architecture review covers the compiler/evaluator and the host profile,
projection and activation layers. This note changes no implemented contract.
Every implementing head that changes those contracts must update their
reference documentation and publish its exact candidate artifacts. Approval of
this decision requires the commissioned request; implementation requires its
approved sealed delivery and satisfaction, on current authority bases.
