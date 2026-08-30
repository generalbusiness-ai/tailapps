package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/generalbusiness-ai/tailapps/internal/control"
	"github.com/generalbusiness-ai/tailapps/internal/engine"
)

const probeApplicationSQL = `CREATE EVENT otel_event (
  key TEXT NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE totals (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE NORMALIZER normalize ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;

CREATE FOLD accumulate ON otel_event
READ prior OPTIONAL ONE AS
  SELECT key, total FROM totals WHERE key = :event.key
USING 'folds/accumulate.jsonata'
WRITES totals;

CREATE EXPORT totals AS
  SELECT key, total FROM totals;
`

const probeNormalizeJSONata = `{
  "decision": "effective",
  "facts": [],
  "events": {"otel_event": [{"key": "k", "source_position": meta.position}]},
  "tables": {}
}
`

const probeAccumulateJSONata = `{
  "decision": "effective",
  "facts": [],
  "tables": {"totals": {"upsert": [{"key": event.key, "total": 1}]}}
}
`

// TestRealToolResultsMatchDeclaredOutputSchemas drives every tool against a
// real engine and validates each live structuredContent against the tool's
// declared outputSchema, empty results included — the contract a validating
// client enforces. This is the test the first stage-1 review found missing:
// a fresh projection's query returned rows:null against a schema declaring a
// required array.
func TestRealToolResultsMatchDeclaredOutputSchemas(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "engine")
	resident, err := engine.Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer resident.Close()

	socketDir, err := os.MkdirTemp("/tmp", "tailapp-conf-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	listener, err := net.Listen("unix", filepath.Join(socketDir, "control.sock"))
	if err != nil {
		t.Fatal(err)
	}
	controlServer := &http.Server{Handler: &control.Server{Engine: resident}}
	t.Cleanup(func() { _ = controlServer.Close() })
	go func() { _ = controlServer.Serve(listener) }()
	server := &Server{Client: control.NewClient(listener.Addr().String()), Home: home}

	session := startWire(t, server)
	session.initialize("2025-06-18")
	validated := make(map[string]bool)
	call := func(name, arguments string) map[string]any {
		t.Helper()
		var decodedArguments map[string]any
		if err := json.Unmarshal([]byte(arguments), &decodedArguments); err != nil {
			t.Fatal(err)
		}
		reply := session.call("tools/call", map[string]any{"name": name, "arguments": decodedArguments})
		result, _ := reply["result"].(map[string]any)
		if result == nil {
			t.Fatalf("%s protocol error: %#v", name, reply["error"])
		}
		if result["isError"] == true {
			t.Fatalf("%s returned an engine error: %#v", name, result["content"])
		}
		structured, ok := result["structuredContent"].(map[string]any)
		if !ok {
			t.Fatalf("%s returned no object structuredContent: %#v", name, result)
		}
		validateAgainstSchema(t, name, outputSchemas[name], structured)
		validated[name] = true
		return structured
	}
	encode := func(value any) string {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}

	// Empty engine first: the wrapped list and both no-argument reads.
	empty := call("tailapps_list", `{}`)
	if apps, ok := empty["apps"].([]any); !ok || len(apps) != 0 {
		t.Fatalf("fresh engine apps = %#v, want empty array", empty["apps"])
	}
	call("tailapp_status", `{}`)
	// The described fresh-resident shape: cumulative counters exist but the
	// never-used resident shows an empty tailapps map and zero intake
	// activity, while uptime is already nonzero.
	metrics := call("tailapp_metrics", `{}`)
	if tailapps, ok := metrics["tailapps"].(map[string]any); !ok || len(tailapps) != 0 {
		t.Fatalf("fresh resident tailapps = %#v, want empty map", metrics["tailapps"])
	}
	intake, _ := metrics["intake"].(map[string]any)
	if unrouted, ok := intake["unrouted_records_total"].(float64); !ok || unrouted != 0 {
		t.Fatalf("fresh resident intake activity = %#v, want zero", intake["unrouted_records_total"])
	}
	records, ok := intake["records_total"].(map[string]any)
	if !ok {
		t.Fatalf("fresh resident records_total = %#v, want the preseeded signal object", intake["records_total"])
	}
	for _, signal := range []string{"log", "span", "metric", "unknown"} {
		count, present := records[signal]
		if !present || count != float64(0) {
			t.Fatalf("fresh resident records_total[%s] = %#v (present %v), want preseeded zero", signal, count, present)
		}
	}
	if len(records) != 4 {
		t.Fatalf("fresh resident records_total keys = %#v, want exactly the four signals", records)
	}
	if uptime, ok := metrics["uptime_seconds"].(float64); !ok || uptime <= 0 {
		t.Fatalf("fresh resident uptime = %#v; cumulative gauges are nonzero even before use", metrics["uptime_seconds"])
	}
	// The runtime gauges the description claims nonzero.
	runtime, ok := metrics["runtime"].(map[string]any)
	if !ok {
		t.Fatalf("fresh resident runtime = %#v, want the gauge object", metrics["runtime"])
	}
	for _, gauge := range []string{"goroutines", "total_memory_bytes"} {
		if value, ok := runtime[gauge].(float64); !ok || value <= 0 {
			t.Fatalf("fresh resident runtime.%s = %#v, want nonzero", gauge, runtime[gauge])
		}
	}

	// Full draft lifecycle on a probe app.
	call("tailapp_create", `{"name":"probe","idempotency_key":"conf-create-1"}`)
	sources := map[string]string{
		"application.sql":          base64.StdEncoding.EncodeToString([]byte(probeApplicationSQL)),
		"folds/normalize.jsonata":  base64.StdEncoding.EncodeToString([]byte(probeNormalizeJSONata)),
		"folds/accumulate.jsonata": base64.StdEncoding.EncodeToString([]byte(probeAccumulateJSONata)),
	}
	revision := ""
	for index, path := range []string{"application.sql", "folds/normalize.jsonata", "folds/accumulate.jsonata"} {
		if revision == "" {
			got := call("tailapp_get", `{"name":"probe"}`)
			app, _ := got["app"].(map[string]any)
			revision, _ = app["draft_revision"].(string)
		}
		put := call("tailapp_put_element", fmt.Sprintf(`{"name":"probe","path":%s,"content":%s,"expected_revision":%s,"idempotency_key":"conf-put-%d"}`,
			encode(path), encode(sources[path]), encode(revision), index))
		revision, _ = put["draft_revision"].(string)
	}
	call("tailapp_validate", fmt.Sprintf(`{"name":"probe","expected_revision":%s}`, encode(revision)))
	call("tailapp_activate", fmt.Sprintf(`{"name":"probe","expected_revision":%s,"mode":"reset","acknowledge_reset":true,"idempotency_key":"conf-activate-1"}`, encode(revision)))

	// The regression case: an activated app whose projection has no rows.
	query := call("tailapp_query", `{"name":"probe","sql":"SELECT key, total FROM totals ORDER BY key"}`)
	if rows, ok := query["rows"].([]any); !ok || len(rows) != 0 {
		t.Fatalf("empty projection rows = %#v, want empty array, never null", query["rows"])
	}
	ineffective := call("tailapp_ineffective", `{"name":"probe"}`)
	if records, ok := ineffective["records"].([]any); !ok || len(records) != 0 {
		t.Fatalf("idle records = %#v, want empty array, never null", ineffective["records"])
	}
	call("tailapp_schema", `{"name":"probe"}`)

	// One bundle install, one delete-element on a fresh draft, one delete.
	call("tailapp_install", `{"name":"cost","bundle":"session-cost","idempotency_key":"conf-install-1"}`)
	removed := call("tailapp_delete_element", fmt.Sprintf(`{"name":"probe","path":"folds/accumulate.jsonata","expected_revision":%s,"idempotency_key":"conf-remove-1"}`, encode(revision)))
	if removed["draft_revision"] == revision {
		t.Fatal("delete_element did not advance the draft revision")
	}
	call("tailapp_delete", `{"name":"probe","idempotency_key":"conf-delete-1"}`)

	for _, item := range tools() {
		if !validated[item.Name] {
			t.Fatalf("tool %s was never validated against a real result", item.Name)
		}
	}
}

// validateAgainstSchema checks a live result against the declared contract:
// every required property is present and non-null (unless declared
// nullable), and every present value matches its declared type. It covers
// exactly the schema subset tools() declares.
func validateAgainstSchema(t *testing.T, toolName string, schema, value map[string]any) {
	t.Helper()
	if schema == nil {
		t.Fatalf("%s declares no outputSchema", toolName)
	}
	properties, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]string)
	if required == nil {
		if anyRequired, ok := schema["required"].([]any); ok {
			for _, name := range anyRequired {
				required = append(required, name.(string))
			}
		}
	}
	for _, name := range required {
		present, exists := value[name]
		if !exists {
			t.Fatalf("%s result misses required %q: %#v", toolName, name, value)
		}
		if present == nil && !nullable(properties[name]) {
			t.Fatalf("%s required %q is null against a non-nullable schema", toolName, name)
		}
	}
	for name, declared := range properties {
		property, _ := declared.(map[string]any)
		present, exists := value[name]
		if !exists || present == nil || property == nil {
			continue
		}
		if err := matchesType(property, present); err != nil {
			t.Fatalf("%s property %q: %v (value %#v)", toolName, name, err, present)
		}
	}
}

func nullable(declared any) bool {
	property, _ := declared.(map[string]any)
	if property == nil {
		return false
	}
	if kinds, ok := property["type"].([]string); ok {
		for _, kind := range kinds {
			if kind == "null" {
				return true
			}
		}
	}
	return false
}

func matchesType(property map[string]any, value any) error {
	kind, _ := property["type"].(string)
	if kind == "" {
		return nil
	}
	switch kind {
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("want object, got %T", value)
		}
		if items, ok := property["properties"].(map[string]any); ok {
			_ = items
		}
	case "array":
		items, _ := value.([]any)
		if value != nil {
			if _, ok := value.([]any); !ok {
				return fmt.Errorf("want array, got %T", value)
			}
		}
		if declared, ok := property["items"].(map[string]any); ok {
			for _, element := range items {
				if err := matchesType(declared, element); err != nil {
					return err
				}
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("want string, got %T", value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("want boolean, got %T", value)
		}
	case "integer", "number":
		switch value.(type) {
		case float64, json.Number:
		default:
			return fmt.Errorf("want %s, got %T", kind, value)
		}
	}
	if kind == "object" {
		object, _ := value.(map[string]any)
		nestedRequired, _ := property["required"].([]string)
		for _, name := range nestedRequired {
			if _, exists := object[name]; !exists {
				return fmt.Errorf("nested required %q missing", name)
			}
		}
	}
	return nil
}
