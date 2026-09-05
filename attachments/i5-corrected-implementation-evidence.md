# I5 correction at c61e7f7c0eba433eabe134bc397d579fba7a0ab4

The reviewer changes-requested report c9183f34 was ratified by Builder as 591fadfe. Hugh's c6b3b414 reproduction confirmed the column-case issue. This correction stays under the original satisfied authority-bearing decision; no self-request or self-promise was filed.

Nine paths changed after df7095298. Table column admission now uses case-insensitive SQL identifier matching. A real host regression additionally exposed selected-spelling mismatch: SQLite returns declaration casing, while the closed read envelope names selected columns. Host and corpus read adapters now map values to the selected names after checking column count. Documentation states that contract. No identities, corpus goldens, pins or runtime guards changed in this correction.

The actual host fixture sends two deliveries through SQL reads and JSONata. The first empty MANY yields count0 with no gap; the second populated MANY yields count1. Lowercase and uppercase selected columns both pass. Reopen and exact duplicate retry preserve two rows and report AlreadyApplied. The core public LoadApplication/Evaluate regression tests lower, upper and mixed case selections: valid INTEGER passes; string, null, fractional number and object refuse before evaluation.

Three actual compiling Go-overlay omissions each fail their intended assertion and leave source unchanged:
- Empty MANY reverted to nil: first host delivery refuses rows prior, array required.
- Host selected-spelling mapping reverted: second uppercase delivery refuses required VALUE.
- Core case-insensitive lookup reverted: uppercase and mixed-case invalid INTEGER values are admitted.

Current correction validation: full host go test ./... passed all packages; full standalone core go test -race ./... passed; both modules go vet passed; focused host read/authorizer tests under race passed. Original full host race, ten guard controls and full corpus migration audit below apply to prior df7095298. Current full core race re-exercised the corpus without golden changes. CI is recorded separately after completion. No release or public activation claimed.


## Current host-tests

```text
?   	github.com/generalbusiness-ai/tailapps/cmd/tailapp	[no test files]
ok  	github.com/generalbusiness-ai/tailapps/internal/buildinfo	(cached)
ok  	github.com/generalbusiness-ai/tailapps/internal/cli	1.063s
ok  	github.com/generalbusiness-ai/tailapps/internal/control	3.162s
ok  	github.com/generalbusiness-ai/tailapps/internal/definition	(cached)
ok  	github.com/generalbusiness-ai/tailapps/internal/engine	5.978s
ok  	github.com/generalbusiness-ai/tailapps/internal/inbox	(cached)
ok  	github.com/generalbusiness-ai/tailapps/internal/ingest	(cached)
ok  	github.com/generalbusiness-ai/tailapps/internal/mcp	1.727s
ok  	github.com/generalbusiness-ai/tailapps/internal/metrics	(cached)
ok  	github.com/generalbusiness-ai/tailapps/internal/profile	3.562s
ok  	github.com/generalbusiness-ai/tailapps/internal/projection	2.171s
ok  	github.com/generalbusiness-ai/tailapps/internal/query	2.446s
ok  	github.com/generalbusiness-ai/tailapps/scripts	52.087s
ok  	github.com/generalbusiness-ai/tailapps/tailapps	4.207s
```

## Current core-race

```text
ok  	github.com/generalbusiness-ai/tailapps/jsonataddl	3.482s
```

## Current host-race

```text
ok  	github.com/generalbusiness-ai/tailapps/internal/projection	1.864s
```

## Current host-vet

```text
```

## Current core-vet

```text
```

## Current omission empty-many

```text
--- FAIL: TestManyReadEmptyAndPopulatedDelivery (0.01s)
    --- FAIL: TestManyReadEmptyAndPopulatedDelivery/value (0.01s)
        projection_test.go:391: delivery 1: input rows read "prior" must be an array of at most 10 rows
FAIL
FAIL	github.com/generalbusiness-ai/tailapps/internal/projection	0.370s
FAIL
```

## Current omission selected-case

```text
--- FAIL: TestManyReadEmptyAndPopulatedDelivery (0.01s)
    --- FAIL: TestManyReadEmptyAndPopulatedDelivery/VALUE (0.01s)
        projection_test.go:391: delivery 2: input rows read "prior": field "VALUE" is required
FAIL
FAIL	github.com/generalbusiness-ai/tailapps/internal/projection	0.368s
FAIL
```

## Current omission column-type

```text
--- FAIL: TestReadColumnCasePreservesTypeAdmission (0.01s)
    --- FAIL: TestReadColumnCasePreservesTypeAdmission/BALANCE (0.00s)
        --- FAIL: TestReadColumnCasePreservesTypeAdmission/BALANCE/string (0.00s)
            input_test.go:358: invalid INTEGER input: <nil>
        --- FAIL: TestReadColumnCasePreservesTypeAdmission/BALANCE/null (0.00s)
            input_test.go:358: invalid INTEGER input: <nil>
        --- FAIL: TestReadColumnCasePreservesTypeAdmission/BALANCE/fraction (0.00s)
            input_test.go:358: invalid INTEGER input: <nil>
        --- FAIL: TestReadColumnCasePreservesTypeAdmission/BALANCE/object (0.00s)
            input_test.go:358: invalid INTEGER input: <nil>
    --- FAIL: TestReadColumnCasePreservesTypeAdmission/BaLaNcE (0.00s)
        --- FAIL: TestReadColumnCasePreservesTypeAdmission/BaLaNcE/string (0.00s)
            input_test.go:358: invalid INTEGER input: <nil>
        --- FAIL: TestReadColumnCasePreservesTypeAdmission/BaLaNcE/null (0.00s)
            input_test.go:358: invalid INTEGER input: <nil>
        --- FAIL: TestReadColumnCasePreservesTypeAdmission/BaLaNcE/fraction (0.00s)
            input_test.go:358: invalid INTEGER input: <nil>
        --- FAIL: TestReadColumnCasePreservesTypeAdmission/BaLaNcE/object (0.00s)
            input_test.go:358: invalid INTEGER input: <nil>
FAIL
FAIL	github.com/generalbusiness-ai/tailapps/jsonataddl	0.367s
FAIL
```

# Prior-head implementation evidence (df7095298)

# I5 implementation evidence

Exact candidate df7095298c48b248d2ca77826715a7865ccd7cb8; 45 changed paths, one commit over delivered e0c83a8.

Self-initiated follow-on authority: Hugh held ratifier when signing request e919a5d7 (#3447); its conditions explicitly commission the decision and authorize implementation only after independent review, sealed delivery and satisfaction. Checker approved corrected decision df154a0e in verdict 8dac7841, which Builder ratified. Receipt 9b602faa sealed it into main 1eef3c19; current decision artifact is 7f8eea0b. Original design report c12654f2 was accepted and the request is satisfied. The request statement and landed decision were effective, fresh and unretired at the follow-on start (captured depth3481, /tmp/tail-i5-authority-current.json); the old commitment row’s inherited historical staleness did not make either governing statement stale. No self-request, self-promise or self-report was created. Reviewer must independently confirm all four authority facts from the durable record.

Core contract: closed immutable metadata and host structured members; scalar SQL parameters remain HostEvent. Required/optional and nullable/non-null are independent, names and exact canonical scalar types are admitted before compilation. Input numbers use existing logical rules and original encoded JSONata bytes; opaque 1e999 is accepted by admission while existing evaluator behavior is unchanged. Container depth1024 is independent of execution depth64. Row shape/cardinality/types derive from the compiled plan. Canonical identity uses escaped fixed-order JSON with declaration-name sorting, preserving semantic array order.

Host boundary: validation before read binding, empty MANY arrays, same-runtime continuation plus actual persisted runtime checked before engine journaling/detachment and again inside projection transaction. Current-compiled recovery cannot substitute for physical identity. Pending-journal recovery, queued obligations, identity and frontier preservation and acknowledged reset controls pass.

Fixture diff reviewed individually: eight obsolete emission_ordinal keys removed, other corpus input bytes unchanged; three runtime/revision goldens updated, storage/export digests unchanged; settle-first marks_seen changes1→0 for empty MANY. All other evaluation/projection diagnostics unchanged, including every malformed-output case. Complete Go producer inputs and separate public negative example migrated explicitly.

Validation: full host go test ./... and go test -race ./... passed; full core suites and race passed; both go vet ./... passed with GOWORK=off. Final small fixture and semantic-array tests reran with race. No dependency changes, release or production activation.

Ten temporary omission mutants were individually restored. Every run failed the intended test, not compilation. Engine persisted-runtime omission lost queued obligations; projection omission admitted unsafe continuation; scalar/collision/identity/depth/pre-read/core-host compatibility omissions all turned their guards red.

## scalar-type-admission

```text
--- FAIL: TestInputContractRefusesIncompleteOrAmbiguousDialects (0.00s)
    --- FAIL: TestInputContractRefusesIncompleteOrAmbiguousDialects/scalar_type_JSON (0.00s)
        input_test.go:286: invalid contract compiled
    --- FAIL: TestInputContractRefusesIncompleteOrAmbiguousDialects/scalar_type_text (0.00s)
        input_test.go:286: invalid contract compiled
    --- FAIL: TestInputContractRefusesIncompleteOrAmbiguousDialects/scalar_type_TEXT_INJECT (0.00s)
        input_test.go:286: invalid contract compiled
    --- FAIL: TestInputContractRefusesIncompleteOrAmbiguousDialects/scalar_type_TEXT;other (0.00s)
        input_test.go:286: invalid contract compiled
FAIL
FAIL	github.com/generalbusiness-ai/tailapps/jsonataddl	0.409s
FAIL
```

## scalar-value-admission

```text
--- FAIL: TestDeclaredInputAdmissionPrecedesEvaluation (0.00s)
    --- FAIL: TestDeclaredInputAdmissionPrecedesEvaluation/integer_fraction (0.00s)
        input_test.go:186: before reads: want "field \"position\"", got <nil>
    --- FAIL: TestDeclaredInputAdmissionPrecedesEvaluation/numeric_boolean (0.00s)
        input_test.go:186: before reads: want "field \"flag\"", got <nil>
    --- FAIL: TestDeclaredInputAdmissionPrecedesEvaluation/invalid_blob (0.00s)
        input_test.go:186: before reads: want "field \"blob\"", got <nil>
    --- FAIL: TestDeclaredInputAdmissionPrecedesEvaluation/nonfinite_real (0.00s)
        input_test.go:186: before reads: want "field \"real\"", got <nil>
    --- FAIL: TestDeclaredInputAdmissionPrecedesEvaluation/nested_scalar_type (0.00s)
        input_test.go:186: before reads: want "field \"quantity\"", got <nil>
    --- FAIL: TestDeclaredInputAdmissionPrecedesEvaluation/inexact_integer (0.00s)
        input_test.go:186: before reads: want "field \"position\"", got <nil>
    --- FAIL: TestDeclaredInputAdmissionPrecedesEvaluation/optional_is_not_nullable (0.00s)
        input_test.go:186: before reads: want "field \"optional\"", got <nil>
    --- FAIL: TestDeclaredInputAdmissionPrecedesEvaluation/integer_string (0.00s)
        input_test.go:186: before reads: want "field \"position\"", got <nil>
FAIL
FAIL	github.com/generalbusiness-ai/tailapps/jsonataddl	0.357s
FAIL
```

## input-kind-identity

```text
--- FAIL: TestInputIdentityIsCanonicalCompleteAndImmutable (0.00s)
    --- FAIL: TestInputIdentityIsCanonicalCompleteAndImmutable/event_kind (0.00s)
        identity_test.go:94: semantic mutation retained identity
FAIL
FAIL	github.com/generalbusiness-ai/tailapps/jsonataddl	0.353s
FAIL
```

## input-collision

```text
--- FAIL: TestInputContractRefusesIncompleteOrAmbiguousDialects (0.00s)
    --- FAIL: TestInputContractRefusesIncompleteOrAmbiguousDialects/event_collision (0.00s)
        input_test.go:286: invalid contract compiled
    --- FAIL: TestInputContractRefusesIncompleteOrAmbiguousDialects/duplicate_structured (0.00s)
        input_test.go:286: invalid contract compiled
FAIL
FAIL	github.com/generalbusiness-ai/tailapps/jsonataddl	0.354s
FAIL
```

## input-depth

```text
--- FAIL: TestOpaqueInputJSONAndDepthBounds (0.01s)
    --- FAIL: TestOpaqueInputJSONAndDepthBounds/1022 (0.00s)
        input_test.go:210: over-depth input: <nil> / <nil>
FAIL
FAIL	github.com/generalbusiness-ai/tailapps/jsonataddl	0.377s
FAIL
```

## pre-read-admission

```text
--- FAIL: TestFoldReadsExecuteUnderTheDefaultDenyAuthorizer (0.02s)
    projection_test.go:351: read binding preceded input admission: program "update_guard_analytics" read "probe": sqlite3: authorization denied: access to tailapp_projection_identity.revision is prohibited
FAIL
FAIL	github.com/generalbusiness-ai/tailapps/internal/projection	0.464s
FAIL
```

## projection-persisted-runtime

```text
--- FAIL: TestContinueChecksStoredRuntimeDespiteCurrentCompiledProfile (0.01s)
    json_upgrade_test.go:198: old runtime continued: <nil>
FAIL
FAIL	github.com/generalbusiness-ai/tailapps/internal/projection	0.360s
FAIL
```

## engine-persisted-runtime

```text
--- FAIL: TestRuntimeProfileUpgradeKeepsQueryAndControlButClosesIngestion (0.07s)
    engine_test.go:815: continue changed obligations: []inbox.Delivery(nil), <nil>
FAIL
FAIL	github.com/generalbusiness-ai/tailapps/internal/engine	0.580s
FAIL
```

## core-runtime-compatibility

```text
--- FAIL: TestContinueCompatibleOverCoreHandles (0.01s)
    application_test.go:375: same storage under different runtime: <nil>
FAIL
FAIL	github.com/generalbusiness-ai/tailapps/jsonataddl	0.359s
FAIL
```

## host-runtime-compatibility

```text
--- FAIL: TestRevisionAndStorageCompatibilityAreSeparate (0.00s)
    compiler_test.go:147: different runtime accepted: <nil>
FAIL
FAIL	github.com/generalbusiness-ai/tailapps/internal/profile	0.446s
FAIL
```
