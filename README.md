# Tailapps

Tailapps turns local agent telemetry into queryable SQLite datasets. Send
Claude Code, Codex, OpenCode, or Pi telemetry to one local resident, then use
SQL over CLI or MCP to inspect activity, cost, performance, coverage, and
suspicious behavior. Each Tailapp retains useful results rather than another
copy of the OTLP stream.

![Tailapps pipeline: OTLP/HTTP enters a local resident, DDL and JSONata define a two-stage Tailapp, and MCP queries its SQLite projection.](docs/assets/tailapp-architecture.svg)

## What you can install

There are five built-in bundles:

| Tailapp | Questions it answers |
| --- | --- |
| [`daily-review`](tailapps/daily-review/README.md) | What happened today? How many records, failures, slow operations, tokens, reported costs, risky actions, or sensitive fields were observed per harness? |
| [`activity-stats`](tailapps/activity-stats/README.md) | Which tools and endpoints are used? What are the request volumes, latency buckets, outcome rates, token totals, and cache rates? |
| [`agent-guard`](tailapps/agent-guard/README.md) | Did the telemetry show an out-of-bounds action, repeated failures, repeated actions, no progress, or missing evidence needed to decide? |
| [`session-cost`](tailapps/session-cost/README.md) | How many input, cached, output, and reasoning tokens—and how much reported cost—has each session accumulated? |
| [`signal-counts`](tailapps/signal-counts/README.md) | Are logs, spans, and metric points arriving from each source, and which event names are present? |

Agents and people can easily build new tailapps or customize these included
bundles.

## What the results look like

`daily-review` materializes one row per UTC day and harness. Summarize the
latest captured day with:

```sh
./tailapp query --sql '
  SELECT harness, record_count, failure_count,
         duration_gt_5000ms_count AS slow_events,
         cost_microusd / 1000000.0 AS cost_usd,
         risky_action_count
  FROM daily_review
  WHERE day_utc = (SELECT MAX(day_utc) FROM daily_review)
  ORDER BY harness' daily-review
```

| Harness | Records | Failures | Slow events | Reported cost | Risky actions |
| --- | ---: | ---: | ---: | ---: | ---: |
| claude-code | 4,301 | 0 | 8 | $0.84 | 0 |
| codex | 6,077 | 11 | 3 | $1.42 | 1 |
| opencode | 452 | 0 | 0 | $0.09 | 0 |

CLI and MCP return the same typed rows plus projection completeness and
position, ready for people, agents, and automation.

## Install and connect

On macOS or Linux with Go 1.26.7 or later, the fastest durable setup is:

```sh
scripts/setup-resident.sh
```

It builds the binary, links `~/.local/bin/tailapp`, starts a no-sudo user
resident, and installs all five built-in bundles. See
[first-time resident setup](docs/reference/first-time-setup.md) for its dry
run and platform details.

To build and start the resident manually instead:

```sh
go build -o tailapp ./cmd/tailapp
export TAILAPP_HOME="$HOME/.local/share/tailapp"
./tailapp init
./tailapp serve --otlp-http 127.0.0.1:4318
```

The unauthenticated OTLP receiver is loopback-only; keep it that way.

In another terminal, install the included Tailapps:

```sh
export TAILAPP_HOME="$HOME/.local/share/tailapp"
for app in activity-stats agent-guard daily-review session-cost signal-counts; do
  ./tailapp apps install --bundle "$app" \
    --idempotency-key "install-$app-v1" "$app"
done
```

Connect [Claude Code](docs/harnesses/claude-code.md),
[Codex](docs/harnesses/codex.md), [OpenCode](docs/harnesses/opencode.md), or
[Pi](docs/harnesses/pi.md). Send OTLP/HTTP to the resident and start
`tailapp mcp` with the same `TAILAPP_HOME`.

After a model request and tool call, confirm intake and projection health:

```sh
./tailapp health
./tailapp metrics --json
```

If intake rises but a result stays empty, `./tailapp ineffective APP` shows
recent records that app did not recognize.

## Create your own

[`signal-counts`](tailapps/signal-counts/README.md) is the smallest complete
template. Install any source directory through the same lifecycle:

```sh
./tailapp apps install --idempotency-key install-my-tailapp-v1 \
  my-tailapp path/to/my-tailapp
```

MCP agents use `tailapp_install`. See the [authoring guide](docs/authoring.md)
for the source format and update lifecycle.

## Boundaries

- Tailapp is local and single-user. The receiver has no authentication or TLS;
  access to the control socket permits MCP mutations.
- `agent-guard` is detective, not preventive. Keep harness permission and
  sandbox controls in place.
- Source OTLP records are discarded after captured projections commit or
  detach. There is no retained event log for replay.
- Projections and diagnostic samples can contain sensitive telemetry. Review
  exporter settings and each Tailapp's retention model.
- Cross-Tailapp composition happens only through bounded, read-only SQL over
  explicit exports.

DDL, JSONata, SQL, resource, timing, and representation limits are part of the
pinned runtime profile. See the [DDL/JSONata](docs/reference/ddl-jsonata.md) and
[query SQL](docs/reference/query-sql.md) references for the admitted subsets.

## Documentation

- [Documentation map](docs/README.md)
- [Harness setup and verification](docs/harnesses/README.md)
- [Author and install a Tailapp](docs/authoring.md)
- [CLI](docs/reference/cli.md) and [MCP](docs/reference/mcp.md) references
- [Canonical OTLP records](docs/reference/otel-records.md) and
  [runtime metrics](docs/reference/metrics.md)
- [Architecture](notes/2026-08-28-tailapp-architecture.md)

## Develop

```sh
go test ./...
go test -race ./...
go vet ./...
```

GitHub Actions runs the same gates with the Go version declared in `go.mod`.
Tailapps is licensed under [Apache 2.0](LICENSE); attribution is in
[NOTICE](NOTICE).
