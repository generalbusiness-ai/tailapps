package mcp

import "testing"

func TestMutationToolsRequireIdempotencyKeys(t *testing.T) {
	mutations := map[string]bool{
		"tailapp_create":         true,
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

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
