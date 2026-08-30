package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func listResources(t *testing.T) []resourceEntry {
	t.Helper()
	server := &Server{}
	reply := server.handle(context.Background(), message{JSONRPC: "2.0", ID: 1, Method: "resources/list", Params: json.RawMessage(`{}`)})
	if reply.Error != nil {
		t.Fatalf("resources/list error: %+v", reply.Error)
	}
	result, _ := reply.Result.(map[string]any)
	entries, ok := result["resources"].([]resourceEntry)
	if !ok {
		t.Fatalf("resources = %#v", result["resources"])
	}
	return entries
}

func TestResourceCatalogIsCompleteAndDeterministic(t *testing.T) {
	first := listResources(t)
	second := listResources(t)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("resource listing is not deterministic")
	}
	if len(first) != len(staticPages)+len(tools()) {
		t.Fatalf("catalog size = %d, want %d static + %d tool pages", len(first), len(staticPages), len(tools()))
	}
	uris := make(map[string]bool, len(first))
	for _, entry := range first {
		if entry.MimeType != "text/markdown" {
			t.Fatalf("%s mimeType = %q", entry.URI, entry.MimeType)
		}
		if entry.Title == "" || entry.Description == "" || entry.Name == "" {
			t.Fatalf("%s is missing display fields: %#v", entry.URI, entry)
		}
		if len(entry.Annotations.Audience) == 0 {
			t.Fatalf("%s declares no audience", entry.URI)
		}
		if !strings.HasPrefix(entry.URI, docsScheme) {
			t.Fatalf("%s is outside the docs scheme", entry.URI)
		}
		uris[entry.URI] = true
	}
	for _, item := range tools() {
		if !uris[docsScheme+"tools/"+item.Name] {
			t.Fatalf("tool %s has no documentation resource", item.Name)
		}
	}
	if !uris[docsScheme+"overview"] {
		t.Fatal("the overview resource is missing")
	}
}

func TestEveryResourceReadsAsStandaloneMarkdown(t *testing.T) {
	server := &Server{}
	for _, entry := range listResources(t) {
		reply := server.handle(context.Background(), message{
			JSONRPC: "2.0", ID: 1, Method: "resources/read",
			Params: json.RawMessage(`{"uri":"` + entry.URI + `"}`),
		})
		if reply.Error != nil {
			t.Fatalf("read %s: %+v", entry.URI, reply.Error)
		}
		result, _ := reply.Result.(map[string]any)
		contents, ok := result["contents"].([]map[string]any)
		if !ok || len(contents) != 1 {
			t.Fatalf("read %s contents = %#v", entry.URI, result["contents"])
		}
		text, _ := contents[0]["text"].(string)
		if len(text) < 200 {
			t.Fatalf("read %s returned %d bytes; a page must stand alone", entry.URI, len(text))
		}
		if contents[0]["mimeType"] != "text/markdown" || contents[0]["uri"] != entry.URI {
			t.Fatalf("read %s content envelope = %#v", entry.URI, contents[0])
		}
		for _, relative := range []string{"](../", "](./", "](docs/", "](notes/", "{{"} {
			if strings.Contains(text, relative) {
				t.Fatalf("%s contains a repository-relative link or unexpanded placeholder %q", entry.URI, relative)
			}
		}
	}
}

func TestOverviewCarriesRuntimeProvenance(t *testing.T) {
	text, known := readResourceText(docsScheme + "overview")
	if !known {
		t.Fatal("overview unknown")
	}
	if !strings.Contains(text, "version 0.1.0") {
		t.Fatalf("overview does not carry the build version: %q", text[:200])
	}
	for _, needed := range []string{"tailapps_list", "tailapp_query", "never inline prevention"} {
		if !strings.Contains(text, needed) {
			t.Fatalf("overview must mention %q", needed)
		}
	}
}

func TestToolPagesDeriveFromLiveMetadata(t *testing.T) {
	for _, item := range tools() {
		text, known := readResourceText(docsScheme + "tools/" + item.Name)
		if !known {
			t.Fatalf("no page for %s", item.Name)
		}
		if !strings.Contains(text, item.Description) {
			t.Fatalf("%s page does not carry the live description", item.Name)
		}
		required, _ := item.InputSchema["required"].([]string)
		for _, name := range required {
			if !strings.Contains(text, "`"+name+"` (") {
				t.Fatalf("%s page omits required argument %q", item.Name, name)
			}
		}
		if _, hasExample := toolExamples[item.Name]; !hasExample {
			t.Fatalf("%s has no curated example", item.Name)
		}
	}
}

func TestUnknownResourceURIFailsSafely(t *testing.T) {
	server := &Server{}
	for _, uri := range []string{"tailapp://docs/nope", "tailapp://docs/tools/nope", "file:///etc/passwd", ""} {
		encoded, _ := json.Marshal(map[string]string{"uri": uri})
		reply := server.handle(context.Background(), message{JSONRPC: "2.0", ID: 1, Method: "resources/read", Params: encoded})
		if reply.Error == nil || reply.Error.Code != -32602 {
			t.Fatalf("read %q error = %+v, want -32602", uri, reply.Error)
		}
		if !strings.Contains(reply.Error.Message, "tailapp://docs/overview") {
			t.Fatalf("read %q message must point at the overview: %q", uri, reply.Error.Message)
		}
		data, _ := reply.Error.Data.(map[string]any)
		if data["uri"] != uri {
			t.Fatalf("read %q must echo the uri in data: %#v", uri, reply.Error.Data)
		}
	}
}

func TestPromptsRemainUndeclaredAndUnimplemented(t *testing.T) {
	server := &Server{}
	reply := server.handle(context.Background(), message{JSONRPC: "2.0", ID: 1, Method: "prompts/list", Params: json.RawMessage(`{}`)})
	if reply.Error == nil || reply.Error.Code != -32601 {
		t.Fatalf("prompts/list = %+v, want method not found", reply.Error)
	}
	init := server.handle(context.Background(), message{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: json.RawMessage(`{"protocolVersion":"2025-06-18"}`)})
	capabilities, _ := init.Result.(map[string]any)["capabilities"].(map[string]any)
	if _, declared := capabilities["resources"]; !declared {
		t.Fatal("the resources capability is not declared")
	}
	if _, declared := capabilities["prompts"]; declared {
		t.Fatal("prompts must stay undeclared")
	}
}
