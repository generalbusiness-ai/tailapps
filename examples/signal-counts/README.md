# Signal counts

This deliberately small Tailapp counts every canonical OTLP record by source,
signal, and event name. It accepts logs, spans, and metric points without
retaining the source records.

Install and first-activate it in one request:

```sh
tailapp apps install \
  --idempotency-key install-signal-counts-v1 \
  signal-counts examples/signal-counts
```

Then query its exported table:

```sh
tailapp query \
  --sql 'SELECT source, signal, event_name, event_count FROM signal_counts ORDER BY source, signal, event_name' \
  signal-counts
```

`README.md` is author documentation and is not installed as executable source;
the install command reads only `application.sql` and `folds/*.jsonata`.
