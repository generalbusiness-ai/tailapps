# Dependency and vulnerability checks

Tailapps pins its direct and selected transitive Go modules in `go.mod` and
records their exact module checksums in `go.sum`. Refreshes use `go get` with
explicit compatible versions followed by `go mod tidy`; they are not an
unbounded `-u` upgrade.

The independently published `jsonataddl` module has its own `go.mod` and
`go.sum`. Run `go mod tidy` in both module directories and review both sums.
The root module's `replace` keeps Tailapps on the same local core source during
development; published `jsonataddl` consumers do not use it.
`scripts/verify-jsonataddl-module.sh` proves a tagged module through a clean
consumer with workspace mode disabled and refuses a resolved replacement. It
clears inherited Go flags and private-module bypasses, then requires
`sum.golang.org`, so the public-proxy proof cannot silently skip checksum
verification because of the caller's environment.

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
