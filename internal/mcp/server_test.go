package mcp

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/generalbusiness-ai/tailapps/internal/control"
)

func TestArrayToolResultOmitsStructuredContent(t *testing.T) {
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
		if envelope.Operation != "apps_list" {
			t.Errorf("operation = %q, want apps_list", envelope.Operation)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"result":[{"name":"example"}]}`))
	})}
	t.Cleanup(func() {
		_ = controlServer.Close()
	})
	go func() {
		if err := controlServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Errorf("serve control socket: %v", err)
		}
	}()

	server := Server{Client: control.NewClient(listener.Addr().String())}
	reply := server.handle(context.Background(), message{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"tailapps_list","arguments":{}}`),
	})
	if reply.Error != nil {
		t.Fatalf("tools/call error: %+v", reply.Error)
	}
	result, ok := reply.Result.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v, want object", reply.Result)
	}
	if _, exists := result["structuredContent"]; exists {
		t.Fatalf("array result must omit structuredContent: %#v", result)
	}
	content, ok := result["content"].([]map[string]string)
	if !ok || len(content) != 1 || content[0]["text"] != `[{"name":"example"}]` {
		t.Fatalf("content = %#v, want complete array JSON", result["content"])
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
