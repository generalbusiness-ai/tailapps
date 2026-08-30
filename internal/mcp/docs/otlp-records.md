# Canonical OTLP records

The resident accepts OTLP/HTTP logs, traces, and metrics on its loopback
receiver and turns every log record, span, and metric point into one
canonical `otlp_record` — the event every normalizer receives.

## Envelope

Scalar fields readable directly and usable as read parameters
(`:event.field`):

| Field | Meaning |
| --- | --- |
| `id` | The record's canonical identity |
| `signal` | `log`, `span`, or `metric` |
| `name` | The event name (for logs: `eventName` or the `event.name` attribute) |
| `source` | The producing service (`service.name` and fallbacks) |
| `time_unix_nano` | Event time as decimal text; null when the producer sent zero |
| `observed_unix_nano` | Observation time as decimal text; null when zero |
| `trace_id`, `span_id` | Hex identifiers; null when absent |
| `content_digest` | Digest of the canonical payload |

## Payload

`record` carries the full canonical payload: `attributes`, `body`,
`resource`, `scope`, and signal-specific fields. Attribute values follow
the shared value model: exact-range integers as numbers, larger integers
as `{"integer_decimal": "…"}`, bytes as `{"bytes_base64": "…"}`, finite
numbers only.

Source records are not retained as an event log: the bounded inbox keeps a
record only until every capturing projection commits or detaches.
