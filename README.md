# Tailapp

Tailapps are simple, local micro-apps that turn OTLP/HTTP logs, spans, and
metric points into SQLite projections you can inspect over CLI or MCP, making
agent behavior easy to monitor.

One resident engine hosts any number of installed Tailapps on your machine.
Each Tailapp is a small declarative source set—not another service process—and
has its own isolated, continuously materialized SQLite projection. Agents can
author, install, query, and manage Tailapps through MCP; people and scripts can
do the same through the CLI.

The shipped `agent-guard` example provides:

- observed denied-tool, denied-operation, and target-boundary evidence;
- explicit `unknown` findings when required telemetry is absent or redacted;
- repeated-action, repeated-failure, and bounded no-progress signals; and
- session progress timestamps for a caller-supplied stalled cutoff.

These are detective controls. OTLP observation occurs after or alongside
harness activity. Tailapp does not block a tool call, terminate an agent, or
prove that an unobserved operation did not happen. Inline prevention requires a
harness control adapter, which is outside this release.

The complementary `activity-stats` example materializes privacy-preserving
event inventory, session activity, cache/token totals, bounded tool frequency,
and request/TTFT distributions across Claude Code, Codex, and OpenCode adapter
telemetry. It retains no prompt, response, command, tool-input, or path content
and reports missing fields as unknown coverage rather than observed zeros.

## Quick demo

```sh
./scripts/demo.sh
```

The demo builds the binary, starts an ephemeral resident, installs three shipped
examples and one custom Tailapp in one request each, sends logs, a span, and a
metric point, and prints a real projection before checking CLI and MCP access.

## Start locally

Prerequisites are macOS or Linux, Go 1.26.7 or later, and Git. Build and start
the resident in one terminal:

```sh
go build -o tailapp ./cmd/tailapp
export TAILAPP_HOME="$HOME/.local/share/tailapp"
./tailapp init
./tailapp serve --otlp-http 127.0.0.1:4318
```

Keep `serve` running. The bind uses an explicit loopback IP because names such
as `localhost` and non-loopback interfaces are deliberately refused. The
actual receiver URL is also written to `$TAILAPP_HOME/engine.json`.

In another terminal, export the same `TAILAPP_HOME`, then install and
first-activate the three examples shipped with this release:

```sh
export TAILAPP_HOME="$HOME/.local/share/tailapp"
./tailapp apps install --bundle activity-stats \
  --idempotency-key install-activity-stats-v1 activity-stats
./tailapp apps install --bundle agent-guard \
  --idempotency-key install-agent-guard-v1 agent-guard
./tailapp apps install --bundle session-cost \
  --idempotency-key install-session-cost-v1 session-cost
```

Connect a harness using the [harness setup guide](docs/harnesses/README.md),
perform a tool call, then inspect what arrived:

```sh
./tailapp health
./tailapp metrics --json
./tailapp ineffective agent-guard
./tailapp query \
  --sql 'SELECT harness, capability, state, reason FROM telemetry_coverage ORDER BY harness, capability' \
  agent-guard
```

The metrics intake counters prove OTLP transport. `tailapp ineffective` shows a
memory-only sample of rejected canonical records, while every query result's
`ineffective_records` field gives the durable rejection count for that
projection. An empty SQL result means the installed Tailapp has not received a
recognized event yet. The harness guides explain exporter batching, expected
event names, and coverage limits.

## Build and install your own

The [`signal-counts`](examples/signal-counts/README.md) example is a complete
custom Tailapp. Install its entire directory in one validated, first-activated
request:

```sh
./tailapp apps install \
  --idempotency-key install-signal-counts-v1 \
  signal-counts examples/signal-counts
```

An MCP agent does the same with one `tailapp_install` call containing the
complete source map. Installation is create-only and never replaces an
existing Tailapp. Incremental updates deliberately retain the lower-level
draft, exact-revision validation, and explicit activation lifecycle described
in the [authoring guide](docs/authoring.md).

## Documentation

- [Documentation map](docs/README.md)
- [Harness setup](docs/harnesses/README.md):
  [Claude Code](docs/harnesses/claude-code.md),
  [Codex](docs/harnesses/codex.md),
  [OpenCode](docs/harnesses/opencode.md), and [Pi](docs/harnesses/pi.md)
- [Author and install a Tailapp](docs/authoring.md)
- [Five-minute signal-counts example](examples/signal-counts/README.md)
- References: [CLI](docs/reference/cli.md), [MCP](docs/reference/mcp.md),
  [DDL/JSONata](docs/reference/ddl-jsonata.md), and
  [query SQL](docs/reference/query-sql.md), plus
  [runtime metrics](docs/reference/metrics.md)
- Examples shipped with this release:
  [`activity-stats`](tailapps/activity-stats/README.md),
  [`agent-guard`](tailapps/agent-guard/README.md), and
  [`session-cost`](tailapps/session-cost/README.md)

The built-in source sets are examples, not a closed catalog. Operators and
agents are encouraged to create new Tailapps, fork or extend these examples,
or substitute different analytics and policy. Custom source sets and the three
shipped examples use the same compiler, install boundary, and runtime.

## Boundaries

The first trust boundary is the local OS user. OTLP is loopback-only and
control uses an owner-only Unix socket, but other processes running as that
user are not authenticated. Telemetry and projection files can contain
sensitive prompts, commands, paths, and identifiers.

Tailapp source cannot use filesystem, network, clock, randomness, extensions,
dynamic evaluation, wildcards, multiplication, or generated ranges. Since
unquoted `*` is excluded, JSONata block comments are unavailable too.
The JSONata wall deadline is an outer safety net, not a claim of deterministic
semantics; the admitted JSONata subset excludes its known unbounded extension
points. SQL additionally has a runtime-profile-fixed SQLite VM progress budget,
with its wall deadline retained as secondary safety. The runtime profile pins
OTLP canonicalization, JSONata, SQLite, numeric rules, and admitted limits.

Consumed OTLP content is not retained as an event log. The bounded inbox keeps
a record only until every captured consumer commits or detaches. For local
diagnosis, the resident keeps only the 16 newest ineffective records per
Tailapp in memory; `tailapp ineffective APP` exposes them, and restart or
activation clears them. A projection gap is fail-stop and local to that
Tailapp; without external replay, telemetry missed while detached is absent.

Mounted exports use explicit `FROM alias.export` or `JOIN alias.export`
relations; comma-listed mounted relations are outside the admitted query
syntax. Give each mounted relation a SQL table alias and qualify its columns
through that alias.

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
./scripts/demo.sh
```

GitHub Actions runs the same four gates on pushes and pull requests to `main`
with the exact Go version declared in `go.mod`. The demo's root `tailapp`
binary is ignored so local verification leaves the checkout clean.
