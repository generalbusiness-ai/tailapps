package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/generalbusiness-ai/tailapps/internal/inbox"
	"github.com/generalbusiness-ai/tailapps/internal/profile"
	"github.com/generalbusiness-ai/tailapps/tailapps"
)

func TestAgentGuardMaterializesAllHarnessesUnknownLoopsAndStalledQuery(t *testing.T) {
	guard, err := tailapps.Load("agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.sqlite")
	projection, err := Create(context.Background(), path, guard, 0, "reset")
	if err != nil {
		t.Fatal(err)
	}

	position := int64(0)
	for _, harness := range []struct{ source, name string }{
		{"claude-code", "claude_code.tool_result"}, {"codex", "codex.tool_result"}, {"opencode", "tool_result"},
	} {
		position++
		if _, err := projection.Process(context.Background(), guardDelivery(position, harness.source, harness.name, map[string]any{
			"session.id": "violation-" + harness.source, "tool_name": "dangerous_shell", "target": "/outside/project", "success": false,
		})); err != nil {
			t.Fatalf("%s violation: %v", harness.source, err)
		}
		position++
		if _, err := projection.Process(context.Background(), guardDelivery(position, harness.source, harness.name, map[string]any{
			"session.id": "allowed-" + harness.source, "tool_name": "read", "target": "/workspace/file", "success": true,
		})); err != nil {
			t.Fatalf("%s allowed: %v", harness.source, err)
		}
		position++
		if _, err := projection.Process(context.Background(), guardDelivery(position, harness.source, harness.name, map[string]any{
			"session.id": "unknown-" + harness.source, "tool_name": "read", "success": true,
		})); err != nil {
			t.Fatalf("%s unknown: %v", harness.source, err)
		}
	}

	// Three identical failures form both failure and repetition evidence; the
	// guard chooses the more specific repeated-failure signal.
	for count := 0; count < 3; count++ {
		position++
		if _, err := projection.Process(context.Background(), guardDelivery(position, "codex", "codex.tool_result", map[string]any{
			"conversation.id": "loop", "tool_name": "shell", "target": "/workspace", "success": false,
		})); err != nil {
			t.Fatal(err)
		}
	}
	// Different actions with one explicitly observed, unchanged progress
	// fingerprint accumulate no-progress without conflating missing telemetry.
	for count := 0; count < 4; count++ {
		position++
		if _, err := projection.Process(context.Background(), guardDelivery(position, "opencode", "tool_result", map[string]any{
			"session_id": "no-progress", "tool_name": fmt.Sprintf("tool-%d", count), "target": "/workspace", "success": true, "progress_fingerprint": "stable-state",
		})); err != nil {
			t.Fatal(err)
		}
	}

	assertCount(t, projection, `SELECT COUNT(*) FROM policy_findings WHERE rule_id='denied-tool'`, 3)
	assertCount(t, projection, `SELECT COUNT(*) FROM policy_findings WHERE rule_id='insufficient-telemetry'`, 3)
	assertCount(t, projection, `SELECT COUNT(*) FROM loop_findings WHERE finding_kind='repeated-failure'`, 1)
	assertCount(t, projection, `SELECT COUNT(*) FROM loop_findings WHERE finding_kind='bounded-no-progress'`, 1)
	assertCount(t, projection, `SELECT COUNT(*) FROM session_progress WHERE last_distinct_progress_unix_nano < ?`, 9, "0000000000000000999")
	frontier, err := projection.Frontier(context.Background())
	if err != nil || frontier.InterpretedPosition != position || !frontier.Complete {
		t.Fatalf("frontier = %#v, %v", frontier, err)
	}

	if err := projection.Close(); err != nil {
		t.Fatal(err)
	}
	projection, err = Open(context.Background(), path, guard)
	if err != nil {
		t.Fatal(err)
	}
	defer projection.Close()
	last := guardDelivery(position, "opencode", "tool_result", map[string]any{"session_id": "no-progress", "tool_name": "tool-2", "target": "/workspace"})
	result, err := projection.Process(context.Background(), last)
	if err != nil || !result.AlreadyApplied {
		t.Fatalf("recovery re-evaluated committed delivery: %#v, %v", result, err)
	}
}

func TestProjectionFailureRollsBackAndOpensGap(t *testing.T) {
	compiled, err := profile.Load(fstest.MapFS{
		"application.sql": {Data: []byte(`
CREATE EVENT otel_event (id TEXT NOT NULL);
CREATE TABLE state (id TEXT PRIMARY KEY, status TEXT NOT NULL CHECK(status='valid'));
CREATE TABLE analytic (id TEXT PRIMARY KEY);
CREATE NORMALIZER normalize ON otlp_record USING 'folds/normalize.jsonata' WRITES state EMITS otel_event;
CREATE FOLD fold ON otel_event USING 'folds/fold.jsonata' WRITES analytic;
CREATE EXPORT state AS SELECT id, status FROM state;`)},
		"folds/normalize.jsonata": {Data: []byte(`{"decision":"effective","facts":[],"events":{"otel_event":[]},"tables":{"state":{"upsert":[{"id":"x","status":"invalid"}]}}}`)},
		"folds/fold.jsonata":      {Data: []byte(`{"decision":"effective","facts":[],"tables":{"analytic":{"upsert":[]}}}`)},
	}, ".", "failing")
	if err != nil {
		t.Fatal(err)
	}
	projection, err := Create(context.Background(), filepath.Join(t.TempDir(), "state.sqlite"), compiled, 0, "reset")
	if err != nil {
		t.Fatal(err)
	}
	defer projection.Close()
	_, err = projection.Process(context.Background(), guardDelivery(1, "codex", "ignored", map[string]any{}))
	if err == nil {
		t.Fatal("constraint failure succeeded")
	}
	frontier, err := projection.Frontier(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if frontier.InterpretedPosition != 0 || frontier.Complete || frontier.GapPosition == nil || *frontier.GapPosition != 1 {
		t.Fatalf("gap frontier = %#v", frontier)
	}
	assertCount(t, projection, `SELECT COUNT(*) FROM state`, 0)
}

func TestCancelledProcessingLeavesDeliveryPendingWithoutGap(t *testing.T) {
	compiled, err := tailapps.Load("agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	item, err := Create(context.Background(), filepath.Join(t.TempDir(), "guard.sqlite"), compiled, 0, "reset")
	if err != nil {
		t.Fatal(err)
	}
	defer item.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := item.Process(ctx, guardDelivery(1, "codex", "codex.tool_result", map[string]any{
		"conversation.id": "retry", "tool_name": "read", "target": "/workspace", "success": true,
	})); err == nil {
		t.Fatal("cancelled processing succeeded")
	}
	frontier, err := item.Frontier(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if frontier.GapPosition != nil || !frontier.Complete || frontier.InterpretedPosition != 0 {
		t.Fatalf("cancelled processing changed frontier: %#v", frontier)
	}
}

func TestJSONNumberRoundTripsThroughDeclaredRead(t *testing.T) {
	value := json.Number("42")
	stored := sqliteValue(value, profile.TypeJSON)
	if stored != "42" {
		t.Fatalf("stored JSON number = %#v", stored)
	}
	decoded, err := fromSQLite(stored, "JSON")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(decoded) != "42" {
		t.Fatalf("decoded JSON number = %#v", decoded)
	}
}

func guardDelivery(position int64, source, name string, attributes map[string]any) inbox.Delivery {
	record, _ := json.Marshal(map[string]any{"attributes": attributes, "resource": map[string]any{"attributes": map[string]any{"service.name": source}}})
	time := fmt.Sprintf("%019d", position*100)
	return inbox.Delivery{Position: position, EventID: fmt.Sprintf("local:%d", position), Signal: "log", Name: name, Source: source,
		TimeUnixNano: &time, ContentDigest: fmt.Sprintf("sha256:%064d", position), JSON: record, Revision: "test"}
}

func assertCount(t *testing.T, projection *Projection, query string, expected int, args ...any) {
	t.Helper()
	var count int
	if err := projection.Database().QueryRowContext(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != expected {
		t.Fatalf("%s: got %d, want %d", strings.TrimSpace(query), count, expected)
	}
}

// TestFoldReadsExecuteUnderTheDefaultDenyAuthorizer is the stage-6 closing
// criterion: the compiled read plan runs with the core's default-deny
// authorizer seated on the projection connection, so a read outside the
// plan is denied at execution time - not only by the compile-time textual
// checks - and the host's own writes stay unrestricted once the plan
// finishes.
func TestFoldReadsExecuteUnderTheDefaultDenyAuthorizer(t *testing.T) {
	ctx := context.Background()
	guard, err := tailapps.Load("agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.sqlite")
	projection, err := Create(ctx, path, guard, 0, "reset")
	if err != nil {
		t.Fatal(err)
	}
	defer projection.Close()
	tx, err := projection.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var fold profile.Program
	for _, candidate := range guard.Folds {
		if candidate.Name == "update_guard_analytics" {
			fold = candidate
		}
	}
	if fold.Name == "" || len(fold.Reads) == 0 {
		t.Fatal("fixture fold with reads is missing")
	}
	event := map[string]any{"harness": "codex", "session_id": "s1"}
	input, err := projection.evaluationInput(ctx, tx, fold, 1, "e1", "otel_event", event)
	if err != nil {
		t.Fatalf("the compiled plan must execute under its own authorizer: %v", err)
	}
	if _, present := input.Rows["prior"]; !present {
		t.Fatalf("plan read missing from input rows: %#v", input.Rows)
	}
	doctored := fold
	doctored.Reads = []profile.Read{{Name: "probe", Cardinality: profile.Many, Limit: 4,
		SQL: "SELECT revision FROM tailapp_projection_identity", Table: "tailapp_projection_identity", Columns: []string{"revision"}}}
	_, err = projection.evaluationInput(ctx, tx, doctored, 1, "e1", "otel_event", event)
	if err == nil || !(strings.Contains(err.Error(), "prohibited") || strings.Contains(err.Error(), "not authorized")) {
		t.Fatalf("a read outside the compiled plan must be denied at execution time: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tailapp_stats SET consumed_records = consumed_records WHERE singleton=1`); err != nil {
		t.Fatalf("host writes must be unrestricted after the plan releases the guard: %v", err)
	}
}
