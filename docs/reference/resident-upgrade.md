# Upgrade the macOS resident

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
