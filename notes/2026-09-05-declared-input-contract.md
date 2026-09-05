# Declare the complete JSONata input contract

Decision commissioned by Tailapp request `e919a5d70d7cd1429ff83d57e65f05fc570fb2aa`,
I5 of the adopted Gitseq shared-core plan at `860ee61a`. Basis: Tailapp
`9280be8b` / released `jsonataddl v0.1.2`, and Inventory's input freeze at
`12c1687b` (`docs/reference/input-contract.md`). Implementation follows this
note's independent approval and sealed delivery; this head releases nothing.

## Decision

Add a small, immutable input contract to `Dialect`. Describe `meta` for both
stages and the normalizer's complete host event. Keep the existing scalar
envelope as the only host fields eligible for SQL read parameters. Structured
fields are program inputs, never new SQL parameters. Analytic event shape
continues to come from the declared private event; read-result shape continues
to come from the existing read plan. Do not create a second declaration for
those contracts.

Use a closed set of forms, not a schema language:

| Form | Meaning |
|---|---|
| Scalar member | Existing TEXT, INTEGER, REAL, BOOLEAN or BLOB logical value; explicit required/optional and nullable/non-null flags. |
| String array | JSON array of strings, preserving order and duplicates. Empty is `[]`; null is distinct. |
| Closed scalar object | Exactly the declared scalar members, each with its own presence/type/nullability. No recursive object declarations. |
| JSON object | Object with arbitrary JSON-valued members; nested keys are deliberately opaque to this core contract. |

`meta` is a closed object of scalar members, including the empty object. The
host event is a closed object combining envelope scalars and separately named
structured members from the forms above. No references, unions, predicates,
regexes, callbacks, capability registry or new DDL syntax are introduced.
Application-specific meaning inside opaque JSON stays in the existing host
canonicalization component and its byte-level conformance fixtures.

Every contract states whether its root object permits null. A missing map
(`nil`, encoded as null) is not `{}`; no default values are synthesized.
Required means the key must exist. Nullable controls its value independently:
an optional non-null member may be absent but may not be present as null.
Unknown members refuse. An entirely unspecified input contract refuses at
compile/load time; it never selects the old unchecked behavior silently.
Constructors and accessors defensively copy nested member lists.

Validate `EnvelopeField.Type` at dialect admission: accept exactly the five
canonical uppercase scalar types above, and reject JSON and unknown spellings.
JSON is supported by the shared *column* value model, but is not a scalar
host read parameter. Use the structured declaration for a JSON input. Reject
duplicate/invalid names and collisions between scalar and structured members.
Adding an explicit presence flag is source-compatible with named struct
literals, but a caller still has to supply the new complete input contract.

## The two real hosts

| Input | Tailapp producer at `9280be8b` | Inventory freeze at `12c1687b` |
|---|---|---|
| `meta`, both stages | Required INTEGER `position`, TEXT `event_id`, TEXT `event_type`; non-null root and members. | Required non-null empty object; no members. |
| Normalizer scalars | The nine existing envelope fields, all present; existing nullability remains, including nullable time/trace/span strings. | The six existing TEXT fields, all present and non-null; position and timestamp remain strings. |
| Structured event members | Required non-null `record`, an opaque JSON object decoded with number preservation from canonical OTLP record bytes. | Required non-null `rests_on` string array; required non-null closed `payload` object with TEXT `id`/`sku` and INTEGER `qty`, all required and non-null. |
| Analytic event | Existing private `otel_event` declaration. | Existing private `inventory_event` declaration. |

Inventory retains its host-side recognized-schema check, signed canonical
payload-byte check, positive quantity rule, 8 KiB payload bound and exact empty
metadata. The input contract does not move admission after evaluation or grant
production replay. Array sorting, deduplication, timestamp conversion, absent
metadata and extra payload fields remain forbidden by that freeze.

Some existing core/examples and Tailapp analytic fixtures intentionally pass
partial metadata, an `emission_ordinal`, or abbreviated host events. Preserve
those input bytes and expected results with explicitly declared *fixture*
contracts, including optional members where the fixture permits omission.
A renamed-host fixture declares its own `body` member rather than inheriting
Tailapp's `record`. Do not pad fixtures with invented producer values or weaken
the production constructor merely to make abbreviated fixtures pass. Keep a
separate check that actual producer inputs satisfy the strict host constructor.

## Validation and identity

`Evaluate` resolves the program, serializes the input once, applies the existing
input-byte bound, and validates that same encoded representation before
invoking JSONata. Decode numbers with `UseNumber`; validate INTEGER/REAL using
the existing logical-value rules, BLOB as base64 text, and nested JSON without
coercion. Native Go integers therefore remain the JSON numbers they already
encode as; they do not become strings. Reject malformed or non-finite numbers,
out-of-range declared INTEGER values, and excessive nesting under the declared
depth bound. Opaque JSON numbers retain their existing JSON representation.
Pass the original encoded bytes to the evaluator after validation. Keep the
existing evaluator confinement and the unresolved production work/allocation
gate; this change does not claim to supply that gate.

Validate metadata for every program, the complete host event for a normalizer,
and the existing declared private-event shape for an analytic fold. Keep read
results under existing read-plan rules. Expose
`Application.ValidateProgramInput(programName string, meta, event map[string]any) error`
for the pre-read check. It resolves the program and applies the same encoded
metadata/event validator and byte/depth bounds, without accepting or querying
rows. Hosts call it before binding host values to a normalizer read query;
`Evaluate` repeats it internally against its complete encoded input
so direct core callers cannot bypass the input boundary. On failure the host
rolls back its existing record transaction and advances no interpreted state.

Version `Dialect.Canonical` to an unambiguous encoding of the whole dialect,
including these contracts. Use fixed-order JSON records with explicit fields
(no omitted defaults); sort object-member declarations by name, escape strings
with Go's `encoding/json` string encoding, and preserve semantic array order.
Include kind, presence, nullability, closed-object member types, root-null
policy and a fixed input-semantics version. Construction rejects duplicates
before encoding. Equivalent declaration order has one identity; every semantic
change has a different identity. Existing limits remain included. Both built-in
host dialect versions and the existing `core.interface` component change;
keep the closed nine-component runtime identity set. Input validation's fixed
semantic version is part of the dialect encoding, without a new component key;
record their exact new composed digests in conformance fixtures.

This mechanically binds declared shapes, not arbitrary producer code. Keep
both hosts' canonicalization components and input-byte goldens load-bearing.
An opaque JSON object's inner semantics cannot change under an unchanged host
component merely because its outer kind still says object.

Old compiled handles retain their old semantics; never mutate or relabel one.
New hosts accept only handles with their expected composed runtime identity.
Stored projections with the old runtime refuse continuation under the new
handle; require an explicit discard/replay into a fresh projection. Inventory
also requires an explicitly authorized new Gitseq binding before any activation.
No automatic rewrite of a binding, signed record or stored value is allowed.

## Delivery gates and owners

1. **Builder / checker:** implement and independently review the core contract,
   admission, validation and canonical identity with the Tailapp producer
   integration. Keep pinned JSONata/SQLite, two stages and host transactions.
2. **Builder / checker:** run the complete core and host suites, race and public
   module/corpus gates; publish a separately reviewed nested-module release.
3. **Inventory implementer / native reviewer:** adopt the released pin and exact
   declared freeze, rerun both corpus repetitions, input goldens and admission
   mutants, then deliver natively. Broader activation remains separately gated.

Tests must change every admitted contract component individually and observe
an identity change, while reordered object declarations remain identical.
Remove validation deliberately and prove that invalid scalar types, missing
required fields, unknown members, null/absence mismatches, malformed arrays,
wrong object shapes and invalid numbers cease to refuse. Preserve causal-array
order/duplicates and both hosts' exact valid bytes. Exercise read-plan and direct
`Evaluate` entry points, old-handle/storage mismatch, immutable copies and the
explicit fixture contracts. No module version is published from this note.
