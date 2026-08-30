package mcp

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/generalbusiness-ai/tailapps/internal/buildinfo"
)

// The documentation resource catalog: finite, offline, embedded, and
// immutable per build. Static pages are installed-only renderings that
// stand alone (no repository-relative links); per-tool pages are rendered
// from the live tool metadata so the catalog and the tool descriptions
// cannot drift. Tool behavior never depends on a client reading resources:
// the catalog is depth, not a prerequisite.

//go:embed docs/*.md
var embeddedDocs embed.FS

const docsScheme = "tailapp://docs/"

type resourceAnnotations struct {
	Audience []string `json:"audience"`
	Priority float64  `json:"priority,omitempty"`
}
type resourceEntry struct {
	URI         string              `json:"uri"`
	Name        string              `json:"name"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	MimeType    string              `json:"mimeType"`
	Annotations resourceAnnotations `json:"annotations"`
}

type staticPage struct {
	slug        string
	title       string
	description string
	priority    float64
}

// The static catalog, in its fixed listing order; tool pages follow in
// tool-catalog order.
var staticPages = []staticPage{
	{"overview", "Tailapp overview", "What Tailapp is, the read-first tools, the draft loop, this server's provenance, and the data-sensitivity posture.", 1},
	{"authoring", "Authoring a Tailapp", "The application.sql statement kinds, program contract, confinement rules, and the host record a normalizer receives.", 0},
	{"query-sql", "Query SQL", "The admitted read-only SQL subset, mounts, the result shape, and the value model.", 0.8},
	{"otlp-records", "Canonical OTLP records", "The canonical record every normalizer receives: envelope fields, payload shape, and retention.", 0},
	{"harnesses", "Connecting harnesses", "Wiring coding-agent OTLP telemetry and MCP to the resident, content gating, and verifying the path.", 0},
}

// toolExamples carries the curated per-tool usage line included in each
// rendered tool page.
var toolExamples = map[string]string{
	"tailapps_list":          `{"name": "tailapps_list", "arguments": {}}`,
	"tailapp_get":            `{"name": "tailapp_get", "arguments": {"name": "session-cost"}}`,
	"tailapp_create":         `{"name": "tailapp_create", "arguments": {"name": "my-app", "idempotency_key": "create-my-app-1"}}`,
	"tailapp_install":        `{"name": "tailapp_install", "arguments": {"name": "session-cost", "bundle": "session-cost", "idempotency_key": "install-session-cost-1"}}`,
	"tailapp_delete":         `{"name": "tailapp_delete", "arguments": {"name": "my-app", "idempotency_key": "delete-my-app-1"}}`,
	"tailapp_put_element":    `{"name": "tailapp_put_element", "arguments": {"name": "my-app", "path": "application.sql", "content": "<base64>", "expected_revision": "sha256:…", "idempotency_key": "put-1"}}`,
	"tailapp_delete_element": `{"name": "tailapp_delete_element", "arguments": {"name": "my-app", "path": "folds/old.jsonata", "expected_revision": "sha256:…", "idempotency_key": "remove-1"}}`,
	"tailapp_validate":       `{"name": "tailapp_validate", "arguments": {"name": "my-app", "expected_revision": "sha256:…"}}`,
	"tailapp_activate":       `{"name": "tailapp_activate", "arguments": {"name": "my-app", "expected_revision": "sha256:…", "mode": "reset", "acknowledge_reset": true, "idempotency_key": "activate-1"}}`,
	"tailapp_status":         `{"name": "tailapp_status", "arguments": {}}`,
	"tailapp_metrics":        `{"name": "tailapp_metrics", "arguments": {}}`,
	"tailapp_ineffective":    `{"name": "tailapp_ineffective", "arguments": {"name": "agent-guard"}}`,
	"tailapp_schema":         `{"name": "tailapp_schema", "arguments": {"name": "agent-guard"}}`,
	"tailapp_query":          `{"name": "tailapp_query", "arguments": {"name": "session-cost", "sql": "SELECT harness, SUM(cost_microusd) FROM session_cost GROUP BY harness"}}`,
}

// resourceCatalog lists every resource in deterministic order.
func resourceCatalog() []resourceEntry {
	entries := make([]resourceEntry, 0, len(staticPages)+len(operations))
	for _, page := range staticPages {
		entries = append(entries, resourceEntry{
			URI: docsScheme + page.slug, Name: page.slug, Title: page.title,
			Description: page.description, MimeType: "text/markdown",
			Annotations: resourceAnnotations{Audience: []string{"assistant"}, Priority: page.priority},
		})
	}
	for _, item := range tools() {
		entries = append(entries, resourceEntry{
			URI: docsScheme + "tools/" + item.Name, Name: "tools/" + item.Name, Title: item.Title,
			Description: "Reference for the " + item.Name + " tool: behavior, arguments, result contract, and an example call.",
			MimeType:    "text/markdown",
			Annotations: resourceAnnotations{Audience: []string{"assistant"}},
		})
	}
	return entries
}

// readResourceText resolves one catalog URI to its Markdown, or reports
// that the URI is unknown.
func readResourceText(uri string) (string, bool) {
	slug, ok := strings.CutPrefix(uri, docsScheme)
	if !ok {
		return "", false
	}
	if toolName, isTool := strings.CutPrefix(slug, "tools/"); isTool {
		for _, item := range tools() {
			if item.Name == toolName {
				return renderToolPage(item), true
			}
		}
		return "", false
	}
	for _, page := range staticPages {
		if page.slug == slug {
			content, err := embeddedDocs.ReadFile("docs/" + slug + ".md")
			if err != nil {
				return "", false
			}
			text := string(content)
			if slug == "overview" {
				text = strings.ReplaceAll(text, "{{version}}", buildinfo.Version())
				sourceLine := ""
				if url := buildinfo.SourceURL(); url != "" {
					sourceLine = ", source " + url
				}
				text = strings.ReplaceAll(text, "{{source_line}}", sourceLine)
			}
			return text, true
		}
	}
	return "", false
}

// renderToolPage derives a tool's reference page from its live catalog
// entry, so the page cannot disagree with what tools/list advertises.
func renderToolPage(item tool) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s (`%s`)\n\n%s\n\n", item.Title, item.Name, item.Description)

	fmt.Fprintf(&builder, "## Behavior\n\n")
	hints := item.Annotations
	switch {
	case hints.ReadOnlyHint:
		builder.WriteString("Read-only: changes nothing.\n")
	case hints.DestructiveHint:
		builder.WriteString("Destructive: discards existing materialized state. Replay-safe through its idempotency key.\n")
	default:
		builder.WriteString("Writes drafts or creates new state; never destroys existing materialized state. Replay-safe through its idempotency key.\n")
	}

	properties, _ := item.InputSchema["properties"].(map[string]any)
	required, _ := item.InputSchema["required"].([]string)
	requiredSet := make(map[string]bool, len(required))
	for _, name := range required {
		requiredSet[name] = true
	}
	if len(properties) > 0 {
		builder.WriteString("\n## Arguments\n\n")
		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			property, _ := properties[name].(map[string]any)
			kind, _ := property["type"].(string)
			marker := "optional"
			if requiredSet[name] {
				marker = "required"
			}
			fmt.Fprintf(&builder, "- `%s` (%s, %s)\n", name, kind, marker)
		}
	} else {
		builder.WriteString("\nNo arguments: pass `{}`.\n")
	}

	if item.OutputSchema != nil {
		if resultProperties, ok := item.OutputSchema["properties"].(map[string]any); ok && len(resultProperties) > 0 {
			builder.WriteString("\n## Result fields\n\n")
			names := make([]string, 0, len(resultProperties))
			for name := range resultProperties {
				names = append(names, name)
			}
			sort.Strings(names)
			fmt.Fprintf(&builder, "`structuredContent` carries: %s. The declared outputSchema names the stable core; results stay open to additions.\n", "`"+strings.Join(names, "`, `")+"`")
		}
	}

	if example, ok := toolExamples[item.Name]; ok {
		fmt.Fprintf(&builder, "\n## Example\n\n```json\n%s\n```\n", example)
	}
	builder.WriteString("\nStart at `tailapp://docs/overview`.\n")
	return builder.String()
}
