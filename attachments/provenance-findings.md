# Bounded source-provenance finding for request3919

A clean5d84 reference is a supported **candidate program baseline** for the deployed binary. An exhaustive comparison accounts for every differing byte using explicit build-path/build-ID changes, equivalent decoded debug data, derived UUIDs and verified signature hashes. This is stronger than matching module sums or sampled behavior. It is **not a raw byte-identical rebuild or recovery of the original dirty file list**.

The initial session started11:22:02UTC on September6 and ended before the11:52:02 limit. It used the accepted3917 inventories, source refs/reflogs, one targeted original-build-ID cache scan and disposable reference builds. No live queue/status scan, telemetry/database copy, service/intake/link change, terminal UI/history/log access or backport occurred.

## Exact evidence

Original binary: `/Users/hughpyle/.local/lib/tailapp/tailapp-20260831T183703Z-5d84ac71d625`, SHA256 `5e500c92db3aa7990bc803884e631e4d2b3f0008bd75f0d1b6d9cf5f8d0bc7c9`. Reference binary SHA256 `52ada92e67d4088edc04327532acdce96429c01f523b255f889efb41cfdd564f`. Both are35,821,058bytes; their Go build information matches exactly, including dirty stamp, source revision/time,21 module versions/sums, source URL, Go1.27.0, darwin/arm64 v8.0 and CGO settings.

Reference source is commit `5d84ac71d625b29d9e7e9dbdf4c7c66c60429434`, tree `d13981ac2c936bfb43aeddc363de8b4daa006657`. The only source-checkout status entry was an untracked `provenance-dirty-marker.txt`, outside every reported Go/compiler/embed input. A new isolated build cache and private copy of the module cache kept the original caches intact. The6.639s build used the historical source-URL ldflag and no production changes. [Build command](reference-build-command.json), [build result](reference-build-result.json) and [toolchain hashes/settings](reference-toolchain.json) record the exact invocation. [Input manifest](reference-input-manifest.json) records405 packages and1,974 resolved source/embed inputs with SHA256 and byte lengths. This is a manifest of the reference inputs, not an invented historical manifest.

The [standalone comparator](compare-binaries.py), [invocation](comparison-command.json) and [complete result](complete-binary-comparison.json) establish:

- All22 Mach-O sections agree after the narrowly recorded normalization. Executable text differs only at the83-byte Go build-ID string; constant data and embedded program assets match byte-for-byte.
- Only two equal-length path prefixes change in runtime file tables and decoded debug line data: the disposable source and module-cache prefixes map to their original locations. Decoded debug sections then match. The compressed debug-line representation is63bytes longer, accounting for the unmapped debug segment's size and subsequent debug offsets; debug padding is checked.
- Each Mach-O UUID matches the Go1.27 linker's documented derivation from its initial build ID. Signature metadata is identical, and every one of8,678 SHA256 page hashes per binary verifies. Native codesign strict verification also passed for both files.
- Bytes outside those verified differences agree: **zero unexplained differing bytes**. Separate one-byte mutations in executable text and constant data fail the named section comparisons; [negative controls](comparison-negative-controls.json) retain those failures. Mutated binaries were removed and never executed.

## What remains unknown

The [bounded source search](source-search-evidence.json) found an August30 stash on an older parent, including documentation/embed edits and non-build assets. It is not an August31 installation-time snapshot. The original main archive build ID was not found in the exact cache scan. Targeted retained archive names did not identify an installation-time source capture. These bounded searches do not prove that no owner archive exists elsewhere.

The evidence cannot distinguish untracked/non-build dirt from comment-only or unlinked-source edits that produce the same linked program, nor recover the exact historical file list or original toolchain-binary hashes. The full-file comparison instead establishes a completely auditable reference program with all differences explained. Definitive historical input recovery would require the owner’s complete19:37BST August31 checkout capture—tracked diff, untracked and embedded files, plus build/toolchain manifest—or the original matching cached compile/link artifacts. No further speculative search session is justified without such new evidence.

## Proposed next step and retention

Have the requester independently accept or reject the comparator, input manifest and qualified baseline. If accepted, a **separate** bounded implementation request can commission only the Pending ordering correction on a new branch from exact5d84, preserving runtime5032, definitions, dependencies and schema, followed by independent source/binary review and disposable preservation/drain checks. Nothing here authorizes implementation, replacing the resident, pausing intake or copying live data. The accepted3917 cold whole-home/WAL and rollback gates still apply.

The disposable reference source clone and both temporary caches are removed; [cleanup evidence](disposable-cleanup.json) names them. Only the reference binary, exact source archive, comparator and text evidence remain in the recorded private temporary evidence directory. Original binary, source checkout, source refs and caches were preserved. [Evidence manifest](evidence-manifest.json) records hashes for the source archive, complete input inventory, build transcript and comparator. The retained801,945-byte compiler transcript is local evidence; it is not a terminal history or a source of user-session logs.
