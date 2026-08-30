package mcp

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/generalbusiness-ai/tailapps/internal/control"
)

// newStubServer serves one control operation from a stub engine over a unix
// socket and returns an MCP server pointed at it.
func newStubServer(t *testing.T, wantOperation, resultJSON string) *Server {
	t.Helper()
	tempDir, err := os.MkdirTemp("/tmp", "tailapp-mcp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	listener, err := net.Listen("unix", filepath.Join(tempDir, "control.sock"))
	if err != nil {
		t.Fatal(err)
	}
	controlServer := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var envelope control.Request
		if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
			t.Errorf("decode control request: %v", err)
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		if envelope.Operation != wantOperation {
			t.Errorf("operation = %q, want %q", envelope.Operation, wantOperation)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"result":` + resultJSON + `}`))
	})}
	t.Cleanup(func() { _ = controlServer.Close() })
	go func() {
		if err := controlServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Errorf("serve control socket: %v", err)
		}
	}()
	return &Server{Client: control.NewClient(listener.Addr().String()), Home: tempDir}
}

func TestEveryToolDeclaresOutputSchema(t *testing.T) {
	for _, item := range tools() {
		if item.OutputSchema == nil {
			t.Fatalf("%s declares no outputSchema", item.Name)
		}
		if item.OutputSchema["type"] != "object" {
			t.Fatalf("%s outputSchema root must be an object: %#v", item.Name, item.OutputSchema)
		}
		encoded, err := json.Marshal(item.OutputSchema)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), `"$schema"`) {
			t.Fatalf("%s outputSchema carries a $schema dialect marker, which disables the catalog on Ajv-validating clients: %s", item.Name, encoded)
		}
	}
}

func TestEngineErrorTextRedactsHomeAndNamesTool(t *testing.T) {
	server := &Server{Home: "/tmp/example-home"}
	raw := `Post "http://unix/v1/control": dial unix /tmp/example-home/engine.sock: connect: no such file or directory`
	text := server.engineErrorText("tailapp_query", raw)
	if !strings.HasPrefix(text, "tailapp_query failed: ") {
		t.Fatalf("error text must name the tool: %q", text)
	}
	if strings.Contains(text, "/tmp/example-home") {
		t.Fatalf("error text leaks the home path: %q", text)
	}
	if !strings.Contains(text, "$TAILAPP_HOME") || !strings.Contains(text, "tailapp serve") {
		t.Fatalf("error text must substitute the home and suggest the next step: %q", text)
	}
}

func TestEveryToolCarriesOrientation(t *testing.T) {
	for _, item := range tools() {
		if item.Title == "" {
			t.Fatalf("%s has no title", item.Name)
		}
		if len(item.Description) >= 2048 {
			t.Fatalf("%s description is %d bytes; the 2KB harness truncation cap requires fewer", item.Name, len(item.Description))
		}
		if !strings.HasSuffix(item.Description, "docs: tailapp://docs/tools/"+item.Name) {
			t.Fatalf("%s description must end with its stable docs URI: %q", item.Name, item.Description)
		}
	}
}

// TestEveryDescriptionStatesItsSemanticClauses pins the three clauses the
// design requires of every description - the result contract, the
// empty-result (or failure-absence) shape, and the workflow position - as
// exact per-tool fragments, so removing or weakening any clause fails.
func TestEveryDescriptionStatesItsSemanticClauses(t *testing.T) {
	clauses := map[string]struct{ contract, empty, workflow string }{
		"tailapps_list":          {`Returns {"apps": [...]}`, `an empty engine returns {"apps": []}, never null`, "Start here"},
		"tailapp_get":            {"Returns {app, sources}", "empty draft returns {} sources", "draft edits are not live until tailapp_activate"},
		"tailapp_create":         {"Returns the app", "a fresh empty draft has only name and draft_revision", "Follow with tailapp_put_element"},
		"tailapp_install":        {"Returns {app, profile, frontier}", "no partial success - failure installs nothing", "one-step alternative to the create/put/validate/activate sequence"},
		"tailapp_delete":         {"Returns {deleted: true}", "an unknown name is an error, not an empty result", "end of a Tailapp's lifecycle"},
		"tailapp_put_element":    {"Returns the app with its new draft_revision", "no other change", "between tailapp_create and tailapp_validate"},
		"tailapp_delete_element": {"Returns the app with its new draft_revision", "removing the last element leaves a valid empty draft", "between create and validate"},
		"tailapp_validate":       {"Returns the full compiled profile", "never a partial profile", "before tailapp_activate"},
		"tailapp_activate":       {"Returns the projection {frontier}", "complete true and no gap fields", "last step of the draft loop"},
		"tailapp_status":         {"Returns {profile, ingestion_ready, inbox, apps, unavailable}", "with no Tailapps installed, apps is {}", "Start here when telemetry seems missing"},
		"tailapp_metrics":        {"Returns a flat object of counters and gauges", "a fresh never-used resident shows an empty tailapps map and zero intake activity while uptime and runtime gauges are already nonzero", "operational triage"},
		"tailapp_ineffective":    {"Returns {tailapp, revision, capacity, ineffective_records, records}", "no rejections returns records: []", "after tailapp_status shows intake"},
		"tailapp_schema":         {"Returns the compiled profile object", "stable between activations", "before writing SQL for tailapp_query"},
		"tailapp_query":          {"Returns {columns, rows, complete, truncated}", "an empty projection returns rows: []", "after tailapps_list"},
	}
	seen := 0
	for _, item := range tools() {
		expected, known := clauses[item.Name]
		if !known {
			t.Fatalf("tool %s has no pinned semantic clauses", item.Name)
		}
		seen++
		for clause, fragment := range map[string]string{"result contract": expected.contract, "empty-result shape": expected.empty, "workflow position": expected.workflow} {
			if !strings.Contains(item.Description, fragment) {
				t.Fatalf("%s description lost its %s clause (%q): %q", item.Name, clause, fragment, item.Description)
			}
		}
	}
	if seen != len(clauses) {
		t.Fatalf("pinned %d tools, saw %d", len(clauses), seen)
	}
}

func TestToolAnnotationsMatchSafetyContract(t *testing.T) {
	readOnly := map[string]bool{
		"tailapps_list": true, "tailapp_get": true, "tailapp_validate": true,
		"tailapp_status": true, "tailapp_metrics": true, "tailapp_ineffective": true,
		"tailapp_schema": true, "tailapp_query": true,
	}
	destructive := map[string]bool{"tailapp_delete": true, "tailapp_activate": true}
	idempotent := map[string]bool{
		"tailapp_create": true, "tailapp_install": true, "tailapp_delete": true,
		"tailapp_put_element": true, "tailapp_delete_element": true, "tailapp_activate": true,
	}
	seen := 0
	for _, item := range tools() {
		seen++
		hints := item.Annotations
		if hints.OpenWorldHint {
			t.Fatalf("%s claims an open world; the server talks only to the local engine", item.Name)
		}
		if hints.ReadOnlyHint != readOnly[item.Name] {
			t.Fatalf("%s readOnlyHint = %v; a wrong value is auto-approval policy on some harnesses", item.Name, hints.ReadOnlyHint)
		}
		if hints.DestructiveHint != destructive[item.Name] {
			t.Fatalf("%s destructiveHint = %v", item.Name, hints.DestructiveHint)
		}
		if hints.IdempotentHint != idempotent[item.Name] {
			t.Fatalf("%s idempotentHint = %v; every mutation tool replays through the idempotency ledger", item.Name, hints.IdempotentHint)
		}
		if hints.ReadOnlyHint && (hints.DestructiveHint || hints.IdempotentHint) {
			t.Fatalf("%s mixes read-only with mutation hints", item.Name)
		}
	}
	if seen != 14 {
		t.Fatalf("tools = %d, want 14", seen)
	}
}

func TestMutationToolsRequireIdempotencyKeys(t *testing.T) {
	mutations := map[string]bool{
		"tailapp_create":         true,
		"tailapp_install":        true,
		"tailapp_delete":         true,
		"tailapp_put_element":    true,
		"tailapp_delete_element": true,
		"tailapp_activate":       true,
	}
	for _, item := range tools() {
		if !mutations[item.Name] {
			continue
		}
		required, ok := item.InputSchema["required"].([]string)
		if !ok || !contains(required, "idempotency_key") {
			t.Fatalf("%s does not require idempotency_key: %#v", item.Name, item.InputSchema)
		}
		properties, ok := item.InputSchema["properties"].(map[string]any)
		if !ok || properties["idempotency_key"] == nil {
			t.Fatalf("%s does not declare idempotency_key", item.Name)
		}
		delete(mutations, item.Name)
	}
	if len(mutations) != 0 {
		t.Fatalf("mutation tools not found: %#v", mutations)
	}
}

func TestInstallToolRequiresExactlyOneCompleteSourceKind(t *testing.T) {
	for _, item := range tools() {
		if item.Name != "tailapp_install" {
			continue
		}
		oneOf, ok := item.InputSchema["oneOf"].([]map[string]any)
		if !ok || len(oneOf) != 2 {
			t.Fatalf("install schema does not select bundle or sources: %#v", item.InputSchema)
		}
		return
	}
	t.Fatal("tailapp_install tool not found")
}

func TestMetricsToolIsExposedWithoutArguments(t *testing.T) {
	for _, item := range tools() {
		if item.Name == "tailapp_metrics" {
			if operation := operations[item.Name]; operation != "metrics" {
				t.Fatalf("metrics operation = %q", operation)
			}
			return
		}
	}
	t.Fatal("tailapp_metrics tool not found")
}

func TestToolSchemasAlwaysDeclarePropertiesObject(t *testing.T) {
	for _, item := range tools() {
		if _, ok := item.InputSchema["properties"].(map[string]any); !ok {
			t.Fatalf("%s properties must be a JSON object: %#v", item.Name, item.InputSchema["properties"])
		}
	}
}

func TestIneffectiveToolRequiresTailappName(t *testing.T) {
	for _, item := range tools() {
		if item.Name != "tailapp_ineffective" {
			continue
		}
		if operation := operations[item.Name]; operation != "ineffective" {
			t.Fatalf("ineffective operation = %q", operation)
		}
		required, ok := item.InputSchema["required"].([]string)
		if !ok || !contains(required, "name") {
			t.Fatalf("ineffective schema = %#v", item.InputSchema)
		}
		return
	}
	t.Fatal("tailapp_ineffective tool not found")
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
