# I7 host-version profile verification

This is a proof component for the live I7 commitment, not its completion or a published dependency pin. It describes evaluator head `bebeed31322f96ce9d75044acd71382466fde5c2` with Go 1.26.7 and CGO disabled.

The previous evaluator-root test binaries inherited main-module Go 1.23 compatibility defaults. The evaluator's module still declares Go 1.23.0 (and its toolchain line remains unchanged); that declaration is not the final host's main-module version. I created an isolated module declaring Go 1.26.7 and temporarily replaced its evaluator dependency with the exact local checkout. Go's own `cmd/go/internal/load/godebug.go` chooses the main-module version for default GODEBUG selection, and generated test mains use that selection. The generated-main metadata and four new binary build records carry no older-version DefaultGODEBUG overrides. This addresses the fixture default-selection mismatch; it does not establish all final host environment constraints or the full runtime call graph.

All four test binaries compiled with GOTOOLCHAIN=go1.26.7, CGO_ENABLED=0 and GOWORK=off. GOARM64 was v8.0 and GOAMD64 was v1; GOFLAGS and GOEXPERIMENT were empty. On native Darwin ARM64, `TestResource|TestParserToken` ran once with GODEBUG, GOGC and GOMEMLIMIT absent and all 422 top-level tests passed. Linux ARM64, Linux AMD64 and Darwin AMD64 were compiled only, not executed. This was the selected resource/parser suite, not the entire legacy evaluator suite.

| Target | Binary bytes | SHA-256 |
|---|---:|---|
| darwin-arm64 | 9612450 | `c7c9b5ce87db16b0c9e36f50a0cb8bebf6f4181297f3191bb1ac2327aa9fa0f4` |
| darwin-amd64 | 10330992 | `e6f7bc5ac2cb531839ec510b96e25c4ee9dee0e95a7bae14a3544c005c4fab12` |
| linux-arm64 | 9453127 | `92e3dff1eb6803d44e43a48d5a48302a15c5ae273cdb9cca9d5fb972ec254115` |
| linux-amd64 | 10227416 | `c55a768de6cfc92898174dfeee026579f6c8e04211202bfbc7f363446aad50d0` |

The `sync.(*Once)` disassembly was compared against the retained preparation-proof binaries. Absolute call/branch addresses were normalized for this limited instruction-shape comparison. On ARM64, two changed ADRP/MOVBU address pairs in each binary were resolved independently from their actual instruction PCs and checked against that binary's `nm` address for `runtime.arm64HasATOMICS` before normalizing those pairs. After those verified data relocations, the symbol/instruction text matches on all four targets. This supports carrying the previously inspected Once frame and instruction-shape component. It is not whole-binary equivalence, a proof of external call-target equivalence, or a completed runtime allocation bound.

Retained evidence:

- `/tmp/tail4043-host-profile-attribution.json`: main/dependency versions, binary identities, test attribution and Go source hashes.
- `/tmp/tail4043-host-profile-directory.txt`: isolated module directory, including generated-main package metadata, build records, binaries, native test log and disassemblies.
- `/tmp/tail4043-host-profile-once-comparison.json`: exact compared files, assembly hashes and verified feature-symbol address arithmetic.

Remaining I7 obligations include final host admission/environment constraints; complete preparation/evaluation/host allocation composition; runtime populations and retained allocator/profile/stack mappings; and the full finite physical F argument. In particular the Linux vgetrandom pool estimate is conditional on its maximum checked-out state population, and the goroutine-pool optimistic read prevents assuming that a fresh G allocation proves every global free G was checked out. Neither conditional component closes those missing population proofs. No GC refund or RSS equivalence is claimed. No repository source, public fork, public dependency pin or resident changed in this verification.
