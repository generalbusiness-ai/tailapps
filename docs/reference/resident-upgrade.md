# Upgrade the resident

For an installed release, use the same release's pinned `upgrade.sh` asset.
It verifies that asset against the signed `checksums.txt` manifest (and the
keyless Cosign bundle when available), stages the verified release through the
release installer, preserves the known-good binary, atomically switches the
stable link, and restarts either launchd or systemd --user:

```sh
curl -fsSL https://github.com/generalbusiness-ai/tailapps/releases/download/vVERSION/upgrade.sh | sh
```

It emits one JSON result. `control_plane: "healthy"` means the resident socket
started. `ingestion_ready: false` is a successful binary upgrade with existing
Tailapps awaiting their explicit source lifecycle; it does not install bundles,
rewrite source, activate apps, reset projections, or change telemetry. Its
`next` field gives `apps status` and the exact upgrade guide. `action` is
`upgraded`, `upgrade_pending`, or `up_to_date`; an explicit successful rollback
reports `rolled_back`. A failed control plane restores the known-good link and
restarts the prior service. Use `--rollback` to select that recorded prior
binary explicitly.

The JSON storage correction changes runtime identity. Existing applications
remain queryable while upgrade-pending, but continuation now checks actual
stored JSON column types. A projection using the old numeric-affinity JSON
storage cannot be continued into the corrected text storage, even if its
source recompiles successfully. Refusal preserves its rows, schema, identity
and frontier. Review the existing data before choosing an explicit,
acknowledged reset; upgrading the binary does not authorize or perform one.
Compatible JSON text storage and unaffected non-JSON projections can still
continue, including compatible additions to their table set.

The source-checkout macOS command below remains supported for building the
current checkout. It is not a release consumer.

## Upgrade the macOS resident from a checkout

`scripts/upgrade-resident-macos.sh` replaces only the binary that the existing
per-user launchd resident executes. It does not touch `TAILAPP_HOME`, Tailapp
source, installed definitions, activation boundaries, SQLite projections, or
telemetry configuration.

The script expects the standard per-user layout:

- `~/.local/bin/tailapp` is an absolute symlink;
- `~/Library/LaunchAgents/ai.generalbusiness.tailapp.plist` runs that exact
  path and sets `TAILAPP_HOME`; and
- the agent is already loaded under `gui/$UID`.

Review the plan first:

```sh
scripts/upgrade-resident-macos.sh --dry-run
```

Then build the current checkout, install a versioned binary under
`~/.local/lib/tailapp`, atomically repoint the executable link, restart the
LaunchAgent, and wait for the existing control socket to become healthy:

```sh
scripts/upgrade-resident-macos.sh
```

The existing binary target is recorded as
`~/.local/lib/tailapp/tailapp.previous`. If the new resident does not become
healthy, the command automatically restores that target and restarts launchd.
You can also explicitly revert a successful upgrade:

```sh
scripts/upgrade-resident-macos.sh --rollback
```

Use `--source DIR`, `--home DIR`, and `--label LABEL` only when the checkout,
engine home, or LaunchAgent label differs from the standard layout. The command
refuses a regular binary, a relative symlink target, a missing agent, or a
plist whose program or `TAILAPP_HOME` does not match its computed target. Those
checks prevent it from silently replacing an unrelated service.

An upgraded binary contains the current built-in bundle definitions, but it
does not silently rewrite installed Tailapp sources. Updating an existing
Tailapp remains the explicit draft/validate/activate lifecycle; a source shape
change may require acknowledged reset activation. Check the resident after an
upgrade with `tailapp health` and `tailapp apps list`.
