# Canonical OTLP records

The resident turns every accepted OTLP log record, span, and individual metric
point into one canonical `otlp_record`. A Tailapp normalizer receives that
record as `event`. This page describes the stable input shape authors can use
without reading Go or protobuf definitions.

For a working application that accepts every signal, see
[`signal-counts`](../../tailapps/signal-counts/README.md).

## Common envelope

Every normalizer receives these fields:

| Field | Type | Meaning |
|---|---|---|
| `event.id` | string | Local durable delivery identifier. |
| `event.signal` | `log`, `span`, or `metric` | Canonical signal kind. |
| `event.name` | string | Log event name, span name, or metric name. |
| `event.source` | string | First nonempty `service.name`, `gen_ai.agent.name`, or `telemetry.sdk.name`; otherwise `unknown`. |
| `event.time_unix_nano` | string or null | Decimal nanosecond timestamp. |
| `event.observed_unix_nano` | string or null | Log observation timestamp when supplied. |
| `event.trace_id` | lowercase hex or null | Trace identifier for logs and spans. |
| `event.span_id` | lowercase hex or null | Span identifier for logs and spans. |
| `event.content_digest` | string | SHA-256 digest of `event.record`. |
| `event.record` | object | Canonical signal-specific content below. |

Nanosecond timestamps are decimal strings so the full unsigned 64-bit OTLP
range survives JSON and JSONata without precision loss.

Every `event.record` contains:

```json
{
  "attributes": {"session.id": "session-1"},
  "resource": {
    "attributes": {"service.name": "demo-agent"},
    "dropped_attributes_count": 0,
    "schema_url": ""
  },
  "scope": {
    "name": "agent-harness",
    "version": "1.0",
    "attributes": {},
    "dropped_attributes_count": 0,
    "schema_url": ""
  },
  "otel": {}
}
```

`record.otel` is the decoded, signal-specific OTLP protobuf object using its
original snake-case field names. Prefer the canonical fields when available;
use `otel` when an application needs details Tailapp deliberately does not
promote into the common shape.

## Log record

A log adds:

| Field | Type |
|---|---|
| `record.body` | any canonical OTLP value |
| `record.severity_number` | OTLP enum-name string |
| `record.severity_text` | string |
| `record.flags` | integer |
| `record.dropped_attributes_count` | integer |

`event.name` comes from the log record's `event_name`; if absent, `tailapp` uses
the string attribute `event.name`.

```json
{
  "signal": "log",
  "name": "codex.tool_result",
  "record": {
    "attributes": {"conversation.id": "s1", "success": true},
    "body": null,
    "severity_number": "SEVERITY_NUMBER_UNSPECIFIED",
    "severity_text": "",
    "flags": 0,
    "dropped_attributes_count": 0
  }
}
```

## Span

A span adds:

| Field | Type |
|---|---|
| `record.kind` | OTLP enum-name string |
| `record.parent_span_id` | lowercase hex or null |
| `record.start_time_unix_nano` | decimal string or null |
| `record.end_time_unix_nano` | decimal string or null |
| `record.trace_state` | string |
| `record.flags` | integer |

The span name becomes `event.name`; its start time becomes
`event.time_unix_nano`.

```json
{
  "signal": "span",
  "name": "agent.run",
  "record": {
    "attributes": {},
    "kind": "SPAN_KIND_UNSPECIFIED",
    "parent_span_id": null,
    "start_time_unix_nano": "1787900000000002000",
    "end_time_unix_nano": "1787900000000002500",
    "trace_state": "",
    "flags": 0
  }
}
```

## Metric point

Each OTLP data point becomes a separate `event.signal = "metric"` record. A
metric adds:

| Field | Type |
|---|---|
| `record.metric.name` | string |
| `record.metric.description` | string |
| `record.metric.unit` | string |
| `record.metric.point_type` | `gauge`, `sum`, `histogram`, `exponential_histogram`, or `summary` |
| `record.metric.aggregation_temporality` | enum-name string, for sum and histogram families |
| `record.metric.is_monotonic` | boolean, for sums |
| `record.start_time_unix_nano` | decimal string or null |
| `record.time_unix_nano` | decimal string or null |

The metric name becomes `event.name`; the point time becomes
`event.time_unix_nano`. Point values, exemplars, buckets, quantiles, and
histogram details remain in `record.otel` in their original OTLP shape.

```json
{
  "signal": "metric",
  "name": "agent.tokens",
  "record": {
    "attributes": {},
    "metric": {
      "name": "agent.tokens",
      "description": "",
      "unit": "token",
      "point_type": "sum",
      "aggregation_temporality": "AGGREGATION_TEMPORALITY_DELTA",
      "is_monotonic": true
    },
    "start_time_unix_nano": null,
    "time_unix_nano": "1787900000000003000",
    "otel": {"time_unix_nano": "1787900000000003000", "as_int": "9"}
  }
}
```

## Attribute values

OTLP strings, booleans, finite doubles, arrays, and key/value lists map to the
corresponding JSON values. Integers within JavaScript's exact range map to JSON
integers. Larger integers become `{"integer_decimal": "..."}`. Bytes become
`{"bytes_base64": "..."}`. Duplicate or empty attribute keys and non-finite
doubles are rejected at ingestion rather than silently rewritten. The receiver
also explicitly rejects `key_strindex` and `string_value_strindex`: they are
profiling-signal string-table references, which cannot identify a log, trace,
or metric attribute without that signal’s dictionary.

## Limits

One canonical record is at most 256 KiB. The receiver accepts OTLP/HTTP JSON or
protobuf at `/v1/logs`, `/v1/traces`, and `/v1/metrics`; it does not accept
OTLP/gRPC. Source records are retained only in the bounded inbox until every
captured Tailapp consumes or detaches from them.
