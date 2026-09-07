Corrected checker recommendation report for tailapp request #4211 (82e3d5fc…9d4bc99f), filed 2026-09-07 against the unchanged promise fc6503ba, replacing report 933cbc53 after Builder's finding 606aa78c (#4214). Planning only: no repository created, no upstream contact, nothing published, no pin or module-path change. Every load-bearing fact was re-checked directly by checker; the module inventory is attached as module-inventory.txt.

# Publication route recommendation for the I7 stage-1 metered evaluator (corrected)

## What changed since 933cbc53

Builder's four findings in 606aa78c are each accepted and answered below: the whole-module inventory and self-import treatment (section 2), review of the eventual complete head rather than a frozen checkpoint plus rename (section 4), no second core-pin assignment (section 5), and qualified ownership and history claims (section 1). The recommendation itself, an organization-maintained public fork, is unchanged and remains a candidate, not an adopted policy.

## 1. Facts

**CONFIRMED, the checkpoint.** The snapshot Builder captured and this report inventories is the private checkpoint `e9fa318e42ca830063ead9f669f6d75faa4ffba1` on `request/i7-metering-proof`, 78 commits and 133 files (+19380/-18) over upstream base `599f35f32e5f31297f8b2153b5da44ddbb330e28`, every changed path under `v206/`, with `go.mod`, `go.sum` and `LICENSE` unchanged. That checkpoint is unfinished: at observation the branch head was `bb572000f57f0600b3c027a84ae31a31f8389493`, three commits later, and Builder's #4043 work continues. Nothing in this report freezes the release to the checkpoint.

**CONFIRMED, the whole published module** (git grep and ls-tree at `e9fa318e`; attached). The module `github.com/jsonata-go/jsonata` ships four things, not one:

| Area | Packages | Self-imports of the module path | Role |
|---|---|---|---|
| root | `.` (adapter_v154.go, adapter_v206.go, adapters.go, jsonata.go) | 3 (the `module` line, and imports of `/v154` and `/v206`) | version-selecting adapter (`Open`, `AvailableVersions`) binding both evaluators |
| `v154/` | `v154`, `jlib`, `jlib/jxpath`, `jparse`, `jtypes`, plus two `main` tools `jsonata-server` and `jsonata-test`, tests and `testdata`, a `.github` workflow | 49 | the older evaluator, retained upstream |
| `v206/` | `v206` | 0 (imports neither root nor `v154`) | the metered evaluator the core uses |
| `exerciser/` | separate module `github.com/jsonata-go/jsonata/exerciser` | 4 (`module` line, `replace … => ../`, `require` pseudo-version 580cd10d, import of the root package) | web tool importing the root adapter |

Total: 56 self-import lines across three Go package areas and two `go.mod` files. A module path rename is therefore a whole-tree rewrite, not one `go.mod` line, and section 2 states the treatment of each area.

**CONFIRMED, shared-core callers.** Unchanged from 933cbc53: five files in two modules (`internal/projection/projection_test.go`; `jsonataddl/application.go`, `compile.go`, `confine.go`, `evaluate.go`), all importing only `…/v206`, both `go.mod` files requiring `github.com/jsonata-go/jsonata v0.0.0-20250709164031-599f35f32e5f`.

**CONFIRMED, upstream, stated within what was observed.** The repository owner is the GitHub organization `jsonata-go`, created 2025-06-07, with one public repository, no public members, and a collaborator roster that is not observable (`403 Must have push access`; checker holds `pull` only). The repository is public, not archived, default branch `master`, last pushed 2025-07-09, no tags or releases. The `LICENSE` (MIT, "Copyright (c) 2021 Blues Inc") and `CONTRIBUTING.md` are inherited from the Blues Inc origin of this code; they say where the code came from and under what terms, not who maintains `jsonata-go/jsonata` now. The API returned zero pull requests, zero issues and zero forks at query time; that shows none are observable now, not that none ever existed. Who maintains the repository, and whether anyone reads contributions, is OWNER-DEPENDENT and unobserved.

**CONFIRMED, Go module rules.** As in 933cbc53: `v206/` has no `go.mod`, so it is an import subpath of a v0 module, not a major version, and needs no `/vN` suffix; pseudo-versions are immutable ordinary versions; a public module is verified through proxy.golang.org and sum.golang.org with no `GOPRIVATE` and no `replace`.

**CONFIRMED, destination.** `generalbusiness-ai/jsonata` and `generalbusiness-ai/jsonata-go` both return 404, so neither exists. The organization already publishes `github.com/generalbusiness-ai/tailapps/jsonataddl` through the proxy. Checker's own account is an active organization admin. That is a fact about the account; it does not authorize any agent, including this one, to create or publish anything.

## 2. Treatment of every retained package under the fork

The fork keeps upstream's whole tree and history (a history-preserving push, so `599f35f3` is a real ancestor). The module path becomes `github.com/generalbusiness-ai/jsonata` and **every one of the 56 self-imports is rewritten** to it in the same reviewed patch. The hazard this closes: an unrenamed import such as `github.com/jsonata-go/jsonata/v206` inside a module named `github.com/generalbusiness-ai/jsonata` is a different module to Go, and the proxy would resolve it to upstream's unmetered evaluator without any error. The root adapter must therefore import the fork's own `/v154` and `/v206`, and the checks below prove it.

- **root adapter** (`.`): retained; both imports rewritten. After the rewrite `jsonata.Open("v2.0.6")` binds to the fork's metered `v206`.
- **`v154/`** and its five subpackages, two tools, tests, testdata and workflow: retained unchanged in behaviour; all 49 self-imports rewritten. The core does not import `v154`; it stays so that the root adapter and upstream's published surface remain intact. Dropping it is a separate, optional, later decision.
- **`v206/`**: the reviewed evaluator; no self-imports to rewrite.
- **`exerciser/`**: a separate module. Rewrite its `module` line, its `replace … => ../`, its `require` and its one import to the new path, or delete the directory; the core does not import it either way. The `replace` stays inside that tool's own `go.mod`; nothing delivered to the core carries a `replace`.
- **Packaging and notices**: `LICENSE` byte-identical; `v206/jsonata-js/LICENSE` untouched; add a `NOTICE` naming upstream `github.com/jsonata-go/jsonata`, the fork point `599f35f32e5f31297f8b2153b5da44ddbb330e28`, the MIT terms as retained, and the fork's purpose. Review the tracked `v206/.claude/settings.local.json` and the two inherited workflow files before anything is published under the organization's name.

**Verification of the closure, at the eventual reviewed head:** `git grep -n jsonata-go/jsonata -- '*.go' '*.mod'` returns nothing; `go list -deps ./...` in the fork, in `exerciser/`, and in both core modules names no `github.com/jsonata-go/` package; `go.sum` in the core modules carries no `github.com/jsonata-go/jsonata` line; `go build ./...` and `go test ./...` green in the fork, and `go vet ./...` clean.

## 3. Routes

**Route A, upstream acceptance:** unchanged assessment. The module path would never change, but completion depends on a maintainer whose identity is unobserved, with no observable precedent of accepted contributions and no bounded timescale, and the patch is not finished. Not supportable as the stage-1 pin; optional contribution-back later.

**Route B, organization-maintained fork:** `generalbusiness-ai/jsonata`, public, history-preserving, module path `github.com/generalbusiness-ai/jsonata`, core import `github.com/generalbusiness-ai/jsonata/v206`, whole-tree self-import rewrite per section 2, immutable pin by ordinary public resolution (an annotated tag such as `v0.1.0-meter.1`, or a pseudo-version; never re-point a tag).

## 4. Review plan: the eventual complete head, not the checkpoint

1. Builder finishes the evaluator on `#4043`. The release candidate is that **complete head**, which will include every justified change: `v206` work, the module rename across all 56 self-imports, `exerciser` treatment, `NOTICE`, packaging and workflow cleanup. The 78-commit checkpoint constrains nothing.
2. **Local exact-head review before first publication**: diff the candidate against `599f35f3`; confirm every change is justified by the stage-1 conditions or by section 2; run the closure checks in section 2 and the corpora and resource-proof gates at that exact head.
3. **Publish** the reviewed head under the separate execution assignment (section 5).
4. **Ordinary public resolution verifies the same bytes**: the core's `go.sum` hash for the pinned version must equal the hash of the reviewed head's module zip as served by the proxy, checked from a clean module cache.
5. The **core pin change**, on Builder's existing `#4043` promise, is reviewed against all five callers: both `require` lines, both `go.sum` files, no `replace`, no `GOPRIVATE`, `go test ./...` green in both modules.

## 5. Authority still needed

1. A **proposal adopted** in the tailapp workroom naming owner, repository, module path, visibility, history-preserving fork point, notice handling and pin form (section 6 is the candidate text).
2. **Repository creation and first publication** performed by a GitHub organization owner or admin under **one separate execution request**, filed only after the route decision is adopted. This is the only new assignment the route needs.
3. The **immutable evaluator pin and the core caller migration stay on Builder's existing `#4043` promise**. No second core-pin request; 933cbc53's item 4 is withdrawn as a duplicate of the commissioned outcome.

Nothing in this report authorizes creation, a public push, a pin, provider admission or upstream contact.

## 6. Candidate decision, for adoption by the ratifier

> Publish the stage-1 metered evaluator as a public organization-maintained fork.
> Create `github.com/generalbusiness-ai/jsonata`, public, with upstream git history preserved from `599f35f32e5f31297f8b2153b5da44ddbb330e28`.
> Set the module path to `github.com/generalbusiness-ai/jsonata` and rewrite every self-import in the tree, including the root adapter, `v154` and `exerciser`, so no retained package binds to `github.com/jsonata-go/jsonata`; the core imports `github.com/generalbusiness-ai/jsonata/v206`.
> Keep upstream `LICENSE` byte-identical and add a NOTICE naming the upstream repository, the fork commit and the fork's purpose.
> Pin by ordinary Go resolution at one immutable revision, verified through proxy.golang.org and sum.golang.org.
> Conditions: the published revision is the complete evaluator head reviewed locally at its exact head before first publication, and every change from `599f35f3` is justified by the stage-1 conditions or by the rename, notice and packaging treatment above; the self-import closure checks pass; corpora and resource proof gates are green at that head; public resolution yields the reviewed bytes; the pin change is reviewed against all five callers on the existing #4043 promise.
> Prohibitions: no delivered `replace` directive, no `GOPRIVATE`, no re-pointed tag, no language or resource-rule change, no host activation, no upstream contact required by this decision.
> Creation and first publication require one separate execution request performed by a GitHub organization admin; contributing the patch upstream stays optional and separate.
