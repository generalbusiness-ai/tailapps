# Corrected I5 nested-module release candidate

Exact proposed source: `8c674fa9ecb4797f7de4322ac97cb3ffe2d21672` on
`request/declared-input-module-release`, pushed; draft PR8.
Module/version: `github.com/generalbusiness-ai/tailapps/jsonataddl@v0.2.0`.
Proposed immutable annotated tag: `jsonataddl/v0.2.0` at that exact commit,
only AFTER independent approval/ratification and its sealed incorporation into
main. The tag source stays the reviewed commit, not a later unreviewed head.
No tag exists and no publication or activation has occurred.

Module checksum: `h1:rD0TyYRPHT+DapFEacbd+jLQKHo6I+mi2ufpcCd+eKY=`.
GoModSum: `h1:fpGrE/1ODSULhyxThhr3TL4JtpfcX3PdA4kJBnKWJRY=`.
The module checksum changed because README.md ships in the archive; go.mod
and go.sum are untouched. The113-entry archive contains exactly the revised
README and otherwise the same module files as c57f34b2. Values are calculated,
not yet publicly resolved. The x/mod v0.39.0 CreateFromVCS/Hash1 method already
reproduced both actual public v0.1.2 hashes as a control; Checker independently
confirmed the method in ratified changes-requested db87db21.

## Review correction

Only jsonataddl/README.md and docs/reference/releases.md change. The packaged
install command now names v0.2.0; its prose names the required complete Input
contract, positive MaxInputDepth and re-pinned composed identity. The obsolete
claim that the install command lacks this contract is removed. Duplicate
publication prose is replaced with a link to the repository release procedure.
That procedure names v0.2.0, retains fail-closed public absence checks, requires
exact expected module and go.mod checksum comparison, explains that tag CI is
a post-publication check and preserves separate root dependency adoption.
No verifier code, dependency pin, runtime behavior or I7 capability changes.
Hugh retains the root requirement/public-install compatibility follow-up for
after v0.2.0 exists. The current docs explicitly forbid claiming a later root
release consumes the new core while its requirement still selects v0.1.2.

Architecture: the core/compiler/evaluator, profile and continuation boundaries
in notes/2026-08-28-tailapp-architecture.md retain their exact c57f34b2 contracts;
this head corrects release instructions, not a layer contract. Security:
publishing the wrong documented version would be permanent, so the immutable
README now matches the proposed tuple. The expected-hash comparison remains
an explicit release operation; the existing example verifier is not claimed
to implement it. Simplification: release ordering has one detailed home.

Current Hugh source child8eb99e73 and promisef4679b6b govern the two-file
source delivery. Parent release requestb9d6cd89 and promise9f5158a0 remain
open for publication and actual public resolution. Checker verdict db87db21 is ratified c8bcd701. Hugh explicitly separated source closure from the publication outcome.
This final head adds only the source-child Rests-On commit; its entire tree
is byte-identical to a0d64245, and its re-derived module hashes are identical.
The parent request is not replaced and no new decision is introduced. Ordinary staleness is acknowledged;
there is no claim that the historical design chain is fresh. The source seal
alone is not the completed release: tag CI and public-resolution evidence
remain required under the release conditions.

## Validation and exact publication plan

All12 prepublication checks reran successfully on byte-identical a0d64245;
the final source-child binding commit changes no file bytes:
go version, dependency download/tidy/verify, module boundary, full module tests,
full race tests, vet, pinned vulnerability scan, module dependency list, actual
host composed-identity fixture and clean tree. GOWORK=off/public-proxy/strict
sumdb environment is unchanged; the task's previously fresh caches are reused
on this documentation-only correction. No dependency replacement is introduced.
The first c57 run used newly created module/build/GOPATH caches; distinguish
that from the current repeat. Actual PR CI/release dry run are also required.

Tailapp composed identity remains
`jsonata-ddl-runtime:sha256:3e60ce38390fa376d766b5880cb8f79ebd9196f0e4036335beb9285783efbd33`.
Core interface2026-09-05, tailapp-otlp/2 digest59524300..., orchestration/3 and
the closed nine-component set are unchanged. Full pins match v0.1.2 exactly.

After exact source+publication review approval is ratified, seal the two changed
paths and push source/history/exact receipt, verify main CI, and confirm the
reviewed commit is an ancestor of public main. Recompute the reviewed hashes
and rerun the existing v0.2.0 absence preflight immediately before publishing.
Push only the approved annotated nested tag. Require actual tag-triggered CI
and verify external resolution with fresh module/build/GOPATH caches, public
proxy and checksum database, GOWORK=off, no replacement or private exemption.
Run the existing packaged-example verifier and separately compare both
returned checksums with this reviewed tuple. Then record the exact public
version/source/checksums/identity as the release handoff and clean owned work.
Inventory gate3 and production activation remain separately authorized work.
