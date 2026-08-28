# Tailapp

Simple micro-apps that read OTLP/HTTP logs, spans, and metric points and
produce SQLite projections, inspectable over CLI or MCP, so you can easily
monitor agent behavior.

One resident engine hosts any number of Tailapps on your machine. Each Tailapp
is a small declarative source set—not another service—and has its own isolated,
continuously materialized SQLite projection. Agents manage and query Tailapps
through MCP; people and scripts can do the same through the CLI.

## Try it

Prerequisites are macOS or Linux, Go 1.26.7 or later, and Git. The demo builds
the binary, starts a temporary resident, installs the three shipped Tailapps
and one custom Tailapp, sends all three OTLP signal types, and queries the
result through CLI and MCP:

```sh
./scripts/demo.sh
```

## Run locally

Build, initialize one engine home, and keep the resident running:

```sh
go build -o tailapp ./cmd/tailapp
export TAILAPP_HOME="$HOME/.local/share/tailapp"
./tailapp init
./tailapp serve --otlp-http 127.0.0.1:4318
```

The receiver is deliberately loopback-only; use an explicit IP rather than
`localhost`. Its actual URL is also written to `$TAILAPP_HOME/engine.json`.

In another terminal, export the same `TAILAPP_HOME` and install the examples:

```sh
export TAILAPP_HOME="$HOME/.local/share/tailapp"
./tailapp apps install --bundle activity-stats \
  --idempotency-key install-activity-stats-v1 activity-stats
./tailapp apps install --bundle agent-guard \
  --idempotency-key install-agent-guard-v1 agent-guard
./tailapp apps install --bundle session-cost \
  --idempotency-key install-session-cost-v1 session-cost
```

Next, configure [Claude Code](docs/harnesses/claude-code.md),
[Codex](docs/harnesses/codex.md), [OpenCode](docs/harnesses/opencode.md), or
[Pi](docs/harnesses/pi.md). Telemetry and MCP are separate connections: the
harness sends OTLP/HTTP to the resident, while its MCP client starts
`tailapp mcp` and connects through the same engine home.

After a model request and tool call, verify transport and interpretation:

```sh
./tailapp health
./tailapp metrics --json
./tailapp query \
  --sql 'SELECT harness, event_family, SUM(event_count) AS events FROM event_inventory GROUP BY harness, event_family ORDER BY harness, event_family' \
  activity-stats
./tailapp query \
  --sql 'SELECT harness, capability, state, reason FROM telemetry_coverage ORDER BY harness, capability' \
  agent-guard
```

An increase in `intake.records_total` proves that OTLP reached the resident.
Rows prove that a Tailapp recognized the records. If intake rises but a query
is empty, `./tailapp ineffective APP` shows up to 16 recent rejected records
for adapter-shape diagnosis; every query result also includes the projection's
durable `ineffective_records` count.

## What you can install

The included Tailapps are a starting kit, not a fixed catalog:

| Tailapp | Materialized analytics |
| --- | --- |
| [`activity-stats`](tailapps/activity-stats/README.md) | Privacy-preserving event, activity, token/cache, tool-frequency, and latency analytics across Claude Code, Codex, and OpenCode |
| [`agent-guard`](tailapps/agent-guard/README.md) | Observed out-of-bounds evidence, explicit unknown coverage, repetition/no-progress signals, and stalled-session queries |
| [`session-cost`](tailapps/session-cost/README.md) | Cumulative token and reported-cost totals by harness and session |

You can create, fork, extend, replace, install, update, query, and delete your
own Tailapps through the same CLI or MCP lifecycle. The
[`signal-counts`](examples/signal-counts/README.md) example is a complete small
source set. Install it in one request:

```sh
./tailapp apps install \
  --idempotency-key install-signal-counts-v1 \
  signal-counts examples/signal-counts
```

An MCP agent can do the same with one `tailapp_install` call. See the
[authoring guide](docs/authoring.md) for the source format and safe update
lifecycle.

## Know the boundaries

- Tailapp is local and single-user. The OTLP receiver has no authentication or
  TLS, and any process with access to the owner-only control socket can use the
  mutation-capable MCP interface.
- The shipped guard is detective, not preventive. It cannot block a tool call,
  stop an agent, or prove that an unobserved operation did not happen. Keep
  harness-native permission and sandbox controls in place.
- Source OTLP records are not retained as an event log. The bounded inbox keeps
  a record only until every captured projection commits or detaches. A failed
  projection has no built-in replay source.
- Projections and ineffective-record samples can contain sensitive telemetry.
  Review exporter content settings and each Tailapp's retention model.
- Tailapps do not form a processing graph. A normalizer emits one private event
  type to its own analytic folds; cross-Tailapp composition happens only in
  bounded read-only queries over explicit exports.

DDL, JSONata, SQL, resource, timing, and representation limits are part of the
pinned runtime profile. See the [DDL/JSONata](docs/reference/ddl-jsonata.md) and
[query SQL](docs/reference/query-sql.md) references for the exact admitted
subsets.

## Documentation

- [Documentation map](docs/README.md)
- [Harness setup and verification](docs/harnesses/README.md)
- [Author and install a Tailapp](docs/authoring.md)
- [CLI](docs/reference/cli.md) and [MCP](docs/reference/mcp.md) references
- [Canonical OTLP records](docs/reference/otel-records.md) and
  [runtime metrics](docs/reference/metrics.md)
- [Architecture](notes/2026-08-28-tailapp-architecture.md) and
  [initial implementation specification](notes/2026-08-28-tailapp-initial-implementation.md)

## Develop

```sh
go test ./...
go test -race ./...
go vet ./...
./scripts/demo.sh
```

GitHub Actions runs the same gates with the Go version declared in `go.mod`.
Tailapp is licensed under [Apache 2.0](LICENSE); attribution is in
[NOTICE](NOTICE).
