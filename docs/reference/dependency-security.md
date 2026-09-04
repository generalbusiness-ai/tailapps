# Dependency and vulnerability checks

Tailapps pins its direct and selected transitive Go modules in `go.mod` and
records their exact module checksums in `go.sum`. Refreshes use `go get` with
explicit compatible versions followed by `go mod tidy`; they are not an
unbounded `-u` upgrade.

The independently published `jsonataddl` module has its own `go.mod` and
`go.sum`. Run `go mod tidy` in both module directories and review both sums.
The root module's `replace` keeps Tailapps on the same local core source during
development; published `jsonataddl` consumers do not use it.
Before pushing a module tag, run
`scripts/check-jsonataddl-version-available.sh vVERSION`. It uses bounded
HTTPS requests and admits publication only after the exact `origin` tag is
absent, the public Go proxy's version list lacks the exact version, and the
checksum database returns a verified not-found response. It never probes the
proxy's version-specific `.info` endpoint before publication: doing so for
v0.1.1 created a 30-minute negative cache that outlived the successful tag and
release workflow. Existing records, missing tools, transport failures,
unexpected HTTP results, and malformed, duplicate, or mismatched output all
fail closed. Deterministic tests inject the remote and HTTP clients and prove
the absent path cannot call the cache-poisoning endpoint.
`scripts/verify-jsonataddl-module.sh` proves a tagged module through a clean
consumer with workspace mode disabled and refuses a resolved replacement. It
clears inherited Go flags and the `GOPRIVATE`, `GONOPROXY`, `GONOSUMDB`, and
`GOINSECURE` bypasses, then requires `sum.golang.org`, so the public-proxy
proof cannot silently skip checksum verification because of the caller's
environment.

Run the same reachable-code vulnerability scan locally and in CI with:

```sh
scripts/vulncheck.sh
(cd jsonataddl && ../scripts/vulncheck.sh)
```

The script pins `govulncheck` at `v1.7.0` and defaults to `./...`. It fails
when the Go vulnerability database finds an advisory reachable through this
repository's compiled code. The analyzer can also report package- and
module-level advisories that are not currently called; review those on each
dependency refresh instead of treating them as a clean bill of health.

The scan needs network access to download the pinned analyzer when it is not
already cached and to retrieve the current Go vulnerability database. CI runs
it after tests and vet, before the demonstration step.
