# Runtime metrics reference

The `tailapp` engine exposes one versioned, payload-free operational snapshot
through:

```sh
tailapp metrics --json
```

An agent reads the same object with the MCP tool `tailapp_metrics`. The output
is JSON in both cases. Poll two snapshots and take counter or cumulative-bucket
deltas to measure rates and latency distributions during active use.

## Lifetime and compatibility

`version` is `tailapp.metrics/v1`. Field meanings and histogram boundaries are
stable within that version. `started_at`, `generated_at`, and `uptime_seconds`
identify the measurement epoch.

Process counters and histograms reset when `tailapp serve` restarts. The
`tailapps.*.durable` consumed, ineffective, and emitted totals live in each
projection database and survive a resident restart. A reset activation starts
a new projection and therefore resets those three durable totals for that
Tailapp.

There is no reset operation for runtime metrics. Sampling is read-only. The
control measurement for a `metrics` request completes after its response is
encoded, so that request first appears in the following snapshot.

## Timing boundaries

Every duration uses these fixed cumulative millisecond buckets:

```text
1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, +Inf
```

Each histogram reports `count`, `sum_milliseconds`, and cumulative `buckets`.
The boundaries mean:

- `intake.duration_milliseconds`: the complete OTLP/HTTP handler request;
- `intake.durable_accept_duration_milliseconds`: active-consumer lookup and
  the bounded inbox transaction, including its engine-lock wait;
- `processing.APP.queue_delay_milliseconds`: durable acceptance to the start
  of that Tailapp's processing attempt;
- `processing.APP.duration_milliseconds`: projection processing through
  obligation settlement, gap detachment, or retry return;
- `queries.engine_lock_wait_milliseconds`: time waiting for the aligned engine
  snapshot barrier;
- `queries.duration_milliseconds`: lock wait, sandbox setup, SQLite execution,
  result conversion, and completion; and
- `control.OPERATION.duration_milliseconds`: private control request parsing,
  dispatch, and response encoding.

Successful processing is counted only after the inbox obligation commits as
consumed. A projection gap is counted after its remaining obligations detach.
A transient failure or failed settlement is a `retry`, not a success.

## Intake and loss indicators

`intake` contains:

- requests and closed outcomes: `ok`, `busy`, `invalid`, `limit`,
  `backpressure`, `not_ready`, or `error`;
- accepted record and canonical-byte totals for `log`, `span`, `metric`, and
  the closed fallback `unknown`;
- `obligations_total`, the accepted fanout across active Tailapps;
- `unrouted_records_total`, accepted records for which no Tailapp was active,
  plus `unrouted_records_by_signal` over the fixed `log`, `span`, `metric`, and
  `unknown` keys; these records are discarded immediately; and
- `detached_obligations_total`, work deliberately abandoned by a gap, runtime
  upgrade, or Tailapp deletion, plus `detached_obligations_by_reason` over the
  fixed `projection_gap`, `runtime_upgrade`, `tailapp_deleted`, and `other`
  keys.

The `inbox` gauge reports current retained records, canonical bytes, delivery
head, oldest/newest positions, oldest receipt timestamp, and pending
obligations. `oldest_inbox_age_milliseconds` makes sustained backpressure easy
to alert on without converting timestamps.

## Tailapp processing

`processing` contains process-lifetime attempts for active Tailapps only. Each
entry reports `ok`, `gap`, `retry`, and `error` outcomes, ineffective records,
emitted private events, the last successful settlement time, queue delay, and
settlement-aware duration. `detached_obligations_total` splits deliberate loss
over the fixed `projection_gap`, `runtime_upgrade`, `tailapp_deleted`, and
`other` reasons. The breakdown follows the same active-only lifetime as its
Tailapp entry; the intake-level total remains visible across app deletion.

`tailapps` contains snapshot gauges and durable projection totals:

- delivery head, interpreted position, and `lag_positions`;
- completeness and optional gap position; and
- durable consumed-record, ineffective-record, and emitted-event totals.

Deleted Tailapp names leave no metrics tombstones. To bound response size and
label cardinality, a snapshot includes at most 256 active Tailapps in sorted
name order and reports the remainder as `omitted_tailapps`. Counts of active,
unavailable, and runtime-upgrade-pending Tailapps remain visible.

## Queries, control, and runtime

`queries` reports outcomes, total rows, encoded row bytes, truncations, engine
lock wait, and total duration. Query outcomes are the closed set `ok`,
`not_found`, `unavailable`, `budget`, `frontier_changed`, `deadline`,
`cancelled`, and `error`.

`control` is keyed by the fixed public operation names. It reports stable
control error classes without retaining arguments, SQL, source, or result
content. Unknown operation names collapse into `unknown`; unknown error
classes collapse into `operation_failed`.

`runtime` is sampled with Go's runtime metrics API and reports goroutines,
heap-object bytes, total runtime-managed memory, and completed GC cycles.
`clock_regressions_total` increments when a wall-clock jump would otherwise
make queue delay or oldest-inbox age negative; the reported duration is clamped
to zero, but the anomaly remains observable.

## Privacy and export boundary

Metrics never contain OTLP bodies, attributes, prompts, tool inputs, SQL,
parameters, source text, event names, revisions as labels, or per-request
records. The `tailapp` engine does not currently expose Prometheus or export
its own OTLP metrics. A future exporter must target a separate collector
endpoint rather than the `tailapp` receiver, avoiding recursive self-ingestion.
