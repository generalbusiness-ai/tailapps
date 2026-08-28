# MCP reference

`tailapp mcp` exposes the resident engine as an MCP stdio server. It is the
primary agent interface for both read-only analytics and Tailapp definition
management.

## Transport and protocol

- Transport: newline-delimited JSON-RPC 2.0 over stdin/stdout
- Advertised MCP protocol version: `2025-06-18`
- Server identity: `tailapp` version `0.1.0`
- Capability: tools
- Maximum input line: 1 MiB

The process is only an adapter. Start `tailapp serve` first and give the MCP
process the same `TAILAPP_HOME` so it can reach `engine.sock`.

Successful tool calls return the value twice: as JSON text in `content` and as
the native JSON value in `structuredContent`:

```json
{
  "content": [{"type": "text", "text": "{...}"}],
  "structuredContent": {...}
}
```

Engine/application errors are MCP tool results, not JSON-RPC failures:

```json
{
  "isError": true,
  "content": [{"type": "text", "text": "error message"}]
}
```

Malformed JSON-RPC, unknown methods, invalid call envelopes, and unknown tools
use normal JSON-RPC errors.

## Common values

### App

```json
{
  "name": "event-counts",
  "draft_revision": "sha256:...",
  "active_revision": "sha256:...",
  "runtime_profile": "tailapp-...",
  "activation_mode": "reset",
  "boundary_position": 42
}
```

The last four fields are absent before first activation.

### Frontier

```json
{
  "revision": "sha256:...",
  "activation_boundary": 42,
  "interpreted_position": 57,
  "last_event_id": "local:57",
  "complete": true,
  "gap_position": 58,
  "gap_reason": "..."
}
```

Gap fields are absent for a healthy projection.

### Idempotency key

All mutating tools require `idempotency_key`: 1 to 128 printable ASCII
characters, with no spaces at either boundary. A key is bound durably to the
exact tool operation and arguments. An identical retry replays the result;
different arguments conflict.

## Definition tools

### `tailapps_list`

Lists all local Tailapp definitions.

Arguments: `{}`

Result: array of [App](#app) objects ordered by name.

### `tailapp_get`

Reads one draft and every source element. Draft content is deployment state and
may differ from the active revision.

Arguments:

```json
{"name": "event-counts"}
```

Result:

```json
{
  "app": {"name": "event-counts", "draft_revision": "sha256:..."},
  "sources": {
    "application.sql": "Q1JFQVRFIC4uLg==",
    "folds/normalize.jsonata": "KC4uLik="
  }
}
```

Source byte values are standard base64.

### `tailapp_create`

Creates an empty custom draft or copies a source set bundled into the binary.

Arguments:

```json
{
  "name": "event-counts",
  "idempotency_key": "event-counts-create-v1"
}
```

Optional `bundle` is a string. Built-in values are `activity-stats`,
`agent-guard`, and `session-cost`; omit it for custom source. Result: [App](#app).

### `tailapp_install`

Validates a complete source set and first-activates one new Tailapp in a single
idempotent request. It never replaces an existing Tailapp.

Custom-source arguments:

```json
{
  "name": "event-counts",
  "sources": {
    "application.sql": "Q1JFQVRFIC4uLg==",
    "folds/normalize.jsonata": "KC4uLik=",
    "folds/count.jsonata": "KC4uLik="
  },
  "idempotency_key": "event-counts-install-v1"
}
```

Every source value is standard base64. Supply the complete source map; the
normal source limits still apply. To install a shipped example, omit `sources`
and pass `"bundle": "activity-stats"`, `"bundle": "agent-guard"`, or
`"bundle": "session-cost"`. Supplying
both is invalid.

Result:

```json
{
  "app": {"name": "event-counts", "active_revision": "sha256:..."},
  "profile": {"name": "event-counts", "revision": "sha256:..."},
  "frontier": {"revision": "sha256:...", "complete": true}
}
```

The real `profile` and `frontier` contain the complete structures documented
under `tailapp_validate` and [Frontier](#frontier). Use element mutation and
explicit activation for later updates.

### `tailapp_delete`

Deletes one definition and detaches only its projection.

Arguments:

```json
{
  "name": "event-counts",
  "idempotency_key": "event-counts-delete-v1"
}
```

Result: `{"deleted": true}`.

### `tailapp_put_element`

Puts one source element into a draft. This does not activate it.

Arguments:

```json
{
  "name": "event-counts",
  "path": "folds/count.jsonata",
  "content": "KC4uLik=",
  "expected_revision": "sha256:...",
  "idempotency_key": "event-counts-put-count-v1"
}
```

`content` is standard base64. Allowed paths are `application.sql` and
`folds/*.jsonata`. Result: [App](#app) with the new `draft_revision`; use it for
the next edit.

### `tailapp_delete_element`

Removes one draft element without changing live behavior.

Arguments:

```json
{
  "name": "event-counts",
  "path": "folds/old.jsonata",
  "expected_revision": "sha256:...",
  "idempotency_key": "event-counts-remove-old-v1"
}
```

Result: [App](#app) with the next revision.

### `tailapp_validate`

Compiles an exact complete draft without activation.

Arguments:

```json
{"name": "event-counts", "expected_revision": "sha256:..."}
```

Result: compiled profile containing:

- `name`, `revision`, and `runtime_profile`;
- `storage_schema_digest` and `export_contract_digest`;
- private `event` columns;
- `normalizer` and `folds`, including reads/writes/source paths;
- `tables`, `views`, `exports`, and writer ownership;
- executable `schema_sql` and replaceable schema SQL.

Use this result as review evidence before activation.

### `tailapp_activate`

Activates an exact compiled draft at a delivery boundary.

Arguments:

```json
{
  "name": "event-counts",
  "expected_revision": "sha256:...",
  "mode": "reset",
  "acknowledge_reset": true,
  "idempotency_key": "event-counts-activate-v1"
}
```

`mode` is `continue` or `reset`. `acknowledge_reset` is logically required for
reset and may be omitted/false for continue. First activation requires reset.
Result: [Frontier](#frontier).

## Inspection tools

### `tailapp_status`

Arguments: `{}`

Result:

```json
{
  "profile": "tailapp-...",
  "ingestion_ready": true,
  "inbox": {
    "records": 0,
    "canonical_bytes": 0,
    "delivery_head": 57,
    "oldest_position": null,
    "newest_position": null,
    "oldest_received_at_unix_nano": null,
    "pending_obligations": 0
  },
  "apps": {"event-counts": {"revision": "sha256:..."}},
  "unavailable": {"broken-app": "reason"}
}
```

Each `apps` value is a full [Frontier](#frontier). `unavailable` is omitted
when empty. `ingestion_ready = false` means the receiver is fail-closed pending
operator action.

### `tailapp_metrics`

Arguments: `{}`

Returns the same versioned, payload-free operational snapshot as
`tailapp metrics --json`. It includes intake and backpressure, active-Tailapp
processing and durable progress, queries, control requests, and Go runtime
gauges. Process counters reset on resident restart; durable projection totals
do not. See the [runtime metrics reference](metrics.md) for the complete schema,
timing boundaries, reset semantics, and cardinality limits.

### `tailapp_ineffective`

Arguments:

```json
{"name": "agent-guard"}
```

Returns the same bounded diagnostic snapshot as `tailapp ineffective APP`:

```json
{
  "tailapp": "agent-guard",
  "revision": "sha256:...",
  "capacity": 16,
  "ineffective_records": 24,
  "available_records": 16,
  "unavailable_records": 8,
  "records": [{
    "position": 57,
    "event_id": "local:57",
    "revision": "sha256:...",
    "signal": "log",
    "name": "codex.unmapped_event",
    "source": "codex",
    "content_digest": "sha256:...",
    "record_bytes": 412,
    "record": {"attributes": {}, "resource": {"attributes": {}}}
  }]
}
```

The per-Tailapp buffer keeps the 16 newest ineffective canonical records in
resident memory only and clears on restart or activation. Payloads above 32
KiB are omitted while their metadata and original byte size remain. Records
may contain sensitive telemetry; this tool deliberately exposes payloads for
local diagnosis and is not an event-history API. `ineffective_records` is the
durable total, while `available_records` and `unavailable_records` explicitly
state how much of that total is and is not represented by the current buffer.

### `tailapp_schema`

Reads the active compiled profile, not the draft.

Arguments:

```json
{"name": "event-counts"}
```

Result: the same compiled-profile shape described under
[`tailapp_validate`](#tailapp_validate).

## Query tool

### `tailapp_query`

Runs one bounded read-only SQL statement against an active Tailapp and optional
mounted exports.

Minimal arguments:

```json
{
  "name": "event-counts",
  "sql": "SELECT source, event_name, event_count FROM event_counts ORDER BY source, event_name"
}
```

Full arguments:

```json
{
  "name": "agent-guard",
  "sql": "SELECT p.session_id, c.input_tokens FROM session_progress p JOIN cost.session_cost c ON c.harness = p.harness AND c.session_id = p.session_id WHERE p.harness = ? ORDER BY p.session_id",
  "parameters": ["codex"],
  "mounts": {"cost": "session-cost"},
  "expected_revision": "sha256:...",
  "expected_position": 57,
  "row_limit": 256
}
```

Fields other than `name` and `sql` are optional:

- `parameters`: positional JSON scalars; maximum 64
- `mounts`: SQL identifier alias to Tailapp name
- `expected_revision`: snapshot precondition
- `expected_position`: interpreted-frontier precondition
- `row_limit`: 1 to 1000; default 256

Result:

```json
{
  "tailapp": "agent-guard",
  "revision": "sha256:...",
  "delivery_head": 57,
  "interpreted_position": 57,
  "ineffective_records": 12,
  "schemas": [{
    "alias": "cost",
    "tailapp": "session-cost",
    "revision": "sha256:...",
    "contract": "sha256:...",
    "interpreted_position": 57
  }],
  "complete": true,
  "columns": [{"name": "session_id", "type": "TEXT"}],
  "rows": [["session-1", 123]],
  "result_bytes": 19,
  "truncated": false
}
```

`ineffective_records` is the primary Tailapp projection's durable count since
initial activation or its most recent reset, including across compatible
continue activations. It records normalizer decisions and is not a count of
rows omitted from this SQL result.

Mounted projections must be complete and aligned to the primary interpreted
position. Only their explicit exports are visible. See the [query SQL
reference](query-sql.md).

## Stable failure classes

The private control service classifies failures including `not_found`,
`revision_changed`, `idempotency_conflict`, `idempotency_in_doubt`,
`projection_unavailable`, `deadline_exceeded`, `frontier_changed`, and
`query_budget_exceeded`. MCP returns these as `isError` text using the stable
`<code>: <message>` prefix convention; it does not yet provide a separate
structured error-code field. Agents may parse the prefix, but should still
treat the complete result as a failure, refresh status/revisions where
appropriate, and never blindly generate a new idempotency key to evade an
in-doubt mutation.
