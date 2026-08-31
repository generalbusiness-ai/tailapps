package tailapps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/generalbusiness-ai/tailapps/internal/profile"
)

const (
	urlPipelineNormalizer = "normalize_url_pipeline"
	urlPipelineFold       = "update_url_pipeline"
)

var urlPipelineReadNames = []string{
	"observation_prior",
	"exclusion_prior",
	"verdict_prior",
	"count_prior",
	"builtin_ip_literal_prior",
	"builtin_localhost_prior",
	"builtin_single_label_prior",
	"builtin_local_prior",
	"builtin_internal_prior",
	"builtin_home_arpa_prior",
	"builtin_test_prior",
}

func TestURLReputationExportsAndSingleWriterTopology(t *testing.T) {
	compiled, err := Load("url-reputation")
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Event.Name != "otel_event" || compiled.Normalizer.Name != urlPipelineNormalizer || compiled.Normalizer.Emits != "otel_event" {
		t.Fatalf("private topology = event %#v normalizer %#v", compiled.Event, compiled.Normalizer)
	}
	if len(compiled.Folds) != 1 || compiled.Folds[0].Name != urlPipelineFold {
		t.Fatalf("folds = %#v", compiled.Folds)
	}
	wantTables := []string{"url_exclusions", "url_observations", "url_pipeline_counts", "url_verdicts"}
	gotWrites := append([]string(nil), compiled.Folds[0].Writes...)
	sort.Strings(gotWrites)
	if !reflect.DeepEqual(gotWrites, wantTables) {
		t.Fatalf("fold writes = %#v, want %#v", gotWrites, wantTables)
	}
	for _, name := range wantTables {
		table, ok := compiled.Tables[name]
		if !ok || table.Writer != urlPipelineFold {
			t.Fatalf("table %q = %#v", name, table)
		}
		if exported, ok := compiled.Exports[name]; !ok || exported.Name != name {
			t.Fatalf("export %q = %#v", name, exported)
		}
	}
}

func TestURLReputationNormalizerFixtures(t *testing.T) {
	compiled := loadURLReputation(t)
	fixtures := urlReputationFixtures(t)

	accepted := map[string]string{
		"observed":                 "observation",
		"exclusion_enabled":        "exclusion",
		"exclusion_disabled":       "exclusion",
		"reputation_clean":         "reputation",
		"reputation_suspected":     "reputation",
		"reputation_error_expired": "reputation",
	}
	for name, family := range accepted {
		t.Run(name, func(t *testing.T) {
			result := normalizeURLFixture(t, compiled, fixtures[name])
			if result.Decision != "effective" || len(result.Events["otel_event"]) != 1 {
				t.Fatalf("result = %#v", result)
			}
			if got := result.Events["otel_event"][0]["event_family"]; got != family {
				t.Fatalf("event family = %#v, want %q", got, family)
			}
		})
	}

	observed := normalizeURLFixture(t, compiled, fixtures["observed"]).Events["otel_event"][0]
	if observed["observed_full"] != "https://Example.COM/path?q=local" || observed["host"] != "example.com" {
		t.Fatalf("observation identity = %#v", observed)
	}
	if observed["harness"] != "codex" || observed["session_id"] != "session-abcdef123456" ||
		observed["session_id_prefix"] != "session-abcd" || observed["tool_name"] != "web_fetch" ||
		observed["project"] != "/work/tailapps" || fmt.Sprint(observed["day_utc"]) != "20696" {
		t.Fatalf("observation dimensions = %#v", observed)
	}
	exclusion := normalizeURLFixture(t, compiled, fixtures["exclusion_enabled"]).Events["otel_event"][0]
	if exclusion["exclusion_pattern"] != ".example.com" || fmt.Sprint(exclusion["exclusion_enabled"]) != "1" {
		t.Fatalf("normalized exclusion = %#v", exclusion)
	}
	suspected := normalizeURLFixture(t, compiled, fixtures["reputation_suspected"]).Events["otel_event"][0]
	if suspected["verdict"] != "suspected" || suspected["threat_types"] != `["MALWARE"]` || suspected["provider"] != "web-risk" {
		t.Fatalf("suspected verdict = %#v", suspected)
	}
	errorEvent := normalizeURLFixture(t, compiled, fixtures["reputation_error_expired"]).Events["otel_event"][0]
	if errorEvent["verdict"] != "error" || errorEvent["error"] != "provider timeout" {
		t.Fatalf("error verdict = %#v", errorEvent)
	}

	for _, name := range []string{"observed_host_mismatch", "missing_required"} {
		t.Run(name, func(t *testing.T) {
			assertURLNormalizationRejected(t, compiled, fixtures[name])
		})
	}
}

func TestURLReputationNormalizerAuthorityBoundaries(t *testing.T) {
	compiled := loadURLReputation(t)
	base := urlReputationFixtures(t)["observed"]

	accepted := map[string]string{
		"authority-end": "https://example.com",
		"slash":         "https://example.com/path",
		"port":          "https://example.com:8443/path",
		"query":         "https://example.com?query=local",
		"fragment":      "http://example.com#local",
		"mixed-case":    "HTTPS://EXAMPLE.COM/path",
	}
	for name, rawURL := range accepted {
		t.Run("accept-"+name, func(t *testing.T) {
			input := cloneURLInput(t, base)
			setURLAttributes(input, map[string]any{
				"tailapp.url.observed_full": rawURL,
				"tailapp.url.host":          "Example.COM",
			})
			result := normalizeURLFixture(t, compiled, input)
			if result.Decision != "effective" || len(result.Events["otel_event"]) != 1 {
				t.Fatalf("%s rejected: %#v", rawURL, result)
			}
		})
	}

	rejected := map[string]string{
		"wrong-suffix":   "https://example.com.evil/path",
		"userinfo":       "https://example.com@evil.test/path",
		"no-boundary":    "https://example.comx/path",
		"wrong-scheme":   "ftp://example.com/path",
		"relative-value": "/example.com/path",
	}
	for name, rawURL := range rejected {
		t.Run("reject-"+name, func(t *testing.T) {
			input := cloneURLInput(t, base)
			setURLAttributes(input, map[string]any{
				"tailapp.url.observed_full": rawURL,
				"tailapp.url.host":          "example.com",
			})
			assertURLNormalizationRejected(t, compiled, input)
		})
	}

	for _, host := range []string{"example.com/path", "example.com?query", "example.com#fragment", "user@example.com"} {
		t.Run("reject-host-shape-"+host, func(t *testing.T) {
			input := cloneURLInput(t, base)
			setURLAttributes(input, map[string]any{"tailapp.url.host": host})
			assertURLNormalizationRejected(t, compiled, input)
		})
	}
}

func TestURLReputationNormalizerRejectsInvalidFamiliesAndUsesObservedTime(t *testing.T) {
	compiled := loadURLReputation(t)
	fixtures := urlReputationFixtures(t)

	fallback := cloneURLInput(t, fixtures["observed"])
	fallback.Event["time_unix_nano"] = nil
	fallback.Event["observed_unix_nano"] = "1788192000000000099"
	result := normalizeURLFixture(t, compiled, fallback)
	if result.Decision != "effective" || result.Events["otel_event"][0]["event_time_unix_nano"] != "1788192000000000099" {
		t.Fatalf("observed-time fallback = %#v", result)
	}

	missingTime := cloneURLInput(t, fallback)
	missingTime.Event["observed_unix_nano"] = nil
	assertURLNormalizationRejected(t, compiled, missingTime)

	nonLog := cloneURLInput(t, fixtures["observed"])
	nonLog.Event["signal"] = "span"
	assertURLNormalizationRejected(t, compiled, nonLog)

	badKind := cloneURLInput(t, fixtures["exclusion_enabled"])
	setURLAttributes(badKind, map[string]any{"tailapp.url.exclusion.kind": "regex"})
	assertURLNormalizationRejected(t, compiled, badKind)

	badVerdict := cloneURLInput(t, fixtures["reputation_clean"])
	setURLAttributes(badVerdict, map[string]any{"tailapp.url.verdict": "unknown"})
	assertURLNormalizationRejected(t, compiled, badVerdict)

	unrecognized := cloneURLInput(t, fixtures["observed"])
	unrecognized.Event["name"] = "tailapp.url.unknown"
	setURLAttributes(unrecognized, map[string]any{"event.name": "tailapp.url.unknown"})
	assertURLNormalizationRejected(t, compiled, unrecognized)
}

func TestURLReputationFoldWritesAllTables(t *testing.T) {
	compiled := loadURLReputation(t)
	fixtures := urlReputationFixtures(t)
	tests := []struct {
		fixture string
		table   string
	}{
		{"observed", "url_observations"},
		{"exclusion_enabled", "url_exclusions"},
		{"reputation_clean", "url_verdicts"},
	}
	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			event := normalizeURLFixture(t, compiled, fixtures[tc.fixture]).Events["otel_event"][0]
			folded := foldURLEvent(t, compiled, event, emptyURLPipelineRows())
			if folded.Decision != "effective" || len(folded.Tables[tc.table].Upsert) == 0 {
				t.Fatalf("folded = %#v", folded)
			}
			if len(folded.Tables["url_pipeline_counts"].Upsert) != 1 {
				t.Fatalf("counts = %#v", folded.Tables["url_pipeline_counts"])
			}
			if len(folded.Tables["url_exclusions"].Upsert) < 7 {
				t.Fatalf("built-ins = %#v", folded.Tables["url_exclusions"].Upsert)
			}
		})
	}
}

func TestURLReputationObservationAndCountsAccumulate(t *testing.T) {
	compiled := loadURLReputation(t)
	input := urlReputationFixtures(t)["observed"]
	firstEvent := normalizeURLFixture(t, compiled, input).Events["otel_event"][0]
	first := foldURLEvent(t, compiled, firstEvent, emptyURLPipelineRows())
	observation := first.Tables["url_observations"].Upsert[0]
	count := first.Tables["url_pipeline_counts"].Upsert[0]

	secondInput := cloneURLInput(t, input)
	secondInput.Meta["position"] = 20
	secondInput.Event["time_unix_nano"] = "1788192000000000200"
	setURLAttributes(secondInput, map[string]any{
		"conversation.id": "session-new-999999",
		"tool_name":       "browser_fetch",
		"project":         "/work/next",
	})
	secondEvent := normalizeURLFixture(t, compiled, secondInput).Events["otel_event"][0]
	rows := emptyURLPipelineRows()
	rows["observation_prior"] = observation
	rows["count_prior"] = count
	installBuiltinPriors(rows, first.Tables["url_exclusions"].Upsert)
	second := foldURLEvent(t, compiled, secondEvent, rows)
	row := second.Tables["url_observations"].Upsert[0]
	if fmt.Sprint(row["observation_count"]) != "2" ||
		row["first_observed_unix_nano"] != firstEvent["event_time_unix_nano"] ||
		row["last_observed_unix_nano"] != "1788192000000000200" ||
		row["latest_session_id"] != "session-new-999999" || row["latest_tool_name"] != "browser_fetch" ||
		row["latest_project"] != "/work/next" || fmt.Sprint(row["last_source_position"]) != "20" {
		t.Fatalf("observation = %#v", row)
	}
	count = second.Tables["url_pipeline_counts"].Upsert[0]
	if fmt.Sprint(count["record_count"]) != "2" || fmt.Sprint(count["error_count"]) != "0" ||
		count["first_event_time_unix_nano"] != firstEvent["event_time_unix_nano"] ||
		count["last_event_time_unix_nano"] != "1788192000000000200" {
		t.Fatalf("count = %#v", count)
	}
}

func TestURLReputationBuiltinsSeedOnceAndPreserveOperatorUpdate(t *testing.T) {
	compiled := loadURLReputation(t)
	fixtures := urlReputationFixtures(t)
	observedEvent := normalizeURLFixture(t, compiled, fixtures["observed"]).Events["otel_event"][0]
	first := foldURLEvent(t, compiled, observedEvent, emptyURLPipelineRows())
	builtins := first.Tables["url_exclusions"].Upsert
	if len(builtins) != 7 {
		t.Fatalf("built-in rows = %#v", builtins)
	}
	byID := indexRows(builtins, "exclusion_id")
	for _, id := range []string{
		"builtin:ip-literal", "builtin:localhost", "builtin:single-label", "builtin:local",
		"builtin:internal", "builtin:home-arpa", "builtin:test",
	} {
		if row := byID[id]; row == nil || fmt.Sprint(row["enabled"]) != "1" {
			t.Fatalf("built-in %q = %#v", id, row)
		}
	}

	disableInput := cloneURLInput(t, fixtures["exclusion_disabled"])
	setURLAttributes(disableInput, map[string]any{
		"tailapp.url.exclusion.id":      "builtin:localhost",
		"tailapp.url.exclusion.kind":    "host-exact",
		"tailapp.url.exclusion.pattern": "localhost",
	})
	disableEvent := normalizeURLFixture(t, compiled, disableInput).Events["otel_event"][0]
	disableRows := emptyURLPipelineRows()
	installBuiltinPriors(disableRows, builtins)
	disableRows["exclusion_prior"] = byID["builtin:localhost"]
	disabled := foldURLEvent(t, compiled, disableEvent, disableRows)
	updates := disabled.Tables["url_exclusions"].Upsert
	if len(updates) != 1 || updates[0]["exclusion_id"] != "builtin:localhost" || fmt.Sprint(updates[0]["enabled"]) != "0" {
		t.Fatalf("operator update = %#v", updates)
	}

	laterRows := emptyURLPipelineRows()
	installBuiltinPriors(laterRows, builtins)
	laterRows["builtin_localhost_prior"] = updates[0]
	later := foldURLEvent(t, compiled, observedEvent, laterRows)
	if rows := later.Tables["url_exclusions"].Upsert; len(rows) != 0 {
		t.Fatalf("recognized event rewrote built-ins: %#v", rows)
	}
}

func TestURLReputationVerdictsRemainProviderSpecificAndCountErrors(t *testing.T) {
	compiled := loadURLReputation(t)
	fixtures := urlReputationFixtures(t)
	cleanEvent := normalizeURLFixture(t, compiled, fixtures["reputation_clean"]).Events["otel_event"][0]
	clean := foldURLEvent(t, compiled, cleanEvent, emptyURLPipelineRows())
	cleanRow := clean.Tables["url_verdicts"].Upsert[0]

	otherInput := cloneURLInput(t, fixtures["reputation_suspected"])
	setURLAttributes(otherInput, map[string]any{
		"tailapp.url.observed_full": cleanEvent["observed_full"],
		"tailapp.url.checked_full":  cleanEvent["checked_full"],
	})
	otherEvent := normalizeURLFixture(t, compiled, otherInput).Events["otel_event"][0]
	other := foldURLEvent(t, compiled, otherEvent, emptyURLPipelineRows())
	otherRow := other.Tables["url_verdicts"].Upsert[0]
	if cleanRow["observed_full"] != otherRow["observed_full"] || cleanRow["provider"] == otherRow["provider"] {
		t.Fatalf("provider keys = clean %#v other %#v", cleanRow, otherRow)
	}

	errorInput := cloneURLInput(t, fixtures["reputation_error_expired"])
	setURLAttributes(errorInput, map[string]any{
		"tailapp.url.observed_full": cleanEvent["observed_full"],
		"tailapp.url.checked_full":  cleanEvent["checked_full"],
		"tailapp.url.provider":      cleanEvent["provider"],
	})
	errorEvent := normalizeURLFixture(t, compiled, errorInput).Events["otel_event"][0]
	rows := emptyURLPipelineRows()
	rows["verdict_prior"] = cleanRow
	rows["count_prior"] = clean.Tables["url_pipeline_counts"].Upsert[0]
	updated := foldURLEvent(t, compiled, errorEvent, rows)
	updatedRow := updated.Tables["url_verdicts"].Upsert[0]
	if updatedRow["provider"] != cleanRow["provider"] || updatedRow["verdict"] != "error" || updatedRow["error"] != "provider timeout" {
		t.Fatalf("updated verdict = %#v", updatedRow)
	}
	count := updated.Tables["url_pipeline_counts"].Upsert[0]
	if fmt.Sprint(count["record_count"]) != "2" || fmt.Sprint(count["error_count"]) != "1" {
		t.Fatalf("reputation count = %#v", count)
	}
}

func BenchmarkURLReputationNormalizer(b *testing.B) {
	compiled, err := Load("url-reputation")
	if err != nil {
		b.Fatal(err)
	}
	input := urlReputationBenchmarkFixture(b, "observed")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := compiled.Evaluate(urlPipelineNormalizer, input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkURLReputationPipelineFold(b *testing.B) {
	compiled, err := Load("url-reputation")
	if err != nil {
		b.Fatal(err)
	}
	input := urlReputationBenchmarkFixture(b, "observed")
	normalized, err := compiled.Evaluate(urlPipelineNormalizer, input)
	if err != nil {
		b.Fatal(err)
	}
	foldInput := profile.EvaluationInput{
		Meta:  map[string]any{"position": 1, "event_id": "local:1:0", "event_type": "otel_event"},
		Event: normalized.Events["otel_event"][0], Rows: emptyURLPipelineRows(),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := compiled.Evaluate(urlPipelineFold, foldInput); err != nil {
			b.Fatal(err)
		}
	}
}

func loadURLReputation(t testing.TB) *profile.Profile {
	t.Helper()
	compiled, err := Load("url-reputation")
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func urlReputationFixtures(t testing.TB) map[string]profile.EvaluationInput {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("testdata", "url-reputation-stage2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]profile.EvaluationInput
	if err := json.Unmarshal(encoded, &fixtures); err != nil {
		t.Fatal(err)
	}
	return fixtures
}

func urlReputationBenchmarkFixture(b *testing.B, name string) profile.EvaluationInput {
	b.Helper()
	fixture, ok := urlReputationFixtures(b)[name]
	if !ok {
		b.Fatalf("fixture %q is missing", name)
	}
	return fixture
}

func normalizeURLFixture(t testing.TB, compiled *profile.Profile, input profile.EvaluationInput) profile.EvaluationResult {
	t.Helper()
	result, err := compiled.Evaluate(urlPipelineNormalizer, input)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func assertURLNormalizationRejected(t testing.TB, compiled *profile.Profile, input profile.EvaluationInput) {
	t.Helper()
	result := normalizeURLFixture(t, compiled, input)
	if result.Decision != "ineffective" || len(result.Events["otel_event"]) != 0 {
		t.Fatalf("normalization was not rejected: %#v", result)
	}
}

func foldURLEvent(t testing.TB, compiled *profile.Profile, event map[string]any, rows map[string]any) profile.EvaluationResult {
	t.Helper()
	result, err := compiled.Evaluate(urlPipelineFold, profile.EvaluationInput{
		Meta: map[string]any{
			"position": event["source_position"], "event_id": "local:fold", "event_type": "otel_event", "emission_ordinal": 0,
		},
		Event: event,
		Rows:  rows,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func emptyURLPipelineRows() map[string]any {
	rows := make(map[string]any, len(urlPipelineReadNames))
	for _, name := range urlPipelineReadNames {
		rows[name] = nil
	}
	return rows
}

func installBuiltinPriors(rows map[string]any, builtins []map[string]any) {
	byID := indexRows(builtins, "exclusion_id")
	for read, id := range map[string]string{
		"builtin_ip_literal_prior":   "builtin:ip-literal",
		"builtin_localhost_prior":    "builtin:localhost",
		"builtin_single_label_prior": "builtin:single-label",
		"builtin_local_prior":        "builtin:local",
		"builtin_internal_prior":     "builtin:internal",
		"builtin_home_arpa_prior":    "builtin:home-arpa",
		"builtin_test_prior":         "builtin:test",
	} {
		rows[read] = byID[id]
	}
}

func indexRows(rows []map[string]any, key string) map[string]map[string]any {
	result := make(map[string]map[string]any, len(rows))
	for _, row := range rows {
		result[fmt.Sprint(row[key])] = row
	}
	return result
}

func cloneURLInput(t testing.TB, input profile.EvaluationInput) profile.EvaluationInput {
	t.Helper()
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var clone profile.EvaluationInput
	if err := json.Unmarshal(encoded, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func setURLAttributes(input profile.EvaluationInput, values map[string]any) {
	record := input.Event["record"].(map[string]any)
	attributes := record["attributes"].(map[string]any)
	for key, value := range values {
		attributes[key] = value
	}
}
