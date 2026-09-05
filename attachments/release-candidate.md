# I5 nested-module release candidate

Candidate module: `github.com/generalbusiness-ai/tailapps/jsonataddl@v0.2.0`.
Proposed immutable annotated tag: `jsonataddl/v0.2.0`.
Exact source: `c57f34b26d2f879c813ea01b8854981c04c0e722`, already sealed and pushed to `main`.
No new source changes or tag have been made. The owned validation checkout is
`/Users/hughpyle/play/tailapp-worktrees/declared-input-module-release` on `request/declared-input-module-release`.

## Scope and authority

Hugh's current request `b9d6cd89c01d28cc19236aa9208ca9a073bd0de5`
authorizes this gate-2 release after independent exact-candidate Checker
approval and Builder ratification. Promise `9f5158a0115281fabec88c351094f1e5f45c2571`
is effective. The request explicitly acknowledges ordinary staleness and
confirms unchanged release conditions, destination, governing decision and
availability. Its ordinary staleness is recorded; it is not claimed fresh.
This release uses the already reviewed I5 behavior, and introduces no new
runtime contract, I7 capability, Inventory adoption or production activation.
The release operation will close by report and Hugh ratification after public
resolution, rather than a duplicate source merge.

Source delivery receipt: `d8885d4e2abb21e6febb6aa40e101c6e95f22a13`.
Exact receipt ref: `refs/gitseq/merge-receipts/ae4557a80f5d166bb84de2a2e5627067973225ab1aa17d63b710885a6b46094c`.
Both that public ref and public main resolve to the exact source above.
Main CI33965132154 passed; source review8914d33d was ratified6260968e.
The delivery published45 changed paths, retired75 covered predecessors and59
unprotected abandoned Builder candidates, and retained3 carried pointers.

## Version and bytes

Current public nested-module tags end at v0.1.2. v0.2.0 is a minor-version
bump for the deliberately incompatible required input contract and runtime
transition. The existing availability script passed: exact tag absent from
GitHub, version absent from the proxy list, and no sum.golang.org record.
No version-specific proxy .info endpoint was probed. Repeat this existing
preflight immediately before publishing; any existing or ambiguous record
refuses publication, and no version may be overwritten.

Proposed module checksum: `h1:azvskIinOqGBiKw57ZgziruJKaFH5fjXG0UIKlcrn2Q=`.
Proposed go.mod checksum: `h1:fpGrE/1ODSULhyxThhr3TL4JtpfcX3PdA4kJBnKWJRY=`.
These are calculated candidate checksums, not a claim of public resolution.
The temporary checksum helper uses golang.org/x/mod v0.39.0
`zip.CreateFromVCS` on the exact Git revision/subdirectory and `dirhash.Hash1`.
Its calculation independently matches BOTH actual public v0.1.2 checksums
from clean Go resolution of released source9280be8b. Candidate archive has
113 files; the attached manifest gives their byte counts and SHA-256.
Neither go.mod nor go.sum changed from v0.1.2: JSONata remains
v0.0.0-20250709164031-599f35f32e5f and ncruces SQLite remains v0.35.3,
with wasm v3.2.35304. The v0.1.0 checksum-conflict retraction remains intact.

## Identity and continuation boundary

Core interface: `jsonata-ddl-application-interface/2026-09-05`.
Dialect: `tailapp-otlp/2+sha256:59524300ac2d079e8abccaafc6c161294f853c560f2ced69105ff43f8eabc464`.
Tailapp composed identity: `jsonata-ddl-runtime:sha256:3e60ce38390fa376d766b5880cb8f79ebd9196f0e4036335beb9285783efbd33`.
The actual pinned composed-runtime test passed against c57f34b2.
The closed nine-component set, grammar, JSONata, SQLite, value codec,
canonicalization and query-value contracts remain as reviewed. Tailapp's
orchestration is two-stage-txn/3. Crossing stored runtimes still requires
explicit acknowledged reset; publishing a module performs no activation.
Inventory must independently bump its owned dialect, declare its frozen
inputs, update its identity fixture and pass gate3 before adoption/activation.

## Fresh validation

All gates ran with GOWORK=off, GOENV=off, public-only GOPROXY,
GOSUMDB=sum.golang.org and empty private/proxy-exemption/insecure/GOFLAGS
settings, using newly created module/build/GOPATH caches. No local replacement
was added to the nested module. Complete core tests, race tests, vet, pinned
vulnerability scan, dependency download/verification/tidy, production and test
module-boundary checks, module-list checks and the actual host identity fixture
passed. govulncheck reported no vulnerabilities. The exact checkout is clean.
These cover the prepublication module release workflow; actual tag-triggered
release CI and clean public consumer resolution can happen only after approval
and publication. Existing verifier will then run with a fresh external module
and fresh caches, no workspace or replacement, require the exact resolved
version and execute the packaged ExampleLoadApplication. Both returned
checksums must equal the reviewed candidate above.

## Review requested

Record Architecture, Security and Simplification for this exact version,
source, hashes and publication plan. The architecture contract in
notes/2026-08-28-tailapp-architecture.md is unchanged at c57f34b2; inspect the
module/host boundary and dependency isolation. Check immutable tag admission,
clean public-resolution method, checksum computation and actual runtime
identity. No new runtime source or additional design decision is proposed.
Publication remains contingent on independent exact approval and ratification.
