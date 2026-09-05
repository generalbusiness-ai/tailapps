# Verified GitHub releases

Pushing an exact semantic-version tag such as `v0.1.0` runs the release
workflow only when that tag’s commit is already reachable from `main`. Cut
releases from merged `main`, not from a feature branch. The workflow tests the
tagged source, builds `darwin/arm64`, `darwin/amd64`, `linux/arm64`, and
`linux/amd64` archives, publishes them with `checksums.txt` and the equivalent
`SHA256SUMS`, and attaches a keyless Cosign signature and bundle for that
checksum manifest. It also publishes version-pinned `install.sh` and
`upgrade.sh` assets. Every archive
contains the `tailapp` binary, `LICENSE`, and `NOTICE`. The workflow also
creates GitHub build-provenance attestations for every archive.

The declared-input contract targets the separately reviewed nested-module
release `jsonataddl/v0.2.0`. Hosts moving from v0.1.2 must add their complete
`Input` contracts and positive `Limits.MaxInputDepth`, then re-pin their
composed runtime identities. Publication, consumer adoption and production
activation remain separate gates. Version v0.1.0 is permanently unusable: its
published bytes conflict with the immutable public checksum-database record.
Do not move or reuse its tag.

Prepare the exact merged source, version, module and go.mod checksums, and
composed identity for independent review. A module's README is part of its
immutable archive and must describe that version. After exact approval and
ratification, immediately before pushing the nested-module tag, run:

```sh
scripts/check-jsonataddl-version-available.sh v0.2.0
```

Proceed only when the command verifies that the exact tag is absent from
`origin`, the exact version is absent from `proxy.golang.org`'s version list,
and `sum.golang.org` has no record for it. The proxy check deliberately does
not request the version-specific `.info` endpoint: the v0.1.1 preflight showed
that its not-found response can remain negatively cached for 30 minutes after
the tag is published. A missing tool, network failure, malformed, duplicate,
mismatched, or unexpected response, or existing record refuses the release.
Publish only the approved `jsonataddl/v0.2.0` tag at its exact reviewed merged
source. Repository rules protect the `jsonataddl/v*` namespace from tag updates
and deletion. The tag workflow is a post-publication check, not a gate that
prevents consumers resolving the tag; run its module gates before publication
and require the actual workflow to pass afterwards.

Verify the published version from an external module with `GOWORK=off`, fresh
module/build caches, no replacement and no private proxy exemption. Run
`scripts/verify-jsonataddl-module.sh v0.2.0`; separately obtain
`go mod download -json github.com/generalbusiness-ai/tailapps/jsonataddl@v0.2.0`
and compare `Sum` and `GoModSum` with the reviewed source-derived values.
Record the exact
version, source, checksums, identity and observed public resolution as the
consumer handoff. The verifier runs the packaged example; it does not perform
that expected-checksum comparison itself.

The root module still requires v0.1.2 behind a local replacement in this
source. After v0.2.0 is publicly resolvable, update that requirement in a
separately reviewed source change before cutting another root release. A
consumer does not inherit a dependency's local replacement. Do not claim the
new core is publicly consumable through a root release while its requirement
still selects the older version.

Download the archive matching your operating system and CPU together with
`checksums.txt`, `checksums.txt.sig`, and `checksums.txt.bundle` from the same
GitHub release. Verify the signature before trusting the checksums:

```sh
cosign verify-blob \
  --bundle checksums.txt.bundle \
  --certificate-identity-regexp '^https://github.com/generalbusiness-ai/tailapps/.github/workflows/release.yml@refs/tags/v[0-9].*$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum --check checksums.txt
```

On macOS, replace the last command with `shasum -a 256 ARCHIVE` and compare its
digest to the matching line in `checksums.txt` when `sha256sum` is unavailable.
The signed checksum manifest covers the exact archive bytes. Then extract the
archive and use the included `tailapp` binary.

For a verified, version-pinned per-user install on macOS or Linux, download
the `install.sh` asset from the same release (not the checkout copy):

```sh
curl -fsSL https://github.com/generalbusiness-ai/tailapps/releases/download/vVERSION/install.sh | sh
```

The rendered asset contains that release's exact version. It selects the
matching archive, downloads `checksums.txt`, its signature and bundle, and
verifies the archive SHA-256 before installing it as
`~/.local/lib/tailapp/tailapp-VERSION` and linking `~/.local/bin/tailapp`.
When `cosign` is available it also verifies the keyless checksum-signature
identity for that exact tag. It never uses sudo or changes telemetry settings.
It configures the no-sudo per-user resident and installs all five missing
built-in bundles without prompting. Use `sh -s -- --bundles LIST|none` after
the pipe to select bundles, `--interactive` only when `/dev/tty` is available,
or `--no-service` to retain the binary and bundles with an explicit foreground
remedy instead of a service. See [first-time setup](first-time-setup.md) for
service status, logs, and the source-checkout path.

The companion `upgrade.sh` consumes the same verified release without changing
bundles, Tailapp source, activation, projections, or telemetry configuration.
It preserves a known-good binary and emits machine-readable control-plane and
ingestion readiness. See [resident upgrades](resident-upgrade.md).

For an independent GitHub-hosted provenance check, use GitHub CLI:

```sh
gh attestation verify ARCHIVE --repo generalbusiness-ai/tailapps
```

The release workflow receives its signing identity from GitHub Actions OIDC;
it uses no long-lived signing key or repository secret. Tag pushes publish
signed releases; pull requests run the unsigned archive-and-installer dry run
only.
