package tailapps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	coverage := normalized.Tables["telemetry_coverage"].Upsert
	if coverage[0]["reason"] != "session and tool identity observed" ||
		coverage[1]["reason"] != "tool target observed" ||
		coverage[2]["reason"] != "progress fingerprint absent, gated, or redacted" {
		t.Fatalf("coverage reasons = %#v", coverage)
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
	unknownCoverage := unknown.Tables["telemetry_coverage"].Upsert
	if unknownCoverage[0]["state"] != "observed" || unknownCoverage[0]["reason"] != "session and tool identity observed" {
		t.Fatalf("tool coverage explains a different capability: %#v", unknownCoverage)
	}
	if unknownCoverage[1]["state"] != "unknown" || unknownCoverage[1]["reason"] != "tool target absent, gated, or redacted" {
		t.Fatalf("target coverage = %#v", unknownCoverage)
	}

	prior = nil
	for position := 10; position <= 12; position++ {
		event := cloneMap(unknown.Events["otel_event"][0])
		event["source_position"] = position
		unknownAnalytic, err = guard.Evaluate("update_guard_analytics", profile.EvaluationInput{
			Meta:  map[string]any{"position": position, "event_id": "local", "event_type": "otel_event", "emission_ordinal": 0},
			Event: event,
			Rows:  map[string]any{"prior": prior},
		})
		if err != nil {
			t.Fatal(err)
		}
		prior = unknownAnalytic.Tables["session_progress"].Upsert[0]
	}
	loops := unknownAnalytic.Tables["loop_findings"].Upsert
	if len(loops) != 1 || loops[0]["finding_kind"] != "repeated-action" {
		t.Fatalf("degraded repeated-action finding = %#v", loops)
	}
	evidence, ok := loops[0]["evidence"].(map[string]any)
	if !ok || evidence["action_fingerprint_coverage"] != "degraded" ||
		evidence["action_fingerprint_reason"] != "tool target absent, gated, or redacted" ||
		evidence["progress_coverage"] != "unknown" {
		t.Fatalf("degraded repeated-action evidence = %#v", loops[0]["evidence"])
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

func TestAgentGuardMapsObservedClaudeOTLPShape(t *testing.T) {
	guard, err := Load("agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	input := observedClaudeInputs(t)["tool_result"]
	normalized, err := guard.Evaluate("normalize_harness_event", input)
	if err != nil {
		t.Fatal(err)
	}
	events := normalized.Events["otel_event"]
	if normalized.Decision != "effective" || len(events) != 1 {
		t.Fatalf("normalized = %#v", normalized)
	}
	event := events[0]
	if event["harness"] != "claude-code" || event["session_id"] != "session-scrubbed" || event["tool"] != "read" {
		t.Fatalf("identity = %#v", event)
	}
	if event["target"] != nil || event["target_coverage"] != "unknown" {
		t.Fatalf("coverage = %#v", event)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "tool_input") || strings.Contains(string(encoded), "<scrubbed>") {
		t.Fatalf("raw tool input escaped normalized output: %s", encoded)
	}
}

func TestSessionCostMapsObservedClaudeUsage(t *testing.T) {
	cost, err := Load("session-cost")
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := cost.Evaluate("normalize_usage", observedClaudeInputs(t)["api_request"])
	if err != nil {
		t.Fatal(err)
	}
	events := normalized.Events["otel_event"]
	if normalized.Decision != "effective" || len(events) != 1 {
		t.Fatalf("normalized = %#v", normalized)
	}
	event := events[0]
	for field, want := range map[string]string{
		"input_tokens": "2", "output_tokens": "118", "cached_input_tokens": "53005", "cost_microusd": "69125",
	} {
		if got := fmt.Sprint(event[field]); got != want {
			t.Fatalf("%s = %s, want %s; event = %#v", field, got, want, event)
		}
	}
}

func TestOpenCodeDEVtheOPSProfileMapsLogsAndIgnoresDuplicateSpan(t *testing.T) {
	inputs := observedOpenCodeInputs(t)

	guard, err := Load("agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	tool, err := guard.Evaluate("normalize_harness_event", inputs["tool_result"])
	if err != nil {
		t.Fatal(err)
	}
	toolEvents := tool.Events["otel_event"]
	if tool.Decision != "effective" || len(toolEvents) != 1 {
		t.Fatalf("tool_result = %#v", tool)
	}
	if event := toolEvents[0]; event["harness"] != "opencode" || event["session_id"] != "session-scrubbed" || event["tool"] != "bash" || event["success"] != true {
		t.Fatalf("tool_result event = %#v", event)
	}
	span, err := guard.Evaluate("normalize_harness_event", inputs["tool_span"])
	if err != nil {
		t.Fatal(err)
	}
	if span.Decision != "ineffective" || len(span.Events["otel_event"]) != 0 {
		t.Fatalf("duplicate content-bearing span = %#v", span)
	}

	cost, err := Load("session-cost")
	if err != nil {
		t.Fatal(err)
	}
	usage, err := cost.Evaluate("normalize_usage", inputs["api_request"])
	if err != nil {
		t.Fatal(err)
	}
	usageEvents := usage.Events["otel_event"]
	if usage.Decision != "effective" || len(usageEvents) != 1 {
		t.Fatalf("api_request = %#v", usage)
	}
	for field, want := range map[string]string{
		"input_tokens": "80", "output_tokens": "20", "cached_input_tokens": "16",
		"reasoning_output_tokens": "7", "cost_microusd": "69125",
	} {
		if got := fmt.Sprint(usageEvents[0][field]); got != want {
			t.Fatalf("%s = %s, want %s; event = %#v", field, got, want, usageEvents[0])
		}
	}
	spanCost, err := cost.Evaluate("normalize_usage", inputs["tool_span"])
	if err != nil {
		t.Fatal(err)
	}
	if spanCost.Decision != "ineffective" || len(spanCost.Events["otel_event"]) != 0 {
		t.Fatalf("span counted as usage = %#v", spanCost)
	}
}

func TestAgentGuardMapsObservedCodexOTLPShape(t *testing.T) {
	guard, err := Load("agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	inputs := observedCodexInputs(t)
	for _, name := range []string{"tool_decision", "tool_result"} {
		t.Run(name, func(t *testing.T) {
			normalized, err := guard.Evaluate("normalize_harness_event", inputs[name])
			if err != nil {
				t.Fatal(err)
			}
			events := normalized.Events["otel_event"]
			if normalized.Decision != "effective" || len(events) != 1 {
				t.Fatalf("normalized = %#v", normalized)
			}
			event := events[0]
			if event["harness"] != "codex" || event["tool"] != "exec_command" || event["session_id"] != "session-scrubbed" {
				t.Fatalf("identity = %#v", event)
			}
			if event["event_time_unix_nano"] != inputs[name].Event["observed_unix_nano"] {
				t.Fatalf("event time did not fall back to observed time: %#v", event)
			}
			if event["target"] != nil || event["target_coverage"] != "unknown" || event["tool_coverage"] != "observed" {
				t.Fatalf("coverage = %#v", event)
			}
			encoded, err := json.Marshal(normalized)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "arguments") || strings.Contains(string(encoded), "<scrubbed>") {
				t.Fatalf("raw arguments escaped normalized output: %s", encoded)
			}
		})
	}
}

func TestCodexServiceIdentitiesNormalizeAcrossBundles(t *testing.T) {
	inputs := observedCodexInputs(t)
	tests := []struct {
		name     string
		tailapp  string
		fold     string
		inputKey string
	}{
		{name: "agent-guard", tailapp: "agent-guard", fold: "normalize_harness_event", inputKey: "app_server_tool_result"},
		{name: "session-cost", tailapp: "session-cost", fold: "normalize_usage", inputKey: "app_server_usage"},
		{name: "activity-stats", tailapp: "activity-stats", fold: "normalize_activity", inputKey: "app_server_tool_result"},
	}
	for _, tc := range tests {
		for _, source := range []string{"codex_cli_rs", "codex_exec", "codex-app-server"} {
			t.Run(tc.name+"/"+source, func(t *testing.T) {
				bundle, err := Load(tc.tailapp)
				if err != nil {
					t.Fatal(err)
				}
				input := inputs[tc.inputKey]
				input.Event = cloneMap(input.Event)
				input.Event["source"] = source
				normalized, err := bundle.Evaluate(tc.fold, input)
				if err != nil {
					t.Fatal(err)
				}
				events := normalized.Events["otel_event"]
				if normalized.Decision != "effective" || len(events) != 1 {
					t.Fatalf("normalized = %#v", normalized)
				}
				if got := events[0]["harness"]; got != "codex" {
					t.Fatalf("harness = %#v, want codex; event = %#v", got, events[0])
				}
			})
		}
	}
}

func TestEmptySessionIdentitiesNormalizeAcrossBundles(t *testing.T) {
	tests := []struct {
		name       string
		tailapp    string
		fold       string
		eventName  string
		attributes map[string]any
	}{
		{name: "agent-guard", tailapp: "agent-guard", fold: "normalize_harness_event", eventName: "codex.tool_result", attributes: map[string]any{"tool_name": "read", "success": true}},
		{name: "session-cost", tailapp: "session-cost", fold: "normalize_usage", eventName: "codex.api_request", attributes: map[string]any{"input_tokens": 1}},
		{name: "activity-stats", tailapp: "activity-stats", fold: "normalize_activity", eventName: "codex.tool_result", attributes: map[string]any{"tool_name": "read", "success": true}},
	}
	for position, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, field := range []string{"session.id", "conversation.id", "session_id"} {
				tc.attributes[field] = ""
			}
			input := harnessInput(position+1, "codex", tc.eventName, tc.attributes)
			resource := input.Event["record"].(map[string]any)["resource"].(map[string]any)
			resource["attributes"].(map[string]any)["service.instance.id"] = ""

			bundle, err := Load(tc.tailapp)
			if err != nil {
				t.Fatal(err)
			}
			normalized, err := bundle.Evaluate(tc.fold, input)
			if err != nil {
				t.Fatal(err)
			}
			events := normalized.Events["otel_event"]
			if normalized.Decision != "effective" || len(events) != 1 {
				t.Fatalf("normalized = %#v", normalized)
			}
			if got := events[0]["session_id"]; got != "unknown:codex" {
				t.Fatalf("session_id = %#v, want unknown:codex; event = %#v", got, events[0])
			}
		})
	}
}

func TestSessionCostMapsObservedCodexSSEUsage(t *testing.T) {
	cost, err := Load("session-cost")
	if err != nil {
		t.Fatal(err)
	}
	input := observedCodexInputs(t)["sse_usage"]
	normalized, err := cost.Evaluate("normalize_usage", input)
	if err != nil {
		t.Fatal(err)
	}
	events := normalized.Events["otel_event"]
	if normalized.Decision != "effective" || len(events) != 1 {
		t.Fatalf("normalized = %#v", normalized)
	}
	event := events[0]
	if event["harness"] != "codex" || event["session_id"] != "session-scrubbed" || event["event_time_unix_nano"] != input.Event["observed_unix_nano"] {
		t.Fatalf("identity = %#v", event)
	}
	for field, want := range map[string]string{
		"input_tokens": "120", "output_tokens": "30", "cached_input_tokens": "40", "reasoning_output_tokens": "9",
	} {
		if got := fmt.Sprint(event[field]); got != want {
			t.Fatalf("%s = %s, want %s; event = %#v", field, got, want, event)
		}
	}
	result, err := cost.Evaluate("accumulate_cost", profile.EvaluationInput{
		Meta:  map[string]any{"position": 43, "event_id": "local:43", "event_type": "otel_event", "emission_ordinal": 0},
		Event: event,
		Rows:  map[string]any{"prior": nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	row := result.Tables["session_cost"].Upsert[0]
	if fmt.Sprint(row["cached_input_tokens"]) != "40" || fmt.Sprint(row["reasoning_output_tokens"]) != "9" || row["last_event_time_unix_nano"] != input.Event["observed_unix_nano"] {
		t.Fatalf("row = %#v", row)
	}
	unrelated, err := cost.Evaluate("normalize_usage", harnessInput(44, "codex_exec", "event <scrubbed-codex-callsite>", map[string]any{
		"event.name": "codex.sse_event", "kind": "response.output_text.delta", "conversation.id": "session-scrubbed",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if unrelated.Decision != "ineffective" || len(unrelated.Events["otel_event"]) != 0 {
		t.Fatalf("unrelated SSE event = %#v", unrelated)
	}
	alternate, err := cost.Evaluate("normalize_usage", harnessInput(45, "codex_cli_rs", "event <scrubbed-codex-callsite>", map[string]any{
		"event.name": "codex.sse_event", "event.kind": "response.completed", "conversation.id": "session-alternate",
		"cached_input_token_count": 7, "reasoning_output_token_count": 3,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if alternate.Decision != "effective" || len(alternate.Events["otel_event"]) != 1 {
		t.Fatalf("alternate Codex counter aliases = %#v", alternate)
	}
	alternateEvent := alternate.Events["otel_event"][0]
	if alternateEvent["harness"] != "codex" || fmt.Sprint(alternateEvent["cached_input_tokens"]) != "7" || fmt.Sprint(alternateEvent["reasoning_output_tokens"]) != "3" {
		t.Fatalf("alternate Codex counter aliases = %#v", alternate)
	}
}

func TestActivityStatsNormalizesCrossHarnessWithoutContent(t *testing.T) {
	stats, err := Load("activity-stats")
	if err != nil {
		t.Fatal(err)
	}
	codex := observedCodexInputs(t)
	claude := observedClaudeInputs(t)
	opencode := observedOpenCodeInputs(t)
	cases := []struct {
		name        string
		input       profile.EvaluationInput
		harness     string
		family      string
		tool        string
		endpoint    string
		performance bool
	}{
		{"claude-response", claude["assistant_response"], "claude-code", "assistant-response", "not-applicable", "not-applicable", false},
		{"codex-prompt", codex["user_prompt"], "codex", "user-prompt", "not-applicable", "not-applicable", false},
		{"codex-ttft", codex["turn_ttft"], "codex", "turn-ttft", "not-applicable", "not-applicable", true},
		{"codex-request", codex["api_request"], "codex", "api-request", "not-applicable", "responses", true},
		{"codex-websocket", codex["websocket_request"], "codex", "websocket-request", "not-applicable", "websocket", true},
		{"opencode-tool", opencode["tool_result"], "opencode", "tool", "shell", "not-applicable", false},
		{"opencode-request", opencode["api_request"], "opencode", "api-request", "not-applicable", "unknown", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			normalized, err := stats.Evaluate("normalize_activity", tc.input)
			if err != nil {
				t.Fatal(err)
			}
			events := normalized.Events["otel_event"]
			if normalized.Decision != "effective" || len(events) != 1 {
				t.Fatalf("normalized = %#v", normalized)
			}
			event := events[0]
			if event["harness"] != tc.harness || event["event_family"] != tc.family || event["tool_bucket"] != tc.tool || event["endpoint_bucket"] != tc.endpoint || event["performance_event"] != tc.performance {
				t.Fatalf("event = %#v", event)
			}
			encoded, err := json.Marshal(normalized)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"prompt_text", "response_text", "tool_input", "arguments", "full_command", "file_path", "<scrubbed>"} {
				if strings.Contains(string(encoded), forbidden) {
					t.Fatalf("%q escaped normalized output: %s", forbidden, encoded)
				}
			}
		})
	}
	ignored, err := stats.Evaluate("normalize_activity", opencode["tool_span"])
	if err != nil {
		t.Fatal(err)
	}
	if ignored.Decision != "ineffective" || len(ignored.Events["otel_event"]) != 0 {
		t.Fatalf("duplicate OpenCode span = %#v", ignored)
	}
}

func TestActivityStatsFoldsCoverageAndLatencyBuckets(t *testing.T) {
	stats, err := Load("activity-stats")
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := stats.Evaluate("normalize_activity", observedCodexInputs(t)["api_request"])
	if err != nil {
		t.Fatal(err)
	}
	event := normalized.Events["otel_event"][0]
	for _, fold := range []string{"update_event_inventory", "update_session_activity", "update_request_performance"} {
		result, err := stats.Evaluate(fold, profile.EvaluationInput{
			Meta:  map[string]any{"position": 46, "event_id": "local:46", "event_type": "otel_event", "emission_ordinal": 0},
			Event: event, Rows: map[string]any{"prior": nil},
		})
		if err != nil {
			t.Fatalf("%s: %v", fold, err)
		}
		switch fold {
		case "update_event_inventory":
			row := result.Tables["event_inventory"].Upsert[0]
			if fmt.Sprint(row["duration_observed_count"]) != "1" || fmt.Sprint(row["token_unknown_count"]) != "1" {
				t.Fatalf("inventory row = %#v", row)
			}
		case "update_session_activity":
			row := result.Tables["session_activity"].Upsert[0]
			if fmt.Sprint(row["request_count"]) != "1" || fmt.Sprint(row["unknown_field_observations"]) == "0" {
				t.Fatalf("session row = %#v", row)
			}
		case "update_request_performance":
			row := result.Tables["request_performance"].Upsert[0]
			if fmt.Sprint(row["duration_le_1000ms"]) != "1" || fmt.Sprint(row["duration_le_500ms"]) != "0" || fmt.Sprint(row["success_count"]) != "1" {
				t.Fatalf("performance row = %#v", row)
			}
		}
	}
}

func TestSessionCostRejectsCounterlessAPIRequest(t *testing.T) {
	cost, err := Load("session-cost")
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := cost.Evaluate("normalize_usage", observedCodexInputs(t)["api_request"])
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Decision != "ineffective" || len(normalized.Events["otel_event"]) != 0 {
		t.Fatalf("counterless api_request = %#v", normalized)
	}
}

func observedCodexInputs(t *testing.T) map[string]profile.EvaluationInput {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("testdata", "codex-cli-0.150.1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]profile.EvaluationInput
	if err := json.Unmarshal(encoded, &inputs); err != nil {
		t.Fatal(err)
	}
	return inputs
}

func observedClaudeInputs(t *testing.T) map[string]profile.EvaluationInput {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("testdata", "claude-code-2.1.250.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]profile.EvaluationInput
	if err := json.Unmarshal(encoded, &inputs); err != nil {
		t.Fatal(err)
	}
	return inputs
}

func observedOpenCodeInputs(t *testing.T) map[string]profile.EvaluationInput {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("testdata", "opencode-adapter.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]profile.EvaluationInput
	if err := json.Unmarshal(encoded, &inputs); err != nil {
		t.Fatal(err)
	}
	return inputs
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
