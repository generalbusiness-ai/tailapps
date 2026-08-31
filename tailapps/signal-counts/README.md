# Signal counts

This deliberately small Tailapp counts every canonical OTLP record by source,
signal, and event name. It accepts logs, spans, and metric points without
retaining the source records. `first_seen_unix_nano` and
`last_seen_unix_nano` preserve the source timestamp, or the observed timestamp
fallback, when one is present; either is null when the producer sent neither.

Install and first-activate it in one request:

```sh
tailapp apps install --bundle signal-counts \
  --idempotency-key install-signal-counts-v1 signal-counts
```

Then query its exported table:

```sh
tailapp query \
  --sql 'SELECT source, signal, event_name, event_count, first_seen_unix_nano, last_seen_unix_nano FROM signal_counts ORDER BY source, signal, event_name' \
  signal-counts
```

The source directory remains a compact template for authoring your own
Tailapp.

Adding the timestamp columns changes this writable table's storage shape. An
older installed copy therefore requires reset activation to upgrade, which
discards its prior counts.

For this release, upgrade the complete changed source set: `application.sql`,
`folds/normalize.jsonata`, and `folds/count.jsonata`. The canonical upgrade
procedure derives every release's source set mechanically from
`git diff --name-only main..HEAD -- tailapps`, retaining only `application.sql`
and `folds/*.jsonata`; follow the [resident upgrade procedure](../../docs/reference/cli.md#upgrading-an-existing-resident)
rather than putting only the files suggested by a table change.
