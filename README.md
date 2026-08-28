# Tailapp

Tailapp is a local, continuously materialized analytics engine for coding-agent
telemetry. It accepts standard OTLP/HTTP logs, spans, and metric points, then
runs deterministic Tailapp definitions into isolated SQLite projections that
agents can inspect through bounded read-only SQL over CLI or MCP.

The shipped `agent-guard` example provides:

- observed denied-tool, denied-operation, and target-boundary evidence;
- explicit `unknown` findings when required telemetry is absent or redacted;
- repeated-action, repeated-failure, and bounded no-progress signals; and
- session progress timestamps for a caller-supplied stalled cutoff.

These are detective controls. OTLP observation occurs after or alongside
harness activity. Tailapp does not block a tool call, terminate an agent, or
prove that an unobserved operation did not happen. Inline prevention requires a
harness control adapter, which is outside this release.

## Quick demo

```sh
./scripts/demo.sh
```

The script builds the binary, starts an ephemeral resident, installs and
activates `agent-guard` and `session-cost` through the public service, sends a
cross-harness OTLP fixture, and verifies policy, unknown coverage, loops,
stalled sessions, export joins, CLI, and MCP results.

For a persistent engine:

```sh
go build -o tailapp ./cmd/tailapp
export TAILAPP_HOME="$HOME/.local/share/tailapp"
./tailapp init
./tailapp serve --otlp-http 127.0.0.1:4318
```

The bind must use an explicit loopback IP; `localhost:4318` is deliberately
refused. Point harness OTLP/HTTP exporters at the address in
`$TAILAPP_HOME/engine.json`, then install the bundles:

```sh
./tailapp apps create --bundle agent-guard --idempotency-key install-agent-guard-v1 agent-guard
./tailapp apps create --bundle session-cost --idempotency-key install-session-cost-v1 session-cost
```

Creation returns a draft revision. Validate and reset-activate each exact draft
with `apps validate --expected REV APP` and `apps activate --expected REV
--mode reset --ack-reset --idempotency-key KEY APP`. Create, delete, element
mutation, and activation keys are durably bound to the exact request; retrying
a completed key replays its original success or error. Reusing it for another
request is refused. A key left pending by a crash is reported as in doubt
instead of risking a duplicate destructive effect. Draft edits never change
live behavior until activation.

## Documentation

- [Documentation map](docs/README.md)
- [Harness setup](docs/harnesses/README.md):
  [Claude Code](docs/harnesses/claude-code.md),
  [Codex](docs/harnesses/codex.md),
  [OpenCode](docs/harnesses/opencode.md), and [Pi](docs/harnesses/pi.md)
- [Author and install a Tailapp](docs/authoring.md)
- References: [CLI](docs/reference/cli.md), [MCP](docs/reference/mcp.md),
  [DDL/JSONata](docs/reference/ddl-jsonata.md), and
  [query SQL](docs/reference/query-sql.md)
- Examples shipped with this release:
  [`agent-guard`](tailapps/agent-guard/README.md) and
  [`session-cost`](tailapps/session-cost/README.md)

The built-in source sets are examples, not a closed catalog. Operators and
agents are encouraged to create new Tailapps, fork or extend these examples,
or substitute different analytics and policy. They can create a draft, upload
its `application.sql` and `folds/*.jsonata` elements, validate the exact
revision, and activate it using either CLI or MCP. Tailapp v1 has no
directory-import command and `--bundle` names only a built-in source set;
custom source sets use the ordinary element lifecycle described in the
authoring guide.

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
a record only until every captured consumer commits or detaches. A projection
gap is fail-stop and local to that Tailapp; without external replay, telemetry
missed while detached is absent.

Mounted exports use explicit `FROM alias.export` or `JOIN alias.export`
relations; comma-listed mounted relations are outside the admitted query
syntax. Give each mounted relation a SQL table alias and qualify its columns
through that alias.

## Verification

```sh
go test ./...
go vet ./...
./scripts/demo.sh
```
