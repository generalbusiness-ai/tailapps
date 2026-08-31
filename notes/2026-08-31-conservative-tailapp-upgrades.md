---
date: 2026-08-31
status: proposed; Stage A design for review
scope: conservative first-party Tailapp source upgrades
---

# Conservative Tailapp upgrades

## Decision

An ordinary binary upgrade and a Tailapp-source upgrade are separate
operations. Replacing the resident binary must continue to leave definitions,
drafts, activation boundaries, projections, and telemetry configuration alone.
Source upgrades must begin with a read-only, machine-readable plan. The only
automatic source change is an explicitly requested `--apply-safe` operation on
a *pristine built-in* whose target release certifies a compatible `continue`.
Everything else requires an explicit app selection and, when applicable, an
explicit reset confirmation after a successful backup.

The resident will persist first-party origin metadata rather than infer it from
an app name or a table shape. Current installations have no such metadata, so
they classify conservatively as user-generated until an operator explicitly
adopts a baseline; they are never auto-upgraded.

## Durable provenance

Add a registry table, conceptually:

```text
tailapp_bundle_provenance(
  tailapp_name primary key,
  origin                 -- builtin | user
  bundle_name,
  installed_release,
  baseline_revision,
  baseline_source_digest,
  baseline_sources_json,
  recorded_at_unix_nano
)
```

`apps install --bundle NAME` writes an `origin=builtin` row in the same
idempotent control transaction as creation and first activation. It records
the named bundle, the released source manifest digest, the compiled baseline
revision, and a byte-exact baseline source map. The map is essential: it makes
the later three-way comparison independent of a checkout, a Git remote, or an
old release still being available. Empty-source `apps create`, source-map
`apps install`, and pre-existing installations record no built-in provenance
and therefore remain `origin=user`.

Provenance survives both `continue` and `reset`: those replace projection
state, not the definition registry. A deliberate operator action can adopt a
baseline only after displaying the byte-level match and naming the source
manifest; it cannot guess that an app named `daily-review` is first-party.

Each signed release carries a small built-in manifest containing its version,
bundle names, source-map digests, compiled revision, and a per-bundle
`continue_safe_from` list. The manifest is an input to planning, not a claim
that any installed source is safe.

## Read-only plan

`tailapp apps upgrade-plan --release MANIFEST [--app APP]` returns JSON and
performs no draft mutation, activation, reset, or service restart. For every
selected app it reports these independent dimensions:

```json
{
  "name": "daily-review",
  "classification": "pristine_builtin",
  "provenance": {
    "bundle": "daily-review",
    "installed_release": "v1.4.0",
    "baseline_source_digest": "sha256:…"
  },
  "runtime": {"state": "upgrade_pending", "current": "sha256:…"},
  "target": {"release": "v1.5.0", "source_digest": "sha256:…"},
  "compatibility": {"mode": "continue", "certified": true},
  "action": "eligible_for_apply_safe"
}
```

`classification` is exactly one of:

| Classification | Test | Default action |
| --- | --- | --- |
| `user_generated` | no built-in provenance, including legacy installs | report only |
| `pristine_builtin` | current source map exactly equals its recorded baseline | compare with the target manifest |
| `modified_builtin` | built-in provenance exists but the current source map differs from the recorded baseline | report a three-way diff or conflict; never mutate |

Runtime compatibility is separate from classification. The plan says whether
the currently running binary has the app upgrade-pending, whether the target
source compiles under that runtime, and whether the target release certifies
`continue`. It must never infer `continue` safety from equal-looking SQL,
table shape, or a successful validation.

For a modified built-in, the plan emits bounded files for the three named
inputs: `baseline` (the stored baseline map), `current` (the app draft), and
`upstream` (the target release map). It reports paths changed on each side and
an explicit conflict list. It offers no merge and does not write a draft. The
operator can make an intentional edit through the existing optimistic
`apps put`/`apps remove` lifecycle, then request a new plan.

## Applying an upgrade

`tailapp apps upgrade --apply-safe --release MANIFEST [--app APP]` is
deliberately narrow:

1. Re-read provenance, the exact draft revision, the release manifest, and the
   plan under one control lock.
2. Select only `pristine_builtin` apps whose manifest names the installed
   baseline and certifies `continue` to the exact target source digest.
3. Put the target source map through the ordinary optimistic draft API,
   validate it, and activate it with explicit `--mode continue` and the
   captured expected revision.
4. Return one JSON result per app, including skipped classifications and the
   activation boundary.

Any revision race, validation failure, certification mismatch, or failed
continue leaves the app untouched and makes the affected result non-success.
It does not fall back to reset. User-generated and modified apps are skipped
even when explicitly named with `--apply-safe`; an explicit name narrows the
plan, it does not grant mutation authority.

Reset-requiring or non-certified changes use a separate command, for example
`tailapp apps upgrade --app APP --mode reset --backup-dir DIR --ack-reset`.
The command requires an explicit app name, target manifest, reset acknowledgement,
and a successful preflight backup; it never applies to a wildcard selection.
`--mode continue` is equally explicit for a non-certified target and requires
the operator to acknowledge that the manifest did not certify it. Both paths
print the exact chosen mode and source digests in JSON.

Drafts are protected throughout: no operation replaces a draft that differs
from the expected revision, and an incomplete update never activates. A
binary-only resident upgrade continues to do none of these operations.

## Backup and restore before a nontrivial change

Before reset or any explicit non-certified source change, the command requires
an operator-selected absolute `--backup-dir`. It preflights that directory by
creating a private staging directory, checking available capacity against the
control database, inbox, projections, and definition source bytes plus a
documented margin, and refusing errors, symlinks that escape the selected
directory, or insufficient space.

The backup is an immutable, timestamped directory with a manifest of SHA-256
hashes, resident version/runtime identity, app revisions/provenance, and the
captured engine files. It is created before the activation journal begins. A
restore command accepts that manifest, verifies every digest, requires the
resident to be stopped, moves the current engine home aside rather than
overwriting it, restores the snapshot, and starts the user service only after
control-plane health succeeds. It reports the old and restored paths. Failed
backup or restore preflight changes no app and starts no reset.

This is intentionally a user-chosen local backup, not an automatic hidden
archive. The restore guide will state the service stop/start commands for
launchd and systemd user units and the foreground fallback.

## Implementation and test plan

Stage B should be split into reviewable changes:

1. Registry migration, provenance model, release-manifest parser, and
   read-only plan. Test legacy/user, pristine, modified, missing/invalid
   provenance, and byte-exact baseline persistence across continue/reset.
2. `--apply-safe` control operation. Test certified continuation, every
   classification skip, stale draft revisions, failed compile/validation,
   activation failure rollback, and the guarantee that no reset occurs.
3. Explicit continue/reset and backup/restore operation. Test required
   selection/acknowledgement/backup location, capacity and permission failure,
   backup hash verification, failed restore rollback, and projection data loss
   only on acknowledged reset.
4. CLI, MCP, JSON schemas, docs, and integration tests against a runtime
   upgrade-pending resident. Test that binary-only upgrades preserve custom
   apps and data and that `ingestion_ready` becomes true only after the
   operator’s chosen app lifecycle actions.

Every stage tests pristine built-ins, modified built-ins, user-generated apps,
multiple apps with a partial failure, and a restart between preparation and
activation. It also proves no automatic selection can overwrite a custom
source or reset a projection.

## Relationship to the current procedure and live restore

The existing [`cli.md`](../docs/reference/cli.md#upgrading-an-existing-resident)
procedure is a manual, release-specific recovery guide: it tells the operator
to obtain matching sources, chain expected draft revisions, and choose
continue or acknowledged reset. It remains the correct procedure until this
design is implemented. The planned command replaces its hard-coded bundle
list with signed release metadata and a read-only plan; it does not reinterpret
the documented steps as permission to automate them.

The recent resident recovery demonstrated the useful distinction between a
healthy control plane and `ingestion_ready`: compatible source continuations
restored intake without resetting data, while schema-changing bundles needed
an explicit choice. That recovery was operator-directed and source-specific;
it did not establish durable built-in provenance and must not be treated as an
automatic-upgrade precedent.
