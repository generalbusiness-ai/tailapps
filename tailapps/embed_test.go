package tailapps

import (
	"fmt"
	"testing"

	"github.com/generalbusiness-ai/tailapp/internal/profile"
)

func TestBundledTailappsCompile(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			compiled, err := Load(name)
			if err != nil {
				t.Fatal(err)
			}
			if compiled.Revision == "" || compiled.StorageSchemaDigest == "" || compiled.ExportContractDigest == "" {
				t.Fatalf("incomplete identity: %#v", compiled)
			}
		})
	}
}

func TestAgentGuardProducesViolationUnknownAndLoopEvidence(t *testing.T) {
	guard, err := Load("agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	input := harnessInput(1, "codex", "codex.tool_result", map[string]any{
		"conversation.id": "session-1", "tool_name": "dangerous_shell", "target": "/outside/project", "success": false,
	})
	normalized, err := guard.Evaluate("normalize_harness_event", input)
	if err != nil {
		t.Fatal(err)
	}
	events := normalized.Events["otel_event"]
	if len(events) != 1 || events[0]["tool"] != "dangerous_shell" {
		t.Fatalf("normalized = %#v", normalized)
	}
	if got := len(normalized.Tables["telemetry_coverage"].Upsert); got != 3 {
		t.Fatalf("coverage rows = %d", got)
	}

	prior := any(nil)
	var analytic profile.EvaluationResult
	for position := 1; position <= 3; position++ {
		event := cloneMap(events[0])
		event["source_position"] = position
		analytic, err = guard.Evaluate("update_guard_analytics", profile.EvaluationInput{
			Meta:  map[string]any{"position": position, "event_id": "local", "event_type": "otel_event", "emission_ordinal": 0},
			Event: event,
			Rows:  map[string]any{"prior": prior},
		})
		if err != nil {
			t.Fatal(err)
		}
		prior = analytic.Tables["session_progress"].Upsert[0]
	}
	if len(analytic.Tables["policy_findings"].Upsert) != 1 {
		t.Fatalf("policy findings = %#v", analytic)
	}
	if len(analytic.Tables["loop_findings"].Upsert) != 1 {
		t.Fatalf("loop findings = %#v", analytic)
	}

	unknownInput := harnessInput(9, "claude-code", "claude_code.tool_result", map[string]any{
		"session.id": "session-2", "tool_name": "read", "success": true,
	})
	unknown, err := guard.Evaluate("normalize_harness_event", unknownInput)
	if err != nil {
		t.Fatal(err)
	}
	unknownAnalytic, err := guard.Evaluate("update_guard_analytics", profile.EvaluationInput{
		Meta:  map[string]any{"position": 9, "event_id": "local:9", "event_type": "otel_event", "emission_ordinal": 0},
		Event: unknown.Events["otel_event"][0], Rows: map[string]any{"prior": nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	finding := unknownAnalytic.Tables["policy_findings"].Upsert
	if len(finding) != 1 || finding[0]["coverage_state"] != "unknown" || finding[0]["rule_id"] != "insufficient-telemetry" {
		t.Fatalf("unknown finding = %#v", finding)
	}
	progress := unknownAnalytic.Tables["session_progress"].Upsert[0]
	if fmt.Sprint(progress["no_progress_count"]) != "0" {
		t.Fatalf("missing progress telemetry counted as no progress: %#v", progress)
	}
}

func TestSessionCostAccumulates(t *testing.T) {
	cost, err := Load("session-cost")
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := cost.Evaluate("normalize_usage", harnessInput(1, "codex", "codex.api_request", map[string]any{
		"conversation.id": "session-1", "input_tokens": 100, "output_tokens": 25, "cost_microusd": 7,
	}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := cost.Evaluate("accumulate_cost", profile.EvaluationInput{
		Meta:  map[string]any{"position": 1, "event_id": "local:1", "event_type": "otel_event", "emission_ordinal": 0},
		Event: normalized.Events["otel_event"][0], Rows: map[string]any{"prior": nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	row := result.Tables["session_cost"].Upsert[0]
	if row["input_tokens"].(interface{ String() string }).String() != "100" {
		t.Fatalf("row = %#v", row)
	}
}

func TestSessionCostMapsClaudeNativeCost(t *testing.T) {
	cost, err := Load("session-cost")
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := cost.Evaluate("normalize_usage", harnessInput(1, "claude-code", "claude_code.api_request", map[string]any{
		"session.id": "session-1", "input_tokens": 100, "output_tokens": 25, "cost_usd_micros": 11,
	}))
	if err != nil {
		t.Fatal(err)
	}
	events := normalized.Events["otel_event"]
	if len(events) != 1 || events[0]["cost_microusd"].(interface{ String() string }).String() != "11" {
		t.Fatalf("normalized = %#v", normalized)
	}
}

func harnessInput(position int, source, name string, attributes map[string]any) profile.EvaluationInput {
	return profile.EvaluationInput{
		Meta: map[string]any{"position": position, "event_id": "local", "event_type": "otlp_record"},
		Event: map[string]any{
			"id": "local", "signal": "log", "name": name, "source": source,
			"time_unix_nano": "1787900000000000000", "observed_unix_nano": nil,
			"trace_id": nil, "span_id": nil, "content_digest": "digest",
			"record": map[string]any{"attributes": attributes, "resource": map[string]any{"attributes": map[string]any{"service.instance.id": "instance-1"}}},
		},
		Rows: map[string]any{},
	}
}

func cloneMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
