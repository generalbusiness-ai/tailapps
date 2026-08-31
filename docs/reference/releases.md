# Verified GitHub releases

Pushing an exact semantic-version tag such as `v0.1.0` runs the release
workflow only when that tag’s commit is already reachable from `main`. Cut
releases from merged `main`, not from a feature branch. The workflow tests the
tagged source, builds `darwin/arm64`, `darwin/amd64`, `linux/arm64`, and
`linux/amd64` archives, publishes them with `checksums.txt` and the equivalent
`SHA256SUMS`, and attaches a keyless Cosign signature and bundle for that
checksum manifest. Every archive
contains the `tailapp` binary, `LICENSE`, and `NOTICE`. The workflow also
creates GitHub build-provenance attestations for every archive.

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
It prints the next foreground command; durable setup remains a separate,
explicit operation.

For an independent GitHub-hosted provenance check, use GitHub CLI:

```sh
gh attestation verify ARCHIVE --repo generalbusiness-ai/tailapps
```

The release workflow receives its signing identity from GitHub Actions OIDC;
it uses no long-lived signing key or repository secret. Tag pushes publish
signed releases; pull requests run the unsigned archive-and-installer dry run
only.
