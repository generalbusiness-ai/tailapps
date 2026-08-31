# Dependency and vulnerability checks

Tailapps pins its direct and selected transitive Go modules in `go.mod` and
records their exact module checksums in `go.sum`. Refreshes use `go get` with
explicit compatible versions followed by `go mod tidy`; they are not an
unbounded `-u` upgrade.

Run the same reachable-code vulnerability scan locally and in CI with:

```sh
scripts/vulncheck.sh
```

The script pins `govulncheck` at `v1.7.0` and defaults to `./...`. It fails
when the Go vulnerability database finds an advisory reachable through this
repository's compiled code. The analyzer can also report package- and
module-level advisories that are not currently called; review those on each
dependency refresh instead of treating them as a clean bill of health.

The scan needs network access to download the pinned analyzer when it is not
already cached and to retrieve the current Go vulnerability database. CI runs
it after tests and vet, before the demonstration step.
