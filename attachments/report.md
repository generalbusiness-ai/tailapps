Checker recommendation report for tailapp request #4211 (82e3d5fc…9d4bc99f), filed 2026-09-07 against promise fc6503ba. Planning only: no repository created, no upstream contact, nothing published, no pin or module-path change.
Facts re-checked directly by checker before filing: patch head e9fa318e is 78 commits and 133 files (+19380/-18) over base 599f35f3, all under v206/, with go.mod, go.sum and LICENSE unchanged and no v206/go.mod; the five shared-core callers listed below; generalbusiness-ai/jsonata returns 404; upstream jsonata-go/jsonata has zero pull requests, zero issues and zero forks ever; hughpyle is an active organization admin (a fact about the account, not authority for any agent). The tracked file v206/.claude/settings.local.json is upstream's own from its first commit, not introduced by the patch; it should still be reviewed before publication under the organization's name.

# Publication route recommendation for the I7 stage-1 metered evaluator

Read-only planning answer to tailapp #4211. No repository was created, no upstream contact made, nothing published, no pin or module path changed.

## 1. Facts

**CONFIRMED — the patch** (read-only in `/Users/hughpyle/play/tailapp-worktrees/i7-metered-evaluator`, branch `request/i7-metering-proof`, head `e9fa318e42ca830063ead9f669f6d75faa4ffba1`, base `599f35f32e5f31297f8b2153b5da44ddbb330e28`):
- `git log --oneline 599f35f3..HEAD | wc -l` = **78** commits.
- `git diff --stat 599f35f3..HEAD | tail -1` = **133 files changed, 19380 insertions(+), 18 deletions(-)**.
- Name-status split: **130 added, 3 modified**. The three modified files are `v206/functions.go`, `v206/jsonata.go`, `v206/utils.go`.
- Every one of the 133 changed paths is under **`v206/`**. No other version directory (`v154`), no root adapter, no `exerciser` file is touched.
- `git diff --stat 599f35f3..HEAD -- go.mod go.sum LICENSE` is **empty**: none of the three changed.
- `grep -rn "^replace" --include=go.mod` finds exactly one line, `exerciser/go.mod:7: replace github.com/jsonata-go/jsonata => ../`. It is present identically at the base, is upstream's own, and `exerciser` is a separate module that the core does not import. The patch introduces no `replace`.

**CONFIRMED — shared-core callers** (`grep -rl --include='*.go' 'jsonata-go/jsonata' .` in `/Users/hughpyle/play/tailapp`). Five files, two modules, all importing only the `/v206` subpackage:

| File | Line | Module |
|---|---|---|
| `internal/projection/projection_test.go` | 13, `jsonatav206 "github.com/jsonata-go/jsonata/v206"` | `github.com/generalbusiness-ai/tailapps` |
| `jsonataddl/application.go` | 9, `jsonata "…/v206"` | `github.com/generalbusiness-ai/tailapps/jsonataddl` |
| `jsonataddl/compile.go` | 15, `jsonata "…/v206"` | jsonataddl |
| `jsonataddl/confine.go` | 9, `jsonata "…/v206"` | jsonataddl |
| `jsonataddl/evaluate.go` | 10, `jsonatav206 "…/v206"` | jsonataddl |

Both `go.mod` files require `github.com/jsonata-go/jsonata v0.0.0-20250709164031-599f35f32e5f`, and both `go.sum` files carry its two hashes.

**CONFIRMED — upstream.** `jsonata-go/jsonata` is public, MIT (`Copyright (c) 2021 Blues Inc`), not archived, default branch `master`, 9 stars, `pushed_at` 2025-07-09. `gh api repos/jsonata-go/jsonata/pulls?state=all` returns **zero pull requests ever**; `issues?state=all` returns **zero issues ever**; `repos/jsonata-go/jsonata/forks` returns **zero forks**; `orgs/jsonata-go/public_members` returns **nobody**. Org created 2025-06-07, one public repo, description "Go port of jsonata-js". `repos/jsonata-go/jsonata/collaborators` is **403 Must have push access** — the maintainer roster is not observable. CONTRIBUTING.md is a Blues Inc fork-and-PR document requiring tests. My permissions: `pull` only. No tags, no releases; the proxy has no version list, only the pseudo-version above.

**CONFIRMED — Go module rules.** `go help private` (go1.27.0, local): the go command defaults to proxy.golang.org and validates everything against sum.golang.org; `GOPRIVATE` (with `GONOPROXY`/`GONOSUMDB`) marks paths that bypass both. go.dev/ref/mod: "The repository root path is the portion of the module path that corresponds to the root directory of the version control repository where the module is developed"; pseudo-versions are `vX.0.0-yyyymmddhhmmss-abcdefabcdef` when no base version is known and "function as ordinary versions"; from v2 a module path needs a `/vN` suffix. **`v206/` has no `go.mod` of its own**, so it is not a separate module and not a Go major version — it is an ordinary import subpath of a `v0` module, and Go imposes no `/v206` suffix rule on it.

**CONFIRMED — destination.** `generalbusiness-ai/jsonata` and `generalbusiness-ai/jsonata-go` both return **404** to an active org **admin**, so neither exists. The org already publishes Go modules from GitHub: `github.com/generalbusiness-ai/tailapps/jsonataddl` is on the proxy at v0.1.0, v0.1.1, v0.1.2, v0.2.0.

**OWNER-DEPENDENT.** Whether upstream would accept the patch, on what timescale, or at all. Who the upstream maintainers are. Whether the org wants to own a JSONata fork. Which human performs repository creation. Repository visibility. Whether a tag or a pseudo-version is the pin. Everything in section 5.

## 2. Route A, upstream acceptance

What it would take: fork `jsonata-go/jsonata`, open a PR of 78 commits and 19,380 added lines against a repository that has **never had a single pull request or issue** and has not been pushed in 14 months, and wait for a maintainer whose identity is not observable to review it, then wait again for a tagged or pushed revision to pin.

The advantage is real and singular: **the module path never changes**, so the two `go.mod` files and all five caller files stay exactly as they are, and provenance is upstream by construction.

It is not supportable as stage 1's pinning route. Acceptance is entirely outside the checker's and the org's power, has no observed precedent in this repository, and has no bounded timescale; the patch is also still incomplete, so there is nothing complete to submit today. The stage-1 decision requires "a reviewed immutable evaluator revision and … its ordinary exact dependency pin for final admission tests", and a route whose completion depends on a silent third party cannot supply that. Route A stays open as a **later, optional** contribution-back, not as the pin.

## 3. Route B, organization-maintained fork

- **Owner/name: `generalbusiness-ai/jsonata`.** It matches the upstream repository's own name and the module path's last element, which keeps `go get` output legible; `jsonata-go` would name the upstream *organization*, not the module, and read as a different project. Confirmed free (404).
- **Module path = import path root: `github.com/generalbusiness-ai/jsonata`.** The `module` line in the root `go.mod` changes to that string; nothing else in the root `go.mod` needs to change. `exerciser/go.mod` (module line and its `replace`) is rewritten the same way or the directory is dropped; it is not imported by the core either way.
- **Core import path: `github.com/generalbusiness-ai/jsonata/v206`.** No `/vN` obligation applies, since the fork stays at v0 and `v206` is a plain subdirectory.
- **Caller change:** the `require` line in `/Users/hughpyle/play/tailapp/go.mod` and in `/Users/hughpyle/play/tailapp/jsonataddl/go.mod`, their `go.sum` entries, and the single import line in each of the five files above. No aliases change, because all five already alias the import.
- **Immutable pin:** publish public, so proxy.golang.org and sum.golang.org verify it and `go.sum` pins the content hash with no `GOPRIVATE` and no `replace`. Prefer an **annotated tag** (`v0.1.0-meter.1`) for readability; a `v0.0.0-<ts>-<sha12>` pseudo-version is equally immutable and needs no tag decision. Note the `retract v0.1.0` line already in `jsonataddl/go.mod`: once bytes are published under a version they are frozen in the checksum database, so never re-point a tag.
- **Provenance and licence:** fork by pushing the existing git history, not by copying a tree, so `599f35f3` is a real ancestor and `git merge-base` proves it. Keep `LICENSE` byte-identical (the patch does not touch it) and `v206/jsonata-js/LICENSE` untouched. Add a `NOTICE` naming upstream `github.com/jsonata-go/jsonata`, the exact fork point `599f35f32e5f31297f8b2153b5da44ddbb330e28`, the MIT terms as retained, and the fork's purpose (deterministic work and allocation metering for tailapp stage 1).
- **Maintenance:** the org owns fixes, security response and any upstream resync; contributing back is optional and separate.

## 4. Recommendation

**Route B**, a public `generalbusiness-ai/jsonata` fork. It is the only route whose completion is inside the org's control, it satisfies the stage-1 requirement of an ordinary exact pin with no delivered `replace`, and its one real cost — a module path change across two `go.mod` files and five caller files — is small, fully enumerated above, and reviewable.

## 5. Authority still needed

1. A **proposal adopted** in the tailapp workroom naming: owner `generalbusiness-ai`, repository `jsonata`, module path `github.com/generalbusiness-ai/jsonata`, public visibility, history-preserving fork from `599f35f3`, licence and NOTICE handling, and the pin form.
2. **Repository creation** by a GitHub org owner or admin. `members_can_create_repositories` is true and `hughpyle` is an active org admin (CONFIRMED), but that is a fact about the human account, **not** authority for any agent to act. Who creates it, and on whose instruction, is OWNER-DEPENDENT.
3. A **separate execution request** for creation and first push, distinct from the #4043 implementation promise, which stays with builder.
4. A further **request for the core pin change**, which must not begin before the fork revision is reviewed.

Nothing in this report authorizes step 2, 3 or 4.

## 6. Review plan for the module path change

1. Review the **exact patch at the fork's immutable revision**: diff the fork's pinned SHA against `599f35f3` and confirm it equals the reviewed 78 commits / 133 files, all under `v206/`, plus only the module-path rename.
2. **Provenance and licence check**: `git merge-base` proves `599f35f3` is an ancestor; `LICENSE` and `v206/jsonata-js/LICENSE` byte-identical; NOTICE present and accurate; the pre-existing tracked `v206/.claude/settings.local.json` reviewed before it is published under the org's name.
3. **Corpora and resource proof gates** run at that revision, per the stage-1 metering conditions, not at the worktree head.
4. Only then, a **core pin change** reviewed against every caller in the section 1 table, checking that both `go.mod` requires and both `go.sum` files resolve through the public proxy with no `replace`, no `GOPRIVATE`, and `go test ./...` green in both modules under the existing `go.work`.

## 7. Proposed decision, ready to adopt

> Publish the stage-1 metered evaluator as a public organization-maintained fork.
> Create `github.com/generalbusiness-ai/jsonata`, public, with upstream git history preserved from `599f35f32e5f31297f8b2153b5da44ddbb330e28`.
> Set the module path to `github.com/generalbusiness-ai/jsonata`; the core imports `github.com/generalbusiness-ai/jsonata/v206`.
> Keep upstream `LICENSE` byte-identical and add a NOTICE naming the upstream repository, the fork commit and the fork's purpose.
> Pin by ordinary Go resolution at one immutable revision, verified through proxy.golang.org and sum.golang.org.
> Conditions: the fork revision differs from `599f35f3` only by the reviewed patch and the module-path rename; provenance, licence and NOTICE reviewed; corpora and resource proof gates green at that revision; the pin change separately reviewed against all five callers.
> Prohibitions: no delivered `replace` directive, no `GOPRIVATE`, no re-pointed tag, no language or resource-rule change, no host activation, no upstream contact required by this decision.
> Creation and first push require a separate execution request performed by a GitHub organization admin; contributing the patch upstream stays optional and separate.