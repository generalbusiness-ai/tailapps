# First-time resident setup

For a published release on macOS or Linux, use its pinned `install.sh` asset.
It installs the binary under `~/.local/lib/tailapp`, links
`~/.local/bin/tailapp`, starts a no-sudo user service, and installs all five
missing built-in bundles without a prompt:

```sh
curl -fsSL https://github.com/generalbusiness-ai/tailapps/releases/download/vVERSION/install.sh | sh
```

Select a subset with `--bundles`, omit bundle installation with `--bundles
none`, or request a terminal confirmation with `--interactive`. The
interactive option refuses when `/dev/tty` is unavailable; the default never
prompts. To retain the binary and requested bundles but deliberately configure
no service, add `--no-service`; it prints the exact foreground remedy.

`scripts/setup-resident.sh` remains the source-checkout path for people
building this repository locally. It has the same user-owned location and
default bundle set, but builds from the checkout rather than verifying a
published release.

Review the exact local paths and service before changing anything:

```sh
scripts/setup-resident.sh --dry-run
```

Run source-checkout setup:

```sh
scripts/setup-resident.sh
```

The canonical default engine home is `~/.local/share/tailapp`, and the
loopback receiver is `127.0.0.1:4318`. This is deliberate: Tailapps does not
use the platform-specific `os.UserConfigDir` default, so the binary, release
installer, source setup path, services, harnesses, and MCP clients agree.
Override it only when all harnesses and MCP clients will use the same value:

```sh
scripts/setup-resident.sh \
  --home /absolute/path/to/tailapp-home \
  --otlp 127.0.0.1:4318
```

The released installer creates or validates
`~/Library/LaunchAgents/ai.generalbusiness.tailapp.plist` and starts the
`ai.generalbusiness.tailapp` LaunchAgent. On Linux it creates or validates
`~/.config/systemd/user/tailapp.service`, enables it with `systemctl --user`,
and asks `loginctl` to enable lingering so the user service survives logout.
Some Linux hosts forbid lingering by policy; the command reports that condition
instead of claiming boot persistence it could not establish.

The script refuses to overwrite a service whose binary path, engine home, or
receiver differs from its requested configuration. It also refuses to replace
an existing `~/.local/bin/tailapp` unless it is an absolute symlink, protecting
a manually installed binary. Re-running with the same configuration is
idempotent: it starts the service, preserves existing custom Tailapps, and
installs only missing built-in bundles. It never activates, resets, updates,
or deletes an existing Tailapp.

Check the result with:

```sh
~/.local/bin/tailapp health
~/.local/bin/tailapp apps list
```

After a resident is healthy, `tailapp setup --bundles LIST|none` is the
machine-readable, in-binary form of the bundle-only step. It uses the normal
create-only control operation, so it reports existing names instead of
overwriting or activating them.
