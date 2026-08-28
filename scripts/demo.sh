#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
demo_tmp=$(mktemp -d "${TMPDIR:-/tmp}/tailapp-demo.XXXXXX")
server_pid=

cleanup() {
  if [ -n "$server_pid" ]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  case "$demo_tmp" in
    "${TMPDIR:-/tmp}"/tailapp-demo.*) rm -rf -- "$demo_tmp" ;;
  esac
}
trap cleanup EXIT INT TERM

binary="$demo_tmp/tailapp"
export TAILAPP_HOME="$demo_tmp/home"

cd "$repo_root"
go build -o "$binary" ./cmd/tailapp
"$binary" init >/dev/null
"$binary" serve --otlp-http 127.0.0.1:0 >"$demo_tmp/server.out" 2>"$demo_tmp/server.err" &
server_pid=$!

attempt=0
while [ ! -S "$TAILAPP_HOME/engine.sock" ]; do
  attempt=$((attempt + 1))
  if [ "$attempt" -gt 200 ]; then
    echo "resident did not start" >&2
    cat "$demo_tmp/server.err" >&2
    exit 1
  fi
  sleep 0.05
done

guard_create=$("$binary" apps create --bundle agent-guard --idempotency-key demo-create-agent-guard agent-guard)
guard_draft=$(printf '%s\n' "$guard_create" | sed -n 's/.*"draft_revision": "\([^"]*\)".*/\1/p')
cost_create=$("$binary" apps create --bundle session-cost --idempotency-key demo-create-session-cost session-cost)
cost_draft=$(printf '%s\n' "$cost_create" | sed -n 's/.*"draft_revision": "\([^"]*\)".*/\1/p')
test -n "$guard_draft"
test -n "$cost_draft"

"$binary" apps validate --expected "$guard_draft" agent-guard >/dev/null
"$binary" apps validate --expected "$cost_draft" session-cost >/dev/null
"$binary" apps activate --expected "$guard_draft" --mode reset --ack-reset --idempotency-key demo-activate-agent-guard agent-guard >/dev/null
"$binary" apps activate --expected "$cost_draft" --mode reset --ack-reset --idempotency-key demo-activate-session-cost session-cost >/dev/null

"$binary" ingest-fixture --signal logs --content-type application/json testdata/otlp/cross-harness.json >/dev/null

attempt=0
while :; do
  health=$("$binary" health)
  if printf '%s\n' "$health" | grep -q '"records": 0'; then
    break
  fi
  attempt=$((attempt + 1))
  if [ "$attempt" -gt 200 ]; then
    echo "projections did not drain" >&2
    printf '%s\n' "$health" >&2
    exit 1
  fi
  sleep 0.05
done

violations=$("$binary" query --sql "SELECT harness, session_id, rule_id, coverage_state FROM policy_findings WHERE rule_id='denied-tool' ORDER BY harness" agent-guard)
unknowns=$("$binary" query --sql "SELECT harness, session_id, rule_id, coverage_state FROM policy_findings WHERE coverage_state='unknown' ORDER BY harness" agent-guard)
loops=$("$binary" query --sql "SELECT harness, session_id, finding_kind FROM loop_findings ORDER BY finding_kind" agent-guard)
stalled=$("$binary" query --sql "SELECT harness, session_id, last_distinct_progress_unix_nano FROM session_progress WHERE last_distinct_progress_unix_nano < ? ORDER BY harness, session_id" --param '"1787900000009999999"' agent-guard)
joined=$("$binary" query --mount cost=session-cost --sql "SELECT progress.harness, progress.session_id, costrow.input_tokens, costrow.output_tokens FROM session_progress progress JOIN cost.session_cost costrow ON costrow.harness=progress.harness AND costrow.session_id=progress.session_id ORDER BY progress.harness" agent-guard)

printf '%s\n' "$violations" | grep -q 'violation-claude'
printf '%s\n' "$violations" | grep -q 'violation-codex'
printf '%s\n' "$violations" | grep -q 'violation-opencode'
printf '%s\n' "$unknowns" | grep -q 'unknown-claude'
printf '%s\n' "$unknowns" | grep -q 'unknown-codex'
printf '%s\n' "$unknowns" | grep -q 'unknown-opencode'
printf '%s\n' "$loops" | grep -q 'repeated-failure'
printf '%s\n' "$loops" | grep -q 'bounded-no-progress'
printf '%s\n' "$stalled" | grep -q 'no-progress-opencode'
printf '%s\n' "$joined" | grep -q 'shared-claude'
printf '%s\n' "$joined" | grep -q 'shared-codex'
printf '%s\n' "$joined" | grep -q 'shared-opencode'

mcp_output=$(printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"tailapp_query","arguments":{"name":"agent-guard","mounts":{"cost":"session-cost"},"sql":"SELECT progress.harness, progress.session_id, costrow.input_tokens, costrow.output_tokens FROM session_progress progress JOIN cost.session_cost costrow ON costrow.harness=progress.harness AND costrow.session_id=progress.session_id ORDER BY progress.harness"}}}' \
  | "$binary" mcp)
printf '%s\n' "$mcp_output" | grep -q 'tailapp_query'
printf '%s\n' "$mcp_output" | grep -q 'shared-opencode'

echo "Tailapp demo passed: cross-harness violations, explicit unknowns, loops, stalled sessions, joined exports, CLI, and MCP."
