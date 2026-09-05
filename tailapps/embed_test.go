package tailapps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/generalbusiness-ai/tailapps/internal/profile"
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

func TestDailyReviewExportsDailyReview(t *testing.T) {
	compiled, err := Load("daily-review")
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Exports["daily_review"].Name != "daily_review" {
		t.Fatalf("compiled profile = %#v", compiled)
	}
}

func TestSignalCountsExportsSignalCounts(t *testing.T) {
	compiled, err := Load("signal-counts")
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Exports["signal_counts"].Name != "signal_counts" {
		t.Fatalf("compiled profile = %#v", compiled)
	}
}

func TestSignalCountsRetainsFirstAndLastObservedTimestamps(t *testing.T) {
	counts, err := Load("signal-counts")
	if err != nil {
		t.Fatal(err)
	}
	input := harnessInput(1, "codex", "codex.tool_result", map[string]any{})
	first, err := counts.Evaluate("normalize_signal", input)
	if err != nil {
		t.Fatal(err)
	}
	firstEvent := first.Events["otel_event"][0]
	firstFold, err := counts.Evaluate("count_signal", profile.EvaluationInput{
		Meta: map[string]any{"position": 1, "event_id": "local#0", "event_type": "otel_event"}, Event: firstEvent, Rows: map[string]any{"prior": nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstRow := firstFold.Tables["signal_counts"].Upsert[0]
	input.Event["time_unix_nano"] = "1787900000000000500"
	input.Meta["position"] = 2
	second, err := counts.Evaluate("normalize_signal", input)
	if err != nil {
		t.Fatal(err)
	}
	secondFold, err := counts.Evaluate("count_signal", profile.EvaluationInput{
		Meta: map[string]any{"position": 2, "event_id": "local#0", "event_type": "otel_event"}, Event: second.Events["otel_event"][0], Rows: map[string]any{"prior": firstRow},
	})
	if err != nil {
		t.Fatal(err)
	}
	row := secondFold.Tables["signal_counts"].Upsert[0]
	if row["first_seen_unix_nano"] != "1787900000000000000" || row["last_seen_unix_nano"] != "1787900000000000500" {
		t.Fatalf("signal timestamps = %#v", row)
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
	prior := any(nil)
	var analytic profile.EvaluationResult
	for position := 1; position <= 3; position++ {
		event := cloneMap(events[0])
		event["source_position"] = position
		analytic, err = guard.Evaluate("update_guard_analytics", profile.EvaluationInput{
			Meta:  map[string]any{"position": position, "event_id": "local#0", "event_type": "otel_event"},
			Event: event,
			Rows:  guardAnalyticRows(prior),
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
	coverage := analytic.Tables["telemetry_coverage"].Upsert
	if len(coverage) != 3 || coverage[0]["reason"] != "session and tool identity observed" ||
		coverage[1]["reason"] != "tool target observed" ||
		coverage[2]["reason"] != "progress fingerprint absent, gated, or redacted" {
		t.Fatalf("coverage reasons = %#v", coverage)
	}

	unknownInput := harnessInput(9, "claude-code", "claude_code.tool_result", map[string]any{
		"session.id": "session-2", "tool_name": "read", "success": true,
	})
	unknown, err := guard.Evaluate("normalize_harness_event", unknownInput)
	if err != nil {
		t.Fatal(err)
	}
	unknownAnalytic, err := guard.Evaluate("update_guard_analytics", profile.EvaluationInput{
		Meta:  map[string]any{"position": 9, "event_id": "local#0", "event_type": "otel_event"},
		Event: unknown.Events["otel_event"][0], Rows: guardAnalyticRows(nil),
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
	unknownCoverage := unknownAnalytic.Tables["telemetry_coverage"].Upsert
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
			Meta:  map[string]any{"position": position, "event_id": "local#0", "event_type": "otel_event"},
			Event: event,
			Rows:  guardAnalyticRows(prior),
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
		Meta:  map[string]any{"position": 1, "event_id": "local#0", "event_type": "otel_event"},
		Event: normalized.Events["otel_event"][0], Rows: map[string]any{"prior": nil, "detail": nil},
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

func TestDailyReviewMapsObservedClaudeNativeNames(t *testing.T) {
	review, err := Load("daily-review")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tool_result", "api_request"} {
		normalized, err := review.Evaluate("normalize_review_event", observedClaudeInputs(t)[name])
		if err != nil {
			t.Fatal(err)
		}
		events := normalized.Events["otel_event"]
		if normalized.Decision != "effective" || len(events) != 1 {
			t.Fatalf("%s normalized = %#v", name, normalized)
		}
		if name == "tool_result" && fmt.Sprint(events[0]["tool_event"]) != "1" {
			t.Fatalf("tool event = %#v", events[0])
		}
		if name == "api_request" && (fmt.Sprint(events[0]["api_event"]) != "1" || fmt.Sprint(events[0]["input_tokens"]) != "2") {
			t.Fatalf("usage event = %#v", events[0])
		}
	}
}

func TestSessionCostRetainsTelemetryDimensions(t *testing.T) {
	cost, err := Load("session-cost")
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := cost.Evaluate("normalize_usage", harnessInput(12, "codex", "codex.api_request", map[string]any{
		"conversation.id": "0198aabbccddeeff", "model": "gpt-5.6", "cwd": "/work/tailapps",
		"input_tokens": 100, "output_tokens": 25, "cost_microusd": 7,
	}))
	if err != nil {
		t.Fatal(err)
	}
	event := normalized.Events["otel_event"][0]
	if event["session_id_prefix"] != "0198aabbccdd" || event["model"] != "gpt-5.6" || event["project"] != "/work/tailapps" {
		t.Fatalf("dimensions = %#v", event)
	}
	result, err := cost.Evaluate("accumulate_cost", profile.EvaluationInput{
		Meta:  map[string]any{"position": 12, "event_id": "local#0", "event_type": "otel_event"},
		Event: event,
		Rows:  map[string]any{"prior": nil, "detail": nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail := result.Tables["session_cost_detail"].Upsert[0]
	if detail["model"] != "gpt-5.6" || detail["project"] != "/work/tailapps" || fmt.Sprint(detail["cost_microusd"]) != "7" {
		t.Fatalf("detail = %#v", detail)
	}
}

func TestSessionCostDetailKeepsMixedModelsSeparate(t *testing.T) {
	cost, err := Load("session-cost")
	if err != nil {
		t.Fatal(err)
	}
	for position, model := range []string{"claude-opus-4-1", "claude-haiku-3-5"} {
		normalized, err := cost.Evaluate("normalize_usage", harnessInput(position+20, "claude-code", "claude_code.api_request", map[string]any{
			"session.id": "mixed-model-session", "model": model,
			"input_tokens": 10, "cost_usd_micros": 3,
		}))
		if err != nil {
			t.Fatal(err)
		}
		event := normalized.Events["otel_event"][0]
		result, err := cost.Evaluate("accumulate_cost", profile.EvaluationInput{
			Meta:  map[string]any{"position": position + 20, "event_id": "local#0", "event_type": "otel_event"},
			Event: event,
			Rows:  map[string]any{"prior": nil, "detail": nil},
		})
		if err != nil {
			t.Fatal(err)
		}
		detail := result.Tables["session_cost_detail"].Upsert[0]
		if detail["model"] != model || fmt.Sprint(detail["cost_microusd"]) != "3" {
			t.Fatalf("detail[%d] = %#v", position, detail)
		}
	}
}

func TestAgentGuardRetainsFailedToolTelemetry(t *testing.T) {
	guard, err := Load("agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := guard.Evaluate("normalize_harness_event", harnessInput(13, "codex", "codex.tool_result", map[string]any{
		"conversation.id": "0198aabbccddeeff", "tool_name": "exec_command", "success": false,
		"cwd": "/work/tailapps", "model": "gpt-5.6", "full_command": "go test ./...",
		"arguments": `{"cmd":"go test ./..."}`, "error.message": "exit status 1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	event := normalized.Events["otel_event"][0]
	result, err := guard.Evaluate("update_guard_analytics", profile.EvaluationInput{
		Meta:  map[string]any{"position": 13, "event_id": "local#0", "event_type": "otel_event"},
		Event: event,
		Rows:  guardAnalyticRows(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := result.Tables["tool_failure_detail"].Upsert
	if len(rows) != 1 {
		t.Fatalf("failure rows = %#v", rows)
	}
	row := rows[0]
	if row["command"] != "go test ./..." || row["tool_arguments"] != `{"cmd":"go test ./..."}` || row["failure_detail"] != "exit status 1" || row["project"] != "/work/tailapps" {
		t.Fatalf("failure detail = %#v", row)
	}
}

func TestAgentGuardFindingAndCoverageTimestamps(t *testing.T) {
	guard, err := Load("agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	input := observedCodexInputs(t)["tool_result"]
	first, err := guard.Evaluate("normalize_harness_event", input)
	if err != nil {
		t.Fatal(err)
	}
	firstEvent := cloneMap(first.Events["otel_event"][0])
	firstEvent["success"] = false
	var session any
	coverageRows := map[string]any{}
	var result profile.EvaluationResult
	for position := 1; position <= 3; position++ {
		event := cloneMap(firstEvent)
		event["source_position"] = position
		rows := map[string]any{
			"prior":                   session,
			"tool_coverage_prior":     coverageRows["tool-observation"],
			"target_coverage_prior":   coverageRows["target-detail"],
			"progress_coverage_prior": coverageRows["progress-detail"],
		}
		result, err = guard.Evaluate("update_guard_analytics", profile.EvaluationInput{
			Meta: map[string]any{"position": position, "event_id": fmt.Sprint(input.Meta["event_id"]) + "#0", "event_type": "otel_event"}, Event: event, Rows: rows,
		})
		if err != nil {
			t.Fatal(err)
		}
		session = result.Tables["session_progress"].Upsert[0]
		for _, row := range result.Tables["telemetry_coverage"].Upsert {
			coverageRows[fmt.Sprint(row["capability"])] = row
		}
	}
	want := fmt.Sprint(firstEvent["event_time_unix_nano"])
	policy := result.Tables["policy_findings"].Upsert[0]
	loop := result.Tables["loop_findings"].Upsert[0]
	if policy["observed_unix_nano"] != want || loop["first_observed_unix_nano"] != want || loop["last_observed_unix_nano"] != want {
		t.Fatalf("finding timestamps: policy=%#v loop=%#v", policy, loop)
	}
	for capability, value := range coverageRows {
		row := value.(map[string]any)
		if row["first_seen_unix_nano"] != want || row["last_seen_unix_nano"] != want {
			t.Fatalf("%s coverage timestamps = %#v", capability, row)
		}
	}
}

func TestAgentGuardDoesNotTreatToolOutputAsFailureDetail(t *testing.T) {
	guard, err := Load("agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := guard.Evaluate("normalize_harness_event", harnessInput(14, "codex", "codex.tool_result", map[string]any{
		"conversation.id": "session", "tool_name": "exec_command", "success": false,
		"output": "secret command output", "tool_content": "secret tool content",
	}))
	if err != nil {
		t.Fatal(err)
	}
	event := normalized.Events["otel_event"][0]
	if event["failure_detail"] != "" {
		t.Fatalf("failure_detail = %#v", event["failure_detail"])
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

func TestVSCodeCopilotChatMapsObservedOTLPShape(t *testing.T) {
	inputs := observedVSCodeCopilotInputs(t)

	guard, err := Load("agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	tool, err := guard.Evaluate("normalize_harness_event", inputs["tool_call"])
	if err != nil {
		t.Fatal(err)
	}
	if tool.Decision != "effective" || len(tool.Events["otel_event"]) != 1 {
		t.Fatalf("tool = %#v", tool)
	}
	toolEvent := tool.Events["otel_event"][0]
	if toolEvent["harness"] != "vscode-copilot-chat" || toolEvent["session_id"] != "resource-session-scrubbed" || toolEvent["tool"] != "manage_todo_list" || toolEvent["success"] != true {
		t.Fatalf("tool identity = %#v", toolEvent)
	}
	if toolEvent["target"] != nil || toolEvent["target_coverage"] != "unknown" {
		t.Fatalf("tool coverage = %#v", toolEvent)
	}
	defaultTool, err := guard.Evaluate("normalize_harness_event", inputs["default_tool_call"])
	if err != nil {
		t.Fatal(err)
	}
	if defaultTool.Decision != "effective" || len(defaultTool.Events["otel_event"]) != 1 {
		t.Fatalf("default tool = %#v", defaultTool)
	}
	defaultToolEvent := defaultTool.Events["otel_event"][0]
	if defaultToolEvent["harness"] != "vscode-copilot-chat" || defaultToolEvent["target"] != nil || defaultToolEvent["target_coverage"] != "unknown" {
		t.Fatalf("default tool identity or content boundary = %#v", defaultToolEvent)
	}

	cost, err := Load("session-cost")
	if err != nil {
		t.Fatal(err)
	}
	usage, err := cost.Evaluate("normalize_usage", inputs["inference_details"])
	if err != nil {
		t.Fatal(err)
	}
	if usage.Decision != "effective" || len(usage.Events["otel_event"]) != 1 {
		t.Fatalf("usage = %#v", usage)
	}
	usageEvent := usage.Events["otel_event"][0]
	if usageEvent["harness"] != "vscode-copilot-chat" || usageEvent["session_id"] != "resource-session-scrubbed" {
		t.Fatalf("usage identity = %#v", usageEvent)
	}
	for field, want := range map[string]string{"input_tokens": "275", "output_tokens": "5", "cached_input_tokens": "0", "cost_microusd": "0"} {
		if got := fmt.Sprint(usageEvent[field]); got != want {
			t.Fatalf("%s = %s, want %s; event = %#v", field, got, want, usageEvent)
		}
	}
	defaultUsage, err := cost.Evaluate("normalize_usage", inputs["default_inference_details"])
	if err != nil {
		t.Fatal(err)
	}
	if defaultUsage.Decision != "effective" || len(defaultUsage.Events["otel_event"]) != 1 || defaultUsage.Events["otel_event"][0]["harness"] != "vscode-copilot-chat" {
		t.Fatalf("default usage identity = %#v", defaultUsage)
	}
	duplicate, err := cost.Evaluate("normalize_usage", inputs["inference_span"])
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Decision != "ineffective" || len(duplicate.Events["otel_event"]) != 0 {
		t.Fatalf("duplicate span counted as usage = %#v", duplicate)
	}

	stats, err := Load("activity-stats")
	if err != nil {
		t.Fatal(err)
	}
	activity, err := stats.Evaluate("normalize_activity", inputs["inference_details"])
	if err != nil {
		t.Fatal(err)
	}
	if activity.Decision != "effective" || len(activity.Events["otel_event"]) != 1 {
		t.Fatalf("activity = %#v", activity)
	}
	activityEvent := activity.Events["otel_event"][0]
	if activityEvent["event_family"] != "api-request" || activityEvent["endpoint_bucket"] != "unknown" || activityEvent["performance_event"] != true {
		t.Fatalf("activity identity = %#v", activityEvent)
	}
	if got := fmt.Sprint(activityEvent["input_tokens"]); got != "275" {
		t.Fatalf("activity input tokens = %s; event = %#v", got, activityEvent)
	}
	defaultActivity, err := stats.Evaluate("normalize_activity", inputs["default_inference_details"])
	if err != nil {
		t.Fatal(err)
	}
	if defaultActivity.Decision != "effective" || len(defaultActivity.Events["otel_event"]) != 1 || defaultActivity.Events["otel_event"][0]["harness"] != "vscode-copilot-chat" {
		t.Fatalf("default activity identity = %#v", defaultActivity)
	}

	daily, err := Load("daily-review")
	if err != nil {
		t.Fatal(err)
	}
	review, err := daily.Evaluate("normalize_review_event", inputs["inference_details"])
	if err != nil {
		t.Fatal(err)
	}
	if review.Decision != "effective" || len(review.Events["otel_event"]) != 1 {
		t.Fatalf("daily review = %#v", review)
	}
	reviewEvent := review.Events["otel_event"][0]
	if reviewEvent["harness"] != "vscode-copilot-chat" || fmt.Sprint(reviewEvent["api_event"]) != "1" || fmt.Sprint(reviewEvent["input_tokens"]) != "275" || fmt.Sprint(reviewEvent["output_tokens"]) != "5" {
		t.Fatalf("daily review event = %#v", reviewEvent)
	}
	defaultReview, err := daily.Evaluate("normalize_review_event", inputs["default_inference_details"])
	if err != nil {
		t.Fatal(err)
	}
	if defaultReview.Decision != "effective" || len(defaultReview.Events["otel_event"]) != 1 || defaultReview.Events["otel_event"][0]["harness"] != "vscode-copilot-chat" {
		t.Fatalf("default daily review identity = %#v", defaultReview)
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
			if strings.Contains(string(encoded), "<scrubbed>") || event["tool_arguments"] != "" {
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
		Meta:  map[string]any{"position": 43, "event_id": fmt.Sprint(input.Meta["event_id"]) + "#0", "event_type": "otel_event"},
		Event: event,
		Rows:  map[string]any{"prior": nil, "detail": nil},
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
	input := observedCodexInputs(t)["api_request"]
	normalized, err := stats.Evaluate("normalize_activity", input)
	if err != nil {
		t.Fatal(err)
	}
	event := normalized.Events["otel_event"][0]
	for _, fold := range []string{"update_event_inventory", "update_session_activity", "update_request_performance"} {
		result, err := stats.Evaluate(fold, profile.EvaluationInput{
			Meta:  map[string]any{"position": 46, "event_id": fmt.Sprint(input.Meta["event_id"]) + "#0", "event_type": "otel_event"},
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

func BenchmarkTelemetryNormalizers(b *testing.B) {
	inputs := []struct {
		bundle  string
		program string
		input   profile.EvaluationInput
	}{
		{"activity-stats", "normalize_activity", harnessInput(1, "codex", "codex.tool_result", map[string]any{"conversation.id": "bench", "tool_name": "exec_command", "success": false, "arguments": `{"cmd":"go test ./..."}`})},
		{"daily-review", "normalize_review_event", harnessInput(2, "claude-code", "claude_code.api_request", map[string]any{"session.id": "bench", "input_tokens": 100, "output_tokens": 20})},
		{"agent-guard", "normalize_harness_event", harnessInput(3, "codex", "codex.tool_result", map[string]any{"conversation.id": "bench", "tool_name": "exec_command", "success": false, "full_command": "go test ./...", "error.message": "exit status 1"})},
		{"session-cost", "normalize_usage", harnessInput(4, "codex", "codex.api_request", map[string]any{"conversation.id": "bench", "model": "gpt-5.6", "cwd": "/work/tailapps", "input_tokens": 100, "output_tokens": 20})},
	}
	for _, item := range inputs {
		compiled, err := Load(item.bundle)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(item.bundle, func(b *testing.B) {
			for range b.N {
				if _, err := compiled.Evaluate(item.program, item.input); err != nil {
					b.Fatal(err)
				}
			}
		})
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

func TestCurrentClaudeFixtureDocumentsStageZeroEvidence(t *testing.T) {
	inputs := currentClaudeInputs(t)
	for name, body := range map[string]string{
		"api_request":           "claude_code.api_request",
		"subagent_api_request":  "claude_code.api_request",
		"api_error":             "claude_code.api_error",
		"tool_result":           "claude_code.tool_result",
		"failed_tool_result":    "claude_code.tool_result",
		"tool_decision":         "claude_code.tool_decision",
		"mcp_connection_failed": "claude_code.mcp_server_connection",
	} {
		attributes := fixtureAttributes(t, inputs[name])
		if inputs[name].Event["source"] != "claude-code" || inputs[name].Event["name"] != attributes["event.name"] {
			t.Fatalf("%s identity = %#v", name, inputs[name].Event)
		}
		if record := inputs[name].Event["record"].(map[string]any); record["body"] != body {
			t.Fatalf("%s body = %#v, want %q", name, record["body"], body)
		}
	}
	if got := fixtureAttributes(t, inputs["subagent_api_request"])["agent.name"]; got != "Explore" {
		t.Fatalf("subagent attribution = %#v", got)
	}
	if got := fmt.Sprint(fixtureAttributes(t, inputs["api_error"])["status_code"]); got != "401" {
		t.Fatalf("api error status = %q", got)
	}
	if got := fixtureAttributes(t, inputs["failed_tool_result"])["success"]; got != false {
		t.Fatalf("failed tool outcome = %#v", got)
	}
	if got := fixtureAttributes(t, inputs["mcp_connection_failed"])["error_code"]; got != "CONNECTION_CLOSED" {
		t.Fatalf("mcp failure = %#v", got)
	}

	encoded, err := json.Marshal(inputs)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"prompt"`, `"response"`, `"tool_input"`, `"tool_parameters"`,
		`"user.`, `"organization.`, `"request_id"`, `"tool_use_id"`, `"message.uuid"`,
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("current fixture retains forbidden field %q", forbidden)
		}
	}

	cost, err := Load("session-cost")
	if err != nil {
		t.Fatal(err)
	}
	result, err := cost.Evaluate("normalize_usage", inputs["api_request"])
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != "effective" || len(result.Events["otel_event"]) != 1 {
		t.Fatalf("current Claude request = %#v", result)
	}
}

func currentClaudeInputs(t *testing.T) map[string]profile.EvaluationInput {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("testdata", "claude-code-2.1.251.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]profile.EvaluationInput
	if err := json.Unmarshal(encoded, &inputs); err != nil {
		t.Fatal(err)
	}
	return inputs
}

func fixtureAttributes(t *testing.T, input profile.EvaluationInput) map[string]any {
	t.Helper()
	record, ok := input.Event["record"].(map[string]any)
	if !ok {
		t.Fatalf("record = %#v", input.Event)
	}
	attributes, ok := record["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("attributes = %#v", record)
	}
	return attributes
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

func observedVSCodeCopilotInputs(t *testing.T) map[string]profile.EvaluationInput {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("testdata", "vscode-copilot-chat-0.63.0.json"))
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

func guardAnalyticRows(prior any) map[string]any {
	return map[string]any{
		"prior": prior, "tool_coverage_prior": nil,
		"target_coverage_prior": nil, "progress_coverage_prior": nil,
	}
}
