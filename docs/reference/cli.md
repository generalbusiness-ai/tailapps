# CLI reference

`tailapp` prints JSON to standard output. Errors are written as command errors
and return a nonzero exit status. Except for `init`, `serve`, and
`ingest-fixture` transport, commands are clients of the resident engine's
owner-only Unix socket.

`tailapp --help` summarizes the shortest init, serve, install, and query path;
`tailapp apps --help` lists the one-shot install and lower-level lifecycle.

## Environment

`TAILAPP_HOME` selects the engine home. If unset, Tailapp uses
`tailapp` beneath the operating system's user configuration directory. Set it
explicitly when the resident and harness-launched MCP processes must share one
engine.

```sh
export TAILAPP_HOME="$HOME/.local/share/tailapp"
```

## Process commands

### `tailapp init`

Creates the protected engine home and projection/temp directories. It is safe
to call again for the same home.

### `tailapp serve [--otlp-http IP:PORT]`

Runs the single resident engine, control socket, projection worker, and
OTLP/HTTP receiver. The default receiver is `127.0.0.1:4318`. The host must be
an explicit IPv4 or IPv6 loopback address; names such as `localhost` and
non-loopback interfaces are refused. Port `0` chooses an ephemeral port.

On startup it prints the home, control socket, and actual receiver URL and
writes owner-only `engine.json`. Stop it with SIGINT or SIGTERM.

### `tailapp health`

Returns the runtime profile, ingestion readiness, inbox statistics, projection
frontiers, and unavailable projections. This is the same engine status model
returned by `apps status` and `tailapp_status`.

### `tailapp metrics [--json]`

Returns the versioned operational metrics snapshot. JSON is the default and
only format; `--json` makes that choice explicit for scripts. Process counters
reset when the resident restarts, while each Tailapp's durable projection
totals survive a restart. See the [runtime metrics reference](metrics.md) for
field definitions, fixed histogram boundaries, privacy constraints, and reset
semantics.

### `tailapp ineffective APP`

Returns the 16 most recent records that the named Tailapp's normalizer declared
ineffective. Samples are held only in resident memory, disappear on restart or
activation, and are ordered oldest to newest. Each sample includes its delivery
position, active revision, signal, event name, source, timestamps and IDs when
present, content digest, and the actual canonical `record` JSON.

`ineffective_records` is the durable total since initial activation or reset,
`available_records` is the number represented by the current memory buffer, and
`unavailable_records` makes restart, activation, and ring-eviction gaps explicit.
The last value is zero only when every durable ineffective decision remains in
the buffer; it does not claim that omitted or redacted fields were observed.

A canonical record larger than 32 KiB retains only metadata, `record_bytes`,
and `record_omitted: true`. The buffer can contain sensitive prompt, tool, path,
or attribute content and is available to any process that can access the
owner-only control socket. Use the durable `ineffective_records` query-result
field or metrics for counting; this command is a bounded diagnostic sample,
not an event log.

### `tailapp mcp`

Runs the MCP JSON-RPC adapter over newline-delimited stdio. It does not start
an engine; `tailapp serve` must already be running with the same
`TAILAPP_HOME`. See the [MCP reference](mcp.md).

### `tailapp ingest-fixture [options] FILE`

Posts an OTLP fixture to the URL in `engine.json`. This is intended for tests
and local demonstrations.

Options:

- `--signal logs|traces|metrics` (default `logs`)
- `--content-type application/json|application/x-protobuf` (default JSON)

## Application commands

Tailapp names match `^[a-z][a-z0-9-]{0,62}$`.

### `tailapp apps list`

Lists definitions with draft and optional active revision metadata.

### `tailapp apps create [--bundle NAME] --idempotency-key KEY APP`

Creates a draft. Omit `--bundle` for an empty custom Tailapp. The only built-in
bundle names in this release are `activity-stats`, `agent-guard`, and
`session-cost`.

### `tailapp apps install [options] --idempotency-key KEY APP [DIRECTORY]`

Validates a complete source set and first-activates one new Tailapp in a single
idempotent control request. It is create-only and refuses an existing app name.

Install custom source from a directory:

```sh
tailapp apps install \
  --idempotency-key install-signal-counts-v1 \
  signal-counts examples/signal-counts
```

The directory may contain author documentation. Only `application.sql` and
regular, non-symlink `folds/*.jsonata` files become executable source.

Install a shipped example without a directory:

```sh
tailapp apps install --bundle agent-guard \
  --idempotency-key install-agent-guard-v1 agent-guard
```

The concise result contains `app`, compiled-profile identity and contract
digests, and the active `frontier`. Pass `--full` to include the complete
compiled profile. Use the lower-level commands below for updates to an existing
app.

### `tailapp apps get APP`

Returns draft metadata and all source elements. Binary JSON fields representing
source bytes use standard base64 in the JSON response.

### `tailapp apps put --expected REV --idempotency-key KEY APP PATH FILE`

Reads `FILE` and puts it at source `PATH` using optimistic revision control.
The result contains the next draft revision; use that revision for the next
mutation. Allowed paths are `application.sql` and `folds/*.jsonata`.

### `tailapp apps rm --expected REV --idempotency-key KEY APP PATH`

Removes one draft element and returns the next draft revision. It does not
alter live behavior.

### `tailapp apps validate --expected REV APP`

Compiles the complete exact draft without activation. The result is the
compiled profile: revision, runtime, event, tables, writers, reads, programs,
views, exports, and storage/export digests.

### `tailapp apps activate [options] --idempotency-key KEY APP`

Activates the exact draft at a drained delivery boundary.

Options:

- `--expected REV`: exact draft revision
- `--mode continue|reset` (default `continue`)
- `--ack-reset`: required with `reset`

The first activation requires `reset`. A `continue` preserves current rows and
requires all existing writable tables to retain their exact stored shape; new
tables are allowed. A `reset` discards only this Tailapp's materialized state.

### `tailapp apps delete --idempotency-key KEY APP`

Deletes a definition and detaches its projection obligations. It does not
delete other Tailapps.

### `tailapp apps status`

Returns engine status; equivalent to `tailapp health`.

### `tailapp apps schema APP`

Returns the active compiled profile. Draft-only Tailapps have no live schema.

## Query command

```text
tailapp query [options] --sql SQL APP
```

Options:

- `--param JSON`: positional parameter; repeat up to 64 times
- `--mount ALIAS=TAILAPP`: expose another active Tailapp's exports; repeatable
- `--expected-revision REV`: fail if the active revision differs
- `--expected-position N`: fail if the interpreted frontier differs
- `--limit N`: row limit, 1 to 1000; default 256

Parameters are parsed as JSON, so quote strings as JSON strings:

```sh
tailapp query \
  --sql 'SELECT session_id FROM session_progress WHERE harness = ? ORDER BY session_id' \
  --param '"codex"' \
  agent-guard
```

The result reports the active revision, delivery head, interpreted position,
durable `ineffective_records` count for the active projection, mounted schemas,
completeness, column metadata, rows, encoded `result_bytes`, and whether output
was truncated. `ineffective_records` counts records rejected since the
projection's initial activation or most recent reset, including across
compatible continue activations; it is independent of the
number of rows returned by the SQL statement. See the [query SQL
reference](query-sql.md).

## Idempotency and revisions

Install, create, put, remove, activate, and delete require a printable ASCII
key of 1 to 128 characters with no leading or trailing space. Retrying the
identical operation, arguments, and key replays the original success or error.
Reusing a key for any different request returns `idempotency_conflict`. A key
left pending across a crash returns `idempotency_in_doubt`; Tailapp will not
guess whether to repeat the mutation.

Element mutations and validation use the exact draft revision. A stale value
returns `revision_changed`. Query expectations return `frontier_changed` when
the live snapshot moved.
