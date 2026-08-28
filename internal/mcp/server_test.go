package mcp

import "testing"

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
