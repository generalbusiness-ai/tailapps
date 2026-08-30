package mcp

import (
	"reflect"
	"strings"
	"testing"
)

func listResourcesOnTheWire(t *testing.T, session *wire) []map[string]any {
	t.Helper()
	reply := session.call("resources/list", map[string]any{})
	result, _ := reply["result"].(map[string]any)
	raw, _ := result["resources"].([]any)
	entries := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		entry, _ := item.(map[string]any)
		entries = append(entries, entry)
	}
	return entries
}

func TestResourceCatalogIsCompleteAndDeterministicOnTheWire(t *testing.T) {
	session := startWire(t, &Server{})
	session.initialize("2025-06-18")
	first := listResourcesOnTheWire(t, session)
	second := listResourcesOnTheWire(t, session)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("resource listing is not deterministic")
	}
	if len(first) != len(staticPages)+len(tools()) {
		t.Fatalf("catalog size = %d, want %d static + %d tool pages", len(first), len(staticPages), len(tools()))
	}
	uris := make(map[string]bool, len(first))
	for _, entry := range first {
		uri, _ := entry["uri"].(string)
		if entry["mimeType"] != "text/markdown" {
			t.Fatalf("%s mimeType = %v", uri, entry["mimeType"])
		}
		if entry["title"] == "" || entry["description"] == "" || entry["name"] == "" {
			t.Fatalf("%s is missing display fields: %#v", uri, entry)
		}
		hints, _ := entry["annotations"].(map[string]any)
		if audience, _ := hints["audience"].([]any); len(audience) == 0 {
			t.Fatalf("%s declares no audience", uri)
		}
		if !strings.HasPrefix(uri, docsScheme) {
			t.Fatalf("%s is outside the docs scheme", uri)
		}
		uris[uri] = true
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

func TestEveryResourceReadsAsStandaloneMarkdownOnTheWire(t *testing.T) {
	session := startWire(t, &Server{})
	session.initialize("2025-06-18")
	for _, entry := range listResourcesOnTheWire(t, session) {
		uri, _ := entry["uri"].(string)
		reply := session.call("resources/read", map[string]any{"uri": uri})
		result, _ := reply["result"].(map[string]any)
		if result == nil {
			t.Fatalf("read %s: %#v", uri, reply["error"])
		}
		contents, _ := result["contents"].([]any)
		if len(contents) != 1 {
			t.Fatalf("read %s contents = %#v", uri, result["contents"])
		}
		item, _ := contents[0].(map[string]any)
		text, _ := item["text"].(string)
		if len(text) < 200 {
			t.Fatalf("read %s returned %d bytes; a page must stand alone", uri, len(text))
		}
		if item["mimeType"] != "text/markdown" || item["uri"] != uri {
			t.Fatalf("read %s content envelope = %#v", uri, item)
		}
		for _, relative := range []string{"](../", "](./", "](docs/", "](notes/", "{{"} {
			if strings.Contains(text, relative) {
				t.Fatalf("%s contains a repository-relative link or unexpanded placeholder %q", uri, relative)
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

func TestUnknownResourceURIFailsSafelyOnTheWire(t *testing.T) {
	session := startWire(t, &Server{})
	session.initialize("2025-06-18")
	for _, uri := range []string{"tailapp://docs/nope", "tailapp://docs/tools/nope", "file:///etc/passwd", ""} {
		reply := session.call("resources/read", map[string]any{"uri": uri})
		failure, _ := reply["error"].(map[string]any)
		if failure == nil || failure["code"] != float64(-32602) {
			t.Fatalf("read %q error = %#v, want -32602", uri, reply["error"])
		}
		data, _ := failure["data"].(map[string]any)
		if data["uri"] != uri {
			t.Fatalf("read %q must echo the uri in data: %#v", uri, failure["data"])
		}
	}
}

func TestAuthoringPageCountMatchesItsOwnList(t *testing.T) {
	text, known := readResourceText(docsScheme + "authoring")
	if !known {
		t.Fatal("authoring page unknown")
	}
	if !strings.Contains(text, "Five statement kinds") {
		t.Fatalf("authoring page lost its pinned count sentence")
	}
	kinds := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- `CREATE ") {
			word := strings.Fields(strings.TrimPrefix(trimmed, "- `CREATE "))[0]
			kinds[strings.Trim(word, "`")] = true
		}
	}
	if len(kinds) != 5 {
		t.Fatalf("authoring page enumerates %d distinct CREATE kinds (%v); the summary claims five", len(kinds), kinds)
	}
}
