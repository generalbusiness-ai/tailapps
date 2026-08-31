# First-time resident setup

On macOS and Linux, `scripts/setup-resident.sh` builds Tailapp from the current
checkout, places a versioned binary under `~/.local/lib/tailapp`, links
`~/.local/bin/tailapp` to it, starts a no-sudo user service, and installs any
missing built-in bundles.

Review the exact local paths and service before changing anything:

```sh
scripts/setup-resident.sh --dry-run
```

Run the setup:

```sh
scripts/setup-resident.sh
```

The default engine home is `~/.local/share/tailapp`, and the loopback receiver
is `127.0.0.1:4318`. Override either only when all harnesses and MCP clients
will use the same value:

```sh
scripts/setup-resident.sh \
  --home /absolute/path/to/tailapp-home \
  --otlp 127.0.0.1:4318
```

On macOS it creates or validates
`~/Library/LaunchAgents/ai.generalbusiness.tailapp.plist` and starts the
`ai.generalbusiness.tailapp` LaunchAgent. On Linux it creates or validates
`~/.config/systemd/user/tailapp.service`, enables it with `systemctl --user`,
and asks `loginctl` to enable lingering so the user service survives logout.
Some Linux hosts forbid lingering by policy; the command reports that condition
instead of claiming boot persistence it could not establish.

The script refuses to overwrite a service whose binary path, engine home, or
receiver differs from its requested configuration. Re-running with the same
configuration is idempotent: it starts the service, preserves existing custom
Tailapps, and installs only missing built-in bundles. It never activates,
resets, updates, or deletes an existing Tailapp.

Check the result with:

```sh
~/.local/bin/tailapp health
~/.local/bin/tailapp apps list
```
