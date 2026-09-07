---
date: 2026-09-05
status: Adopted I7 baseline, callable, group-order and tuple-value amendments; object-key probe amendment pending ordinary adoption. No runtime change is delivered by this note.
author: builder
rests_on:
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:25fb87520cd4dec29a01623a19d02707cea50925
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:46ea04c2b168dd54c173579532c593fcf6f0a8d9
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:4357cae21cccb883131fe9ad679696ad14ac14a5
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:5f18c7a25e3db4273f34173d61ef343a4a23561e
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:155ce10aab90af9a224f36e9893180c1a5e08e1f
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:d57c7611630a752c260c48c7300007a14070e735
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:4b35df409937323e350d2312ab7e78418a4ec624
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:8481b0930148b2ac6452bd2deb42f4d3eddf68d2
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:c559f37cda0ea323f61704ebb32369dd583e04fd
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:eecb4b4b5ec6ccf8760574971c1a236a0fe8e8a9
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:e0a136b5d39cd816e2ddba7eb0ce79e3094b549d
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:c6004de1a348d21a47de8819e8459abbc0c20014
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:5f053189530b7fa137bf307edc2967f8f6cbd08c
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:97fc3ede2cbdb5154fdd9fe4c656b160bfa401dc
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:61abf2ec8d2bd831ec6f73864387c70f2cef65b2
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:624d1573104bb80410494f0e862c9b23b69c6c06
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:ae3801cfd041232f9a87aa7d9e3cfb05f9066798
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:007036be14f46653e931b2f75cc0434ca82d0cc8
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:7462d4f4f1998929f2ddc79832ba73b757e3ae6e
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:bff81b1bd1b944923392e76802f7e04bffdba647
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:4b34dd24e29560a022bc86073695eedd12260ffd
  - git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:43766b053958f896ca932ae65d3ec2055a154ada
---

# Bounded capabilities and exact-record context

Add an optional, explicitly declared extension boundary to the shared core.
A host installs reviewed providers; an application selects their semantic
contracts and narrower limits. Programs receive only their declared functions
and context. Missing or unenforceable providers make the application
uninterpretable before replay. JSONata alone returns decisions and mutations;
the host alone owns the transaction and frontier.

Hugh adopted the #3926 baseline through proposal #4033 and ratification #4034;
its published decision is #4040. Request #4043 and promise `5cfedb59` carry the
full stage-1 implementation. Hugh adopted the callable amendment at `67bc03e8`
through proposal `b9b7fb7c` and ratification `2b8cd0be`; Checker approved it in
`f087b14c`. It landed through receipt `fe1e8d8e` and publication `d57c7611` under
#4135. Its policy below is unchanged.

Hugh adopted the group-order amendment under #4144 through proposal
`43168a26` and ratification `501d65f9`. It landed at `3a593721` through receipt
`5396efab` and publication `4357cae2`. The tuple value amendment under #4182
landed at `756ce43a` through receipt `3c288b2a` and publication `727ab6a7` after
Hugh's adoption and independent review. Request #4228 now commissions the
object-key probe amendment below. It still needs an ordinary proposal and
Hugh's ratification before independent Checker review. A commission or technical
approval does not adopt policy. All four stages and eight admission gates
remain; these source amendments do not close the original I7 implementation.

Repository source links open at the immutable head of this note's owning
artifact. Its source-provenance attachment records the earlier exact revisions
and verifies that those files have identical bytes at this head.

The governing requirements remain sections 3 and 5 of the
[adoption note](https://github.com/generalbusiness-ai/gitseq/blob/860ee61a07aa753dcbc2d50e74da2b7b6547625b/notes/2026-09-04-tailapps-jsonataddl-adoption.md#L1) and the six rules and eight gates of the
[stable-extensions note](https://github.com/generalbusiness-ai/gitseq/blob/860ee61a07aa753dcbc2d50e74da2b7b6547625b/notes/2026-08-27-jsonata-ddl-stable-extensions.md#L1). Chess is the first consumer. No Chess type, identity scheme, rules
library or host import enters this core.

| Adopted rule | Enforcing part of this decision |
|---|---|
| Explicit | DDL declarations and the per-program AST allowlist |
| Identified by meaning | Immutable contracts and canonical registry digest |
| No ambient authority | Reviewed argument-only providers and separate verified context |
| Total within bounds | Pre-operation meters and admission proofs |
| Data, not effects | Validated results; JSONata output and host-owned transaction |
| Failures retain meaning | Typed domain refusals, fixed interpretation errors and rollback |

## Current evidence and the admission limit

The baseline is Tailapps `7ecffd53013fd6ca45693a1a8e28b7c8d52432e8`. Input-contract
implementation is complete; I7 extension functionality remains future work.
These exact source references distinguish the two:

| Current source | Implemented behavior and remaining integration |
|---|---|
| [Input admission](jsonataddl/input.go:10) | `ValidateProgramInput` validates metadata and the complete event before host reads. It marshals first, then checks encoded bytes/depth and declared values. It is not a pre-allocation meter. |
| [Input contracts](jsonataddl/input_contract.go:9) | Four closed forms: scalar, string array, scalar object and opaque JSON object. Base metadata accepts scalar fields only; normalizer event scalars come from `HostEvent`. These forms are not arbitrary bounded arrays or tagged unions. |
| [Dialect](jsonataddl/dialect.go:24) | Explicit host input contracts and limits are part of the dialect. Keep base metadata closed when adding compiled per-program context shapes. |
| [Evaluation](jsonataddl/evaluate.go:64) | `Evaluate` marshals and repeats admission with declared read results before evaluating. Preserve this second check. Output size is checked after evaluator allocation, so I7 still needs bounded encoding and construction. |
| [Compiled application](jsonataddl/application.go:76) | A mutex serializes evaluation of each compiled expression. I7 sessions must preserve isolation while supplying only their selected program's bindings. |
| [Confinement](jsonataddl/confine.go:20) and [compiler](jsonataddl/compile.go:448) | Nineteen allowed procedure spellings, with lambdas and other dynamic call syntax refused; depth/range limits and a 2,000 ms safety deadline. This name check does not confine builtin values: see the current callable evidence below. No extension allowlist or deterministic allocation/work meter is implemented. |
| [Loader](jsonataddl/load.go:1) and [identity](jsonataddl/identity.go:22) | Loading accepts a runtime digest string; identity has exactly nine components. Registry verification and the tenth component remain I7 work. |
| [Host read preparation](internal/projection/projection.go:518) | Both normalizer and fold paths validate before binding/performing reads. `Prepare` must retain this ordering and add verified context. |
| [Engine upgrade](internal/engine/engine.go:187), [activation check](internal/engine/engine.go:626) and [stored-runtime guard](internal/projection/projection.go:306) | Historical runtimes can be opened for queries but remain upgrade-pending. Engine activation and transactional continuation check the physical stored runtime; acknowledged reset is required across identities. |
| [Gap persistence](internal/projection/projection.go:428) and [MCP status](internal/mcp/tools.go:95) | Ordinary failures still persist `processErr.Error()` as `gap_reason`, exposed through status. I7 must sanitize the inner, outer and persistence boundaries. This is a future extension integration requirement, not evidence of a current extension leak. |

The [module pin](jsonataddl/go.mod:8) still selects JSONata `599f35f32e5f31297f8b2153b5da44ddbb330e28` and SQLite
adapter 0.35.3. The [resident upgrade contract](docs/reference/resident-upgrade.md:23) explains why source delivery and live
recovery are separate: compatible schemas do not override a stored runtime
mismatch. This note commissions no deployment or reset.

Chess is now at `b97c6a82ef7e3618721696f5a69efef13da10a79`. Its
[module pins](https://github.com/generalbusiness-ai/gitseq-chess/blob/b97c6a82ef7e3618721696f5a69efef13da10a79/go.mod#L6) still select Gitseq host `7152e79a741e6c9c277568a6aabec8e9b6cbd792` and notnil/chess v1.10.0.
Its [projection initialization](https://github.com/generalbusiness-ai/gitseq-chess/blob/b97c6a82ef7e3618721696f5a69efef13da10a79/chess.go#L196) still resolves the whole log, and its
[outcome mapping](https://github.com/generalbusiness-ai/gitseq-chess/blob/b97c6a82ef7e3618721696f5a69efef13da10a79/chess.go#L531) still uses `Method().String()`. Neither implementation is admitted by this
decision; the bounded provider and exact-record differential gates still apply.

The pinned JSONata revision has [expression-local registration](https://github.com/jsonata-go/jsonata/blob/599f35f32e5f31297f8b2153b5da44ddbb330e28/v206/jsonata.go#L529)
and [per-evaluation bindings](https://github.com/jsonata-go/jsonata/blob/599f35f32e5f31297f8b2153b5da44ddbb330e28/v206/jsonata.go#L352). A control against that exact module
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

The evaluator's [entry callback](https://github.com/jsonata-go/jsonata/blob/599f35f32e5f31297f8b2153b5da44ddbb330e28/v206/jsonata.go#L669) can count AST visits. It does not intercept
allocations or all work inside built-ins. Its existing depth/range/time limits
are not deterministic work and allocation meters. Neither a Go callback that
reports its own cost nor a timer around it supplies that guarantee.

Consequently, no production extension is admitted on the current evaluator.
The implementation first needs a narrowly maintained instrumentation patch to
this evaluator, covering its admitted subset and codecs. Preserve its language
except for the explicit callable-boundary, group-order, tuple-value and proposed
object-key probe compatibility changes below; preserve the SQLite pin.
A new exact evaluator pin is an essential, corpus-gated change
only after that patch proves the bounds below; no unreviewed replacement,
`replace` directive or timer-only fallback qualifies. If this cannot be done,
leave extensions unavailable and report the blocker. This is also the adoption
plan's existing production evaluation limit, not a new claim about old hosts.
Develop the patch in an isolated evaluator checkout with a temporary workspace
or module override for proof runs. Record that override in the evidence; it
must not enter the delivered module. Publish a reviewed immutable evaluator
revision and use its ordinary exact dependency pin for final admission tests.

## Callable-boundary amendment for adoption

At Tailapps `c9bdbd32792008e942727248c2cf9ecb30f7d2d8`, the
[confinement check](jsonataddl/confine.go:20)
examines procedure spelling, not the value bound to it. Thus direct
`$reverse([1,2])` refuses, but rebinding `$sum` to `$reverse` admits that same
outside-set implementation. Even `$exists($reverse)` admits a reference
without invoking it. Hugh independently reproduced these paths in #4135's
`callable-decision-evidence.json`. These are confinement/evaluator probes,
not evidence of host replay or production admission.

Retain exactly these nineteen builtin implementations by immutable callable
identity: `abs`, `boolean`, `ceil`, `contains`, `count`, `exists`, `floor`,
`length`, `lookup`, `lowercase`, `max`, `min`, `not`, `number`, `round`,
`string`, `substring`, `sum` and `uppercase`. Retain already-admitted aliases
and partials whose ultimate target is one of those implementations, including
nested rebinding. An allowed procedure name grants no authority to a different
implementation. Do not add a builtin or admit new dynamic syntax. Preserve all
existing syntactic and ambient-source refusals, and the stricter extension
rules: direct calls only, with no reference, shadowing or partial application.

Outside-set builtin values must not become visible through references,
containers, callbacks, partials or returned values. Refuse an explicit reference
that resolves to one, including a non-invoked reference such as
`$exists($reverse)`; substituting an absent value or another result is not a
refusal. Resolve actual builtin identity through lexical scope: an ordinary
local variable called `$reverse` containing data, or an object member named
`reverse`, is not that builtin. Do not implement this distinction by blanket
text matching; the existing ambient-source rejection remains unchanged.

Reject at compilation where resolution proves the violation. For admitted
paths whose values need runtime resolution, use a fail-closed, metered identity
guard before exposing the value or invoking its ultimate target. Meter alias
resolution, partial construction/application, container and callback traversal,
and every retained callable path under the same work, allocation and depth
rules as direct calls. Immutable callable identity carries no mutable session
state; preserve session isolation and prohibit package-global replacement.

This deliberately narrows the former preserve-language promise for accidental
outside-set access only. It does not claim that every previously admitted
program remains equivalent. The compatibility corpus must separate unchanged
allowed behavior from deliberate new refusals, with at least these fixtures:

| Source or path | Required outcome under the amended runtime |
|---|---|
| `($sum := $length; $sum("abc"))` | Preserve `3`; the target is the permitted `length` implementation. |
| `($sum := $substring(?,1); $sum("abc"))` | Preserve `"bc"`; an already-admitted partial retains its permitted ultimate target. |
| `($sum := $length; ($sum := $abs; $sum(-2)); $sum("abc"))` | Preserve `3`; the nested binding must not leak into its parent scope. |
| `($reverse := 3; $exists($reverse))` and `{"reverse":3}.reverse` | Preserve `true` and `3`; ordinary lexical data and object keys grant no builtin authority. |
| `$exists([$length])` | Preserve `true`; a permitted callable value may follow an already-admitted container path. |
| `($sum := $reverse; $sum([1,2]))` | New refusal before exposure or invocation, rather than the former `[2,1]`. |
| `($sum := $map; $sum(["aa","bbb"], $length))` | New refusal of the outside-set higher-order target, rather than the former `[2,3]`. |
| `$exists($reverse)` and `$exists([$reverse])` | New refusal of the non-invoked reference/container value, rather than the former `true`. |
| Direct `$reverse([1,2])`, existing forbidden dynamic syntax and ambient sources | Continue to refuse. |

Also cover nested outside-set rebinding, outside-set partials, callback and
returned-value paths, permitted controls and separate sessions. Remove static
resolution checks and demonstrate forbidden compile-time admission. Exercise
the dynamic identity guard independently and remove it to expose an actual
extra value or invocation. Remove metering on
each retained indirect path and demonstrate work/allocation beyond the bound;
a test that only observes a refusal is not sufficient coverage proof.

Revalidate stored programs under the new runtime before any replay write.
A refusal leaves the projection and frontier unchanged. Account for this
confinement change in I7's already-required `core.jsonata`, grammar, dialect
and corpus/version changes wherever affected. Keep exactly ten identity
components, old-identity recognition, stored-identity refusal and the separate
acknowledged reset/activation gate. Add no extra component or automatic
migration/reset. This source amendment authorizes no evaluator/module
publication, production admission, live operation, reset or release. After
exact adoption, all implementation and remaining stage-1 conditions stay on
request #4043 and promise `5cfedb59`.

## Object and group order amendment for adoption

The pinned evaluator's [group value loop](https://github.com/jsonata-go/jsonata/blob/599f35f32e5f31297f8b2153b5da44ddbb330e28/v206/jsonata.go#L2676)
ranges over a Go map after collecting groups. Current confinement accepts:

```jsonata
($x := 0; {"a": $x := 1, "b": $x})
```

Two independent runs of 1,000 identical executions each produced both
`{"a":1,"b":0}` and `{"a":1,"b":1}`. The reproduction and exact pinned source
are retained with requests #4139 and #4144. These observations establish the
existing variation, not probabilities or a determinism proof. They exercise
confinement and the evaluator, not host replay or production admission.

Preserve two phases, with this order:

1. Discover groups in input sequence order and, within each input, source
   key/value-expression order. Keep the existing preliminary object-constructor
   key checks, their evaluations and errors, and the subsequent key evaluation,
   except for the selected-extension key rule proposed below.
   Finish all grouping before evaluating any group's value.
2. Evaluate each distinct group's value once, in the order its key was first
   encountered during that discovery. Repeated production of the same key by
   the same key expression keeps its first slot and accumulates inputs in their
   existing sequence order. A different key expression producing that key
   retains the existing duplicate-key error in the discovery phase; it is
   neither overwrite nor a second value evaluation.

Keep existing undefined-key, per-item invalid-key-type and empty-input behavior,
lexical frames and nested-block scope. The tuple value amendment below
qualifies tuple/group container alias effects; the object-key probe amendment
qualifies preliminary key evaluation. Numeric-looking and Unicode
keys follow encounter order, not numeric or lexical sorting. This order governs
semantic evaluation; JSON object serialization and canonical encoding remain
separate. Preserve the adopted callable policy and all syntax refusals.

Use an ordered vector of group slots with a key lookup that finds an existing
slot. Add a slot only on first encounter, then traverse that vector for the
value phase. Do not use key-map iteration to choose semantic order. Charge key
lookup, order bookkeeping, group accumulation, growth, copying and traversal
before their work or allocation, and reserve recursive entry before evaluating
children. Outcomes, selected errors, work/allocation counts and failure
boundaries must all be deterministic for the same admitted inputs and limits.
This is the implementation strategy for the later I7 work, not delivered code.

This is an explicit compatibility exception: previously order-dependent
admitted programs acquire the single specified outcome. Preserve pure,
order-independent outputs and existing refusals. The implementation corpus
must cover every admitted grouping path that can affect values or budget
failure, including these controls:

| Case | Required result or preserved condition |
|---|---|
| Direct binding above | Always `a=1,b=1`. |
| Reverse those source pairs: `{"b": $x, "a": $x := 1}` after `$x := 0` | `b=0,a=1`; source order matters. |
| Repeat one key through the same key expression, interleaved with another key | One value evaluation per key in first-encounter order; retain grouped input sequence order and the original first slot. |
| Different key expressions collide | The existing duplicate-key refusal occurs during discovery, before group values run. |
| `($x := 0; {"a": ($x := 1), "b": $x})` | Preserve `a=1,b=0`: the nested block owns its binding. |
| Undefined or invalid keys; empty input; tuple/reduce and nested groups | Preserve key checks and scope rules; complete discovery before values. Preserve values except for the explicit alias exception in the proposed tuple value amendment. |
| Keys such as `"10"` then `"2"`, or Unicode keys in reverse lexical order | An order-sensitive value expression follows encounter order; sorting serialized fields proves nothing about evaluation order. |

Use positive controls that distinguish the chosen order. Intentionally reverse
or shuffle value traversal, or remove order tracking, and show a changed value,
error or budget boundary. Remove the relevant charges and demonstrate actual
extra work/allocation beyond the limit. Repeated Go-map executions without
observed divergence are not proof, and sorting only output fields is not a
semantic fix. Do not replace the retained evaluator contract with a blanket
grouping refusal.

Revalidate stored programs before replay writes; refusal leaves the projection
and frontier unchanged. Account for changed evaluator semantics and the corpus
in I7's existing `core.jsonata`, grammar, dialect and identity versions wherever
affected. Retain exactly ten components, historical identity recognition,
stored-identity refusal and separate acknowledged reset/activation. Add no
component or automatic reset/migration. After exact ordinary adoption, all
runtime implementation and remaining stage-1 conditions stay on #4043 and
promise `5cfedb59`. This source amendment authorizes no evaluator/module
publication, production provider admission, resident operation, reset or release.

## Tuple value amendment for adoption

Recommend **value-semantic accumulation**: grouping and tuple reduction must
not mutate their inputs or another field's value through a shared container.
Keep the value and nesting rules below; array backing capacity and Go-map
traversal must not choose a JSON result. This is one proposed choice, not an
adoption or delivered evaluator change.

### Evidence and the alternative

At examined Tailapps main `3a593721231326d589541113add606b270490e35`, the
[current confinement](jsonataddl/confine.go:20) and the
[pinned module](jsonataddl/go.mod:8) accept:

```jsonata
($w := [[0,1,2]];
 $rows := [{"a":$w,"b":$w},{"a":["A"],"b":["B"]}];
 $rows.a@$a.b@$b{"one":{"a":$a,"b":$b}})
```

The unchanged pinned [tuple reducer](https://github.com/jsonata-go/jsonata/blob/599f35f32e5f31297f8b2153b5da44ddbb330e28/v206/jsonata.go#L2706)
shallow-copies the first tuple and appends subsequent field values into those
borrowed arrays. Here `a` and `b` share a length-3, capacity-4 backing array.
Appending either field writes slot 3; the later field overwrites both visible
results. Two 1,000-execution public-evaluator trials produced all-A/all-B counts
136/864 and 148/852. Hugh independently reproduced 155/845 and 162/838.
Breakdown `5f18c7a2` (#4179) retains the confinement probe, logs, unchanged
reducer and source provenance as six attachments. These runs prove existing
variation, not outcome probabilities, future determinism or host admission.

The proposed result is exactly:

```json
{"one":{"a":[0,1,2,"A"],"b":[0,1,2,"B"]}}
```

The alternative is to traverse fields in a fixed order while retaining borrowed
array appends. Ascending `a,b` produces all-B; descending `b,a` produces all-A
for the capacity-4 control. Both produce separate A/B arrays at capacity 3 or
with independent backing arrays. Fixing traversal therefore still lets spare
capacity choose the result and can overwrite a caller's longer alias into the
same array. Reject that alternative. Choosing either historical winner would
encode an allocation accident into the language's JSON-value contract.

There is a second affected path: the pinned
[group value phase](https://github.com/jsonata-go/jsonata/blob/599f35f32e5f31297f8b2153b5da44ddbb330e28/v206/jsonata.go#L2676)
reduces a single tuple by returning its original map, then deletes `@` from it.
The new rule extracts context without deleting a member of an input tuple.
An unchanged function body alone is therefore insufficient compatibility proof.

### Complete value rule

Process input tuples in encounter order. Field processing within a tuple uses
ascending raw UTF-8 field-name bytes, without locale or numeric coercion.
This order governs reduction bookkeeping, copying and failure accounting;
it does not reorder source expressions, group discovery or group values.
Field values have already been evaluated. No callback runs while enumerating,
comparing or copying them.

For each field, presence is distinct from value: a present undefined binding
shadows its parent just as before; a missing field contributes nothing.

1. The first occurrence of a field supplies its value, with mutable containers
   copied under the ownership rule below. An absent field in a later tuple
   leaves that accumulated value alone. A field first seen in a later tuple
   follows the same first-occurrence rule. Tuple fields are already unique
   object members; this does not change object-construction duplicate rules.
2. On each later occurrence, if the accumulated value is an ordinary array,
   extend it by **one element**, the incoming value. Otherwise make a two-element
   array containing the accumulated value and the incoming value. An incoming
   array is one nested element, not a spread. Thus `1` then `[2,3]` gives
   `[1,[2,3]]`; `[1,2]` then `[3,4]` gives `[1,2,[3,4]]`; `[]` then `[]` gives
   `[[]]`. Three scalars `1,2,3` give `[1,2,3]`. Do not add a separate hidden
   accumulator tag that changes these existing array rules.
3. A singleton tuple retains its field values and context. Read `@` as the value
   expression's context; omit it only from the newly created lexical frame.
   Never delete it from an input or sibling map. Empty reduction input remains
   empty. The legacy helper's non-array pass-through, and ignoring non-object
   members in a nonempty reduction array, retain their value results; any
   mutable value passed onward still needs the ownership protection below.
   This does not broaden which shapes the compiler or tuple producer admits.
4. Same-key grouping retains the adopted first-encounter slot and accumulates
   inputs in encounter order. Keep the existing shape rule there too: an
   existing ordinary array gains the next input as one element; otherwise
   combine the two inputs into an array. Isolate borrowed containers before
   accumulation. This includes non-tuple grouping. Finish all key discovery
   before reducing/evaluating group values, once per group in first-encounter
   order. Different key expressions colliding still raise `D1009` during
   discovery. Undefined keys, `T1003` checks and preliminary constructor checks
   keep their existing evaluations, scope and selected errors, except for the
   selected-extension preliminary evaluation rule proposed below.
5. Apply these rules recursively in nested tuple/group evaluation. Retain the
   distinction between ordinary arrays and evaluator sequence/tuple wrappers,
   including their metadata and existing singleton collapse behavior. Do not
   reinterpret an array-valued field as another tuple stream. Repeated evaluation
   of the same expression and unchanged input must start from unchanged input
   values; independent sessions retain their existing isolation.

### Ownership and deterministic charging

Copy all mutable array, sequence and object containers recursively when a
field or group first adopts a borrowed value, and when appending an incoming
value. Preserve wrapper metadata. A shared source subgraph is copied separately
for each owning field/group; do not memoize across those ownership boundaries.
Nested containers must not provide a route back to an input or sibling value.
An accumulator may reuse its own capacity only while it and its mutable
descendants have one owner and no input, sibling or published observer can see
an append. Already owned descendants of that accumulator need not be recopied.

Give the first private copy canonical capacity equal to its length, ignoring
source backing capacity. A scalar pair starts at length/capacity 2. For an
append requiring length `n` beyond current capacity `c`, grow to
`min(B, max(n, max(1, 2*c)))`, where `B` is the admitted array-length bound;
check `n <= B` and compute the clamped doubling without overflow. Below that
capacity, retain storage. Reserve the entire new capacity, copying of the old
prefix, and incoming-value copy before growth; charge every retained-capacity
append before its write too. Private logical capacity follows this rule even
if the allocator reserves more physical storage. Allocation rounding and copy
work remain covered by the four-build bound proof. No source capacity, random
map order or allocator decision enters the logical charge schedule.

This avoids mandatory quadratic prefix copying for long repeated fields while
retaining deterministic work/allocation totals and selected failures for the
same values and limits. Sharing or publishing the accumulator ends permission
to mutate it; any later adoption by another owner requires the recursive copy
rule. The amendment does not permit borrowed append or an uncharged growth
optimization.

Immutable scalars, strings and approved immutable callable identities may be
shared. A callable's established lexical environment is not a JSON container
to clone: preserve lexical binding and same-session closure semantics under the
adopted callable policy. This amendment grants no new callable, input shape,
cycle, native value or host capability. Continue to reject inadmissible input
cycles at admission; recursive copying has balanced depth reservations and a
sticky bounded failure if a defective internal graph cycles. It cannot hang or
invoke a callback. Results never expose the private copying machinery.

Every enumeration, key comparison/lookup, slot insertion, array growth, scalar
or container copy, and recursive entry is prepaid under the event's existing
work, allocation and depth budgets. Check count/byte arithmetic and reserve
storage before allocating. Copy cost includes each occurrence, not only each
unique pointer; a shared graph is not a free copy. Fail before a disallowed
operation and publish no partial result. Earlier charges and the first fatal
resource error remain sticky; no refund, scope reset or fresh budget on retry.

Use the fixed field order for recursive object copies too. If obtaining that
order needs a temporary vector and sorting, reserve enumeration, all scratch
storage and a proven worst-case comparison/byte-work bound **before** collecting
or sorting. Base that reservation on bounded field counts and the admitted key
byte limit, not randomized comparison counts. Account for any extra work used
to establish those bounds. Sorting allocation and comparisons cannot run first
and be charged afterwards. Tuple-field insertion order and backing capacity
must not change the logical charge schedule or selected resource failure.
The exact cost constants and their four-build physical mapping remain I7
implementation/admission proof obligations; this note supplies neither.

### Compatibility and finite acceptance matrix

Preserve old value results where container sharing was unobservable, including
scalar/array nesting, missing fields, lexical shadowing and pure nested groups.
Intentionally change results or later observations that depended on shared
backing storage, cross-field/group mutation, or deletion from an input tuple.
This includes an alias-dependent result that happened to be stable across map
trials. Earlier group-order and callable exceptions remain as adopted. There
is no blanket tuple/group refusal, new identity component or relaxation of
complete pre-admission and runtime failure contracts.

The following is a finite minimum corpus. Its fixture JSON and unchanged
legacy controls accompany the candidate artifact. It is a requirement for the
later exact evaluator head, not a claim that this source note executes it.

| Case | Required result and distinguishing check |
|---|---|
| Admitted public reproducer above | Exact separate A/B result; retain both observed legacy outcomes. Run one compiled expression repeatedly, then separate expressions/sessions. |
| Shared `[0,1,2]`, capacity 3 and 4; independent arrays at both capacities | All four produce `a=[0,1,2,"A"], b=[0,1,2,"B"]`. Force both field orders in the retained legacy control: capacity 4 distinguishes the alternatives. |
| Private growth at capacity 0, 1, 2, 3 and the declared maximum; same values with different borrowed capacities | Pin initial capacity, retained-capacity append, clamped doubling, exact/one-below precharges and overflow refusal. No input capacity enters the charge schedule; omitted growth/copy charges permit measured forbidden work. |
| Caller has a length-4 alias including a sentinel at slot 3 | Sentinel remains unchanged after reduction; caller input and sibling field snapshots remain byte-equal. |
| First/middle/last missing fields, present undefined and null; three repeated names across tuples | Missing contributes nothing; present values use the two accumulation rules. Undefined still shadows; no conflation with missing or null. |
| Scalar then array; array then array; empty arrays; nested array/object fields | Exact examples above; nested values retain their shape. No accidental spread, flattening, wrapper conversion or deep-copy alias to input. |
| Shared nested arrays/objects, overlapping slices and repeated reference inside one input | Preserve values and detach each owning result from all input/sibling mutable descendants. An instrumented write through one result cannot change the others; no production mutation API is added. |
| Singleton tuple with `@`; repeated evaluation of that same tuple | Same context both times, original `@` remains; new lexical frame excludes only its own context slot. |
| Same-key interleaved tuple and non-tuple groups; nested groups | First group slot and input encounter order persist. One value evaluation per key; independently shared input containers remain unchanged. |
| Reversed and shuffled field insertion/enumeration; names `10`, `2`, `é`, `日本` | Same values, canonical reduction/copy order, charges and selected failure; source group pair reversal still has its separately adopted effect. |
| Key collision, invalid/undefined keys and preliminary check errors | Preserve discovery-phase `D1009`/`T1003` and existing ordinary-error selection; no group value runs before discovery finishes. |
| Exact and one-below work/allocation/depth limits at enumeration, lookup, growth, recursive copy and value entry | Record actual operations/allocations and no operation past its bound. A second evaluation shares event totals; ordinary errors do not reset them and fatal errors stay sticky. |
| Actual omitted-copy, omitted-order and omitted-charge variants | Borrowed append must expose the capacity-4/sentinel or nested-alias defect. Reverse/shuffle copy order must change an instrumented traversal/error or budget boundary. Removing each charge must permit a measured otherwise-blocked operation/allocation. Detect omitted singleton isolation by the lost `@` control. |

For omission tests, change the actual later implementation and show the positive
control and failing mutant at the same limits. A synthetic counter that never
observes the guarded operation is insufficient. Repeating map trials without
seeing a difference, sorting serialized output, or checking only final JSON
cannot prove ownership, ordering or pre-operation bounds.

No semantic choice is left to the implementer in the cases above. Physical cost
constants, concrete private container layout and exhaustive admitted-path
coverage are still unproved implementation obligations, not discretion to
change these outcomes. Any newly discovered admitted case that conflicts with
this rule must be named on the live I7 commitment and resolved by an explicit
source decision before implementing a different result.

Revalidate stored programs under the changed evaluator/corpus identity before
replay writes. Refusal leaves projection and frontier unchanged. Account for
the change in the already-required `core.jsonata`, grammar, dialect and corpus
versions wherever affected; retain exactly ten identity components and
historical identity recognition. Stored-identity refusal and the separate
acknowledged reset/activation gate still apply. Adoption permits no automatic
migration, actual reset, module publication/pin/replace, provider admission,
installation or live service change. Runtime implementation and all remaining
I7 gates stay on #4043 and promise `5cfedb59`. This child delivers only the
ordinary-adopted, independently reviewed source amendment.

## Object-key probe amendment for adoption

The pinned evaluator preliminarily evaluates each constructor key on the whole
input before discovering its per-item keys. The legacy callback controls in
#4225 reproduce three calls for `{$probe("key"):$}` on `[1,2]`, and one call
for `{$count($)>1 ? $probe("probe-only") : "k":$}` whose per-item branch never calls
the provider. These are baseline observations, not admitted metered providers.
Keeping that probe conflicts with the no-speculation rule below.

At compile/admission, classify each constructor key expression by a complete,
bounded walk of its admitted AST, including nested expressions and all branches.
Use resolved direct calls to selected extensions, never source-text matching
or evaluation of the expression. If any such call occurs in that key AST,
omit its **entire preliminary whole-input evaluation**, including pure work,
lexical effects and errors, even when the selected call's branch is dormant.
Do not substitute a dry-run provider, memoized result, alternate argument frame,
result reuse or uncharged speculative dispatch. This changes neither extension
selection nor its direct-call/no-reference/no-shadow/no-partial confinement.

Evaluate that key normally once at each existing per-item/tuple semantic
evaluation point, with its normal lexical scope, key checks, provider calls,
work charging and failure ordering. Preserve source key-expression order,
undefined keys, per-item `T1003`, discovery-phase duplicate-key `D1009`, and
complete discovery before values run once per group in adopted first-encounter
order. Preserve empty/null/tuple discovery: if it creates a virtual undefined
item, its one normal key evaluation is semantic and any executed selected call
is charged once. Neither omit that evaluation nor add a second provider probe.
A key AST with no selected extension retains the legacy preliminary evaluation
and errors. A call only in a group's value does not change that group's key rule.

This is an explicit compatibility exception. A pure whole-input branch that
previously returned a non-string and caused preliminary `T1003` can now yield
valid per-item string keys, including when the selected call is never executed.
Omitting pure lexical effects can also change later keys and values; the lexical
scope rules themselves remain. For input `[1,2]`, the pinned evaluator gives
`{"2":3,"3":3}` with zero calls for:

```jsonata
($x := 0; {$string($x := $x+1) & (false ? $probe("never") : ""): $x})
```

The amended result must be `{"1":2,"2":2}`, still with zero calls. Its
extension-free twin, replacing `$probe("never")` with `"never"`, must retain
`{"2":3,"3":3}`. The amendment artifact retains executable baseline evidence
for both; proposed results are requirements for #4043, not delivered behavior.

The fixture provider below returns its string argument. Unless stated otherwise,
input is `[1,2]`; each listed call is charged once when executed.

| Expression or case | Required result and ordered provider calls |
|---|---|
| `[$probe("same"),$probe("same")]` | `["same","same"]`; `same,same`. |
| `false ? $probe("unused") : "ok"` | `"ok"`; no calls. |
| `$.($probe("item"))` | `["item","item"]`; `item,item`. |
| `{$probe("key"):$}` | `{"key":[1,2]}`; `key,key`, not three calls. |
| `{$count($)>1 ? $probe("whole") : "k":$}` | `{"k":[1,2]}`; no calls, not one. |
| `{"k":$probe("value")}` | `{"k":"value"}`; `value`. |
| `{$count($)>1 ? 7 : $probe("k"):$}` | `{"k":[1,2]}`; `k,k`, replacing preliminary `T1003`. |
| `{false ? $probe("unused") : ($count($)>1 ? 7 : "k"):$}` | `{"k":[1,2]}`; no calls, replacing preliminary `T1003`. |
| `{$count($)>1 ? 7 : "k":$}` | Preserve preliminary `T1003`; no calls. |
| `{$count($)>1 ? $probe("whole") : 7:$}` | Preserve per-item `T1003`; no calls. |
| `{$probe("k"):$}` with `[]`, then with `null` | `{}`, then `{"k":null}`; one `k` call in each separate evaluation. |

Require exact output/error, ordered call traces, deterministic work/allocation
totals and at-bound/one-over judgments for these cases and the lexical pair.
Also cover nested key expressions/groups, tuple/reduce and two colliding key
expressions, including failure before any group value. Pair the cases with an
extension-free corpus. Remove a semantic dispatch, restore the extra probe,
misclassify a dormant/nested call, or skip a relevant charge and demonstrate
the changed result, error, trace or actual work/allocation beyond the bound.
These necessary controls extend gate 3; they do not replace complete coverage
or any of the eight gates.

Account for changed evaluation/admission and corpus semantics in the existing
`core.jsonata`, grammar, dialect and corpus/version components wherever affected.
Keep exactly ten identity components and old-identity recognition. Revalidate
stored programs before replay writes; refusal leaves projection and frontier
unchanged. Preserve the separate acknowledged reset/activation boundary and
all four delivery stages. Add no identity component or automatic migration.
After exact ordinary adoption, runtime implementation, the eventual pin and
all callers stay on #4043's existing promise. This source amendment authorizes
no public evaluator/module publication, production admission, live operation,
reset or release.

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
[tagged codec](jsonataddl/values.go:51), and text is UTF-8. Structured values cannot contain functions,
undefined values, channels or host pointers. Reuse the implemented scalar and
closed-object validation rules. Extend them
with the bounded arrays and tagged unions required by extension contracts;
do not treat existing string arrays or opaque host JSON as that schema.
Keep opaque host JSON separate. Meter encoding and validation before their
allocations; simply calling the current marshal-first validator cannot prove
the I7 bounds.

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
The implemented `Application.ValidateProgramInput` must also refuse an
extension-enabled application once those applications exist. `Prepare` reuses
its pre-read base-input rules, rejects caller context, then inserts and validates
core-owned context before returning the handle. Its extension path must bound
encoding and allocations before work, while `session.Evaluate` rechecks the
complete prepared input and rows. Neither existing check becomes a bypass.
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
Use per-evaluation bindings for entry instrumentation too; [Assign](https://github.com/jsonata-go/jsonata/blob/599f35f32e5f31297f8b2153b5da44ddbb330e28/v206/jsonata.go#L524) mutates the
cached expression's environment and would share a meter between sessions.
The pinned library also exposes [package-global registration](https://github.com/jsonata-go/jsonata/blob/599f35f32e5f31297f8b2153b5da44ddbb330e28/v206/jsonata.go#L538). The compile-time
name allowlist alone does not keep outside-set builtin values unreachable.
The callable-boundary amendment requires immutable-identity confinement as
well as the preserved syntax checks; admission must prohibit replacement of
allowed built-ins. No host may register an extension globally or alter that frame.

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

Register no extension in SQLite in this version. Preserve the
[default-deny read authorizer](jsonataddl/authorizer.go:14), writer seating/release and query pool contracts.
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
shape, not the host's arbitrary input map. The implemented base metadata
contract remains closed and scalar-only; it does not yet reserve the name
`context`. I7 must reserve that name and add its structured shape to compiled
per-program metadata rather than accept caller-supplied context through
`InputContract.Meta`. Context does not become a SQL parameter
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
Chess the [seat authority checks](https://github.com/generalbusiness-ai/gitseq-chess/blob/b97c6a82ef7e3618721696f5a69efef13da10a79/chess.go#L696) require scope `chess` or `chess:<game>`, vouching Witnessed/SelfSigned
and verification LiveLookup/InLog; an unknown value grants no seat authority.

Resolve at position n with signed timestamp t: an anchor at n is visible at n;
a revocation at n removes authority at n; later records never alter the result
for n. Delegation inherits only the authority available at its own admission
position, and every ancestor must still stand at the queried position. Equal
timestamps do not collapse positions. Expiry matches the pinned host exactly:
`NotAfter == 0` is unlimited; otherwise `t > NotAfter` expires, equality does
not. Root withdrawal invalidates replayed copies of the same Nostr grant.
These rules must be differentially checked against the [actual pinned host resolver](https://github.com/generalbusiness-ai/gitseq/blob/7152e79a741e6c9c277568a6aabec8e9b6cbd792/host/identity/resolve.go#L317).

The current [identity.Resolve(log)](https://github.com/generalbusiness-ai/gitseq/blob/7152e79a741e6c9c277568a6aabec8e9b6cbd792/host/identity/resolve.go#L108) scans history, and [LookupAt](https://github.com/generalbusiness-ai/gitseq/blob/7152e79a741e6c9c277568a6aabec8e9b6cbd792/host/identity/resolve.go#L263) may scan an
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
with the implemented stored-identity guard also protecting continuation.
No module release
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
   diagnostics. Integrate the existing `ValidateProgramInput`/`InputContract`
   rules with metered preparation and the reserved compiled metadata shape;
   preserve both admission checks and reject caller context. Update core README,
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
   and whole-log identity resolver are **not admitted**. The [rules engine](https://github.com/notnil/chess/blob/e70b77074fafd166362ce355d15558b6e95bfb19/game.go#L159)
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
reference documentation and publish its exact candidate artifacts. Decision
delivery alone activates no provider, module or service.
