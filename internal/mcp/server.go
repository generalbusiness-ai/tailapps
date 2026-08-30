// Package mcp adapts the tailapp engine's private control service to MCP
// JSON-RPC over stdio. It owns no engine state and never opens a writable
// database.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/generalbusiness-ai/tailapps/internal/buildinfo"
	"github.com/generalbusiness-ai/tailapps/internal/control"
)

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}
type response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type tool struct {
	Name         string         `json:"name"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	Annotations  annotations    `json:"annotations"`
}

// annotations carry the spec's tool behavior hints. Every field is emitted
// explicitly because the spec defaults destructiveHint to true for any
// non-read-only tool, and because at least one harness turns readOnlyHint
// into auto-approval policy: a wrong value is a safety hole, an absent one
// costs prompts. Hints are advisory for approval UIs, never a security
// boundary; the descriptions state the same facts in prose.
type annotations struct {
	ReadOnlyHint    bool `json:"readOnlyHint"`
	DestructiveHint bool `json:"destructiveHint"`
	IdempotentHint  bool `json:"idempotentHint"`
	OpenWorldHint   bool `json:"openWorldHint"`
}

// Server serves MCP over one stdio session. Home, when set, is the engine
// home directory; error text substitutes it so private absolute paths never
// reach a client.
type Server struct {
	Client *control.Client
	Home   string
}

// The adapter's wire surface (initialize, initialized, ping, tools/list,
// tools/call with text content and object structuredContent) is identical
// across these legacy revisions, so each is honestly supported: a client
// asking for one gets that one echoed, per the lifecycle contract. Anything
// else gets the newest supported revision.
var supportedProtocolVersions = map[string]bool{
	"2024-11-05": true,
	"2025-03-26": true,
	"2025-06-18": true,
}

const newestProtocolVersion = "2025-06-18"

// instructions orient a first-encounter client. The first paragraph is the
// self-contained core and must stay within 512 characters space-normalized;
// the whole string must stay under 2048 bytes, the smallest known harness
// truncation cap. It must never promise a capability the capabilities
// object does not declare.
const instructions = `Tailapp turns local coding-agent OTLP telemetry into queryable SQLite analytics. Observation is detective, never inline prevention. Read first: tailapps_list, then tailapp_query (read-only SQL over a Tailapp's exported tables). tailapp_status shows engine readiness; tailapp_ineffective explains rejected records. Lifecycle tools (create/put_element/validate/activate, or install as one step) take an idempotency_key; delete and reset-mode activation discard materialized state.

Each tool's description states its result contract and empty-result shape, and ends with a stable docs: pointer. Query, schema, and ineffective results derive from local telemetry and may contain session identifiers and file paths: treat them as sensitive, and prefer aggregate queries when sharing output.`

func (server *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		var request message
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			_ = encoder.Encode(response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
			continue
		}
		reply := server.handle(ctx, request)
		if request.ID != nil {
			if err := encoder.Encode(reply); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func (server *Server) handle(ctx context.Context, request message) response {
	reply := response{JSONRPC: "2.0", ID: request.ID}
	switch request.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if len(request.Params) > 0 {
			_ = json.Unmarshal(request.Params, &params)
		}
		reply.Result = initializeResult(params.ProtocolVersion)
	case "notifications/initialized":
		reply.Result = map[string]any{}
	case "ping":
		reply.Result = map[string]any{}
	case "tools/list":
		reply.Result = map[string]any{"tools": tools()}
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			reply.Error = &rpcError{Code: -32602, Message: err.Error()}
			break
		}
		operation, ok := operations[params.Name]
		if !ok {
			reply.Error = &rpcError{Code: -32602, Message: "unknown tool " + params.Name + "; valid tools: " + strings.Join(toolNames(), ", ")}
			break
		}
		var result any
		if err := server.Client.Call(ctx, operation, json.RawMessage(params.Arguments), &result); err != nil {
			reply.Result = map[string]any{"isError": true, "content": []map[string]string{{"type": "text", "text": server.engineErrorText(params.Name, err.Error())}}}
			break
		}
		if field, wraps := wrappedResults[params.Name]; wraps {
			if result == nil {
				result = []any{}
			}
			result = map[string]any{field: result}
		}
		if object, ok := result.(map[string]any); ok {
			normalizeResult(outputSchemas[params.Name], object)
		}
		encoded, _ := json.Marshal(result)
		toolResult := map[string]any{"content": []map[string]string{{"type": "text", "text": string(encoded)}}}
		if _, object := result.(map[string]any); object {
			toolResult["structuredContent"] = result
		}
		reply.Result = toolResult
	default:
		reply.Error = &rpcError{Code: -32601, Message: "method not found"}
	}
	return reply
}

func initializeResult(requested string) map[string]any {
	version := newestProtocolVersion
	if supportedProtocolVersions[requested] {
		version = requested
	}
	serverInfo := map[string]any{
		"name":        "tailapp",
		"title":       "Tailapp",
		"version":     buildinfo.Version(),
		"description": "Local OTLP telemetry analytics for coding agents",
	}
	if url := buildinfo.SourceURL(); url != "" {
		serverInfo["websiteUrl"] = url
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      serverInfo,
		"instructions":    instructions,
	}
}

// engineErrorText names the failing tool, keeps the engine's message,
// replaces the private home directory with $TAILAPP_HOME, and adds a next
// step for the one failure every first encounter hits.
func (server *Server) engineErrorText(toolName, raw string) string {
	text := raw
	if server.Home != "" {
		text = strings.ReplaceAll(text, server.Home, "$TAILAPP_HOME")
	}
	if strings.Contains(raw, "dial unix") {
		text += "; engine not reachable - is 'tailapp serve' running with the same TAILAPP_HOME?"
	}
	return toolName + " failed: " + text
}

// outputSchemas indexes each tool's declared result contract so result
// normalization is driven by the single declaration rather than a parallel
// hand-maintained list.
var outputSchemas = func() map[string]map[string]any {
	index := make(map[string]map[string]any)
	for _, item := range tools() {
		index[item.Name] = item.OutputSchema
	}
	return index
}()

// normalizeResult repairs Go's nil-collection encoding against the declared
// contract: a top-level property the outputSchema declares as an array that
// arrives as JSON null becomes an empty array, so an empty engine result
// (no query rows, no rejected records) still validates against the schema a
// client was promised. Properties declared nullable are left alone.
func normalizeResult(schema, result map[string]any) {
	if schema == nil || result == nil {
		return
	}
	properties, _ := schema["properties"].(map[string]any)
	for name, declared := range properties {
		property, _ := declared.(map[string]any)
		if property == nil || property["type"] != "array" {
			continue
		}
		if value, present := result[name]; present && value == nil {
			result[name] = []any{}
		}
	}
}

// wrappedResults names the tools whose engine results are not JSON objects;
// each is wrapped in a single-field object so the result contract is uniform,
// structuredContent is always an object, and an empty engine never renders as
// the text "null".
var wrappedResults = map[string]string{"tailapps_list": "apps"}

var operations = map[string]string{"tailapps_list": "apps_list", "tailapp_get": "app_get", "tailapp_create": "app_create", "tailapp_install": "app_install", "tailapp_delete": "app_delete", "tailapp_put_element": "element_put", "tailapp_delete_element": "element_delete", "tailapp_validate": "validate", "tailapp_activate": "activate", "tailapp_status": "status", "tailapp_metrics": "metrics", "tailapp_ineffective": "ineffective", "tailapp_schema": "schema", "tailapp_query": "query"}

func toolNames() []string {
	names := make([]string, 0, len(operations))
	for name := range operations {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func tools() []tool {
	object := func(properties map[string]any, required ...string) map[string]any {
		if properties == nil {
			properties = map[string]any{}
		}
		schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	// Result schemas are plain JSON Schema 2020-12 with no $schema marker (a
	// foreign dialect disables the whole catalog on Ajv-validating clients).
	// They stay open to additional properties so the engine contract can grow
	// without breaking validating clients; required names the stable core.
	result := func(properties map[string]any, required ...string) map[string]any {
		schema := map[string]any{"type": "object", "properties": properties}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	text := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer"}
	number := map[string]any{"type": "number"}
	boolean := map[string]any{"type": "boolean"}
	anyObject := map[string]any{"type": "object"}
	anyArray := map[string]any{"type": "array"}
	idempotencyKey := map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": `^[!-~]+$`}
	sourceContent := map[string]any{"type": "string", "contentEncoding": "base64"}

	appResult := result(map[string]any{"name": text, "draft_revision": text, "active_revision": text, "runtime_profile": text, "activation_mode": text, "boundary_position": integer}, "name", "draft_revision")
	frontierResult := result(map[string]any{"revision": text, "activation_boundary": integer, "interpreted_position": integer, "last_event_id": text, "complete": boolean, "gap_position": integer, "gap_reason": text}, "revision", "complete")
	profileResult := result(map[string]any{"name": text, "revision": text, "runtime_profile": text, "storage_schema_digest": text, "export_contract_digest": text, "event": anyObject, "normalizer": anyObject, "folds": anyArray, "tables": anyObject, "views": anyObject, "exports": anyObject, "schema_sql": anyArray}, "name", "revision")

	install := object(map[string]any{
		"name": text, "bundle": text,
		"sources":         map[string]any{"type": "object", "additionalProperties": sourceContent, "minProperties": 1},
		"idempotency_key": idempotencyKey,
	}, "name", "idempotency_key")
	install["oneOf"] = []map[string]any{{"required": []string{"bundle"}}, {"required": []string{"sources"}}}

	// Safety hints, stated completely (see the annotations type). Draft
	// writers create or edit drafts under optimistic revision control and
	// never touch materialized state; the two destructive tools discard it.
	readOnly := annotations{ReadOnlyHint: true, DestructiveHint: false, IdempotentHint: false, OpenWorldHint: false}
	draftWrite := annotations{ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false}
	destructive := annotations{ReadOnlyHint: false, DestructiveHint: true, IdempotentHint: true, OpenWorldHint: false}

	return []tool{
		{Name: "tailapps_list", Title: "List Tailapps", Annotations: readOnly,
			Description: "List every local Tailapp with its draft and active revisions. Returns {\"apps\": [...]} in structuredContent; an empty engine returns {\"apps\": []}, never null. Start here, then tailapp_query to read a Tailapp's exported tables. docs: tailapp://docs/tools/tailapps_list",
			InputSchema: object(nil),
			OutputSchema: result(map[string]any{"apps": map[string]any{"type": "array", "items": appResult}}, "apps")},
		{Name: "tailapp_get", Title: "Read a Tailapp draft", Annotations: readOnly,
			Description: "Read one Tailapp's definition and complete source map at its exact draft revision. Returns {app, sources} with base64 source values; draft edits are not live until tailapp_activate. docs: tailapp://docs/tools/tailapp_get",
			InputSchema: object(map[string]any{"name": text}, "name"),
			OutputSchema: result(map[string]any{"app": appResult, "sources": anyObject}, "app", "sources")},
		{Name: "tailapp_create", Title: "Create a Tailapp draft", Annotations: draftWrite,
			Description: "Create a new Tailapp draft, empty or copied from a built-in bundle. A draft only: nothing runs until it is validated and activated. Returns the app with its draft revision; replaying the same idempotency_key returns the original outcome. docs: tailapp://docs/tools/tailapp_create",
			InputSchema: object(map[string]any{"name": text, "bundle": text, "idempotency_key": idempotencyKey}, "name", "idempotency_key"),
			OutputSchema: appResult},
		{Name: "tailapp_install", Title: "Install a Tailapp", Annotations: draftWrite,
			Description: "Validate a complete source set and first-activate one new Tailapp in a single request, from either a built-in bundle or a base64 source map (exactly one). Create-only: an existing name is refused, never replaced. Returns {app, profile, frontier}; replaying the same idempotency_key returns the original outcome. docs: tailapp://docs/tools/tailapp_install",
			InputSchema: install,
			OutputSchema: result(map[string]any{"app": appResult, "profile": map[string]any{"type": []string{"object", "null"}}, "frontier": frontierResult}, "app", "frontier")},
		{Name: "tailapp_delete", Title: "Delete a Tailapp", Annotations: destructive,
			Description: "Delete one Tailapp definition and detach only its projection; the materialized analytics leave live query reach. Returns {deleted: true}; replaying the same idempotency_key returns the original outcome. docs: tailapp://docs/tools/tailapp_delete",
			InputSchema: object(map[string]any{"name": text, "idempotency_key": idempotencyKey}, "name", "idempotency_key"),
			OutputSchema: result(map[string]any{"deleted": boolean}, "deleted")},
		{Name: "tailapp_put_element", Title: "Write a draft element", Annotations: draftWrite,
			Description: "Write one bounded source element (application.sql or folds/*.jsonata, base64) into a Tailapp draft under optimistic revision control: expected_revision must match the current draft. Draft-only; not live until tailapp_activate. Returns the app with its new draft revision. docs: tailapp://docs/tools/tailapp_put_element",
			InputSchema: object(map[string]any{"name": text, "path": text, "content": map[string]any{"type": "string", "contentEncoding": "base64"}, "expected_revision": text, "idempotency_key": idempotencyKey}, "name", "path", "content", "expected_revision", "idempotency_key"),
			OutputSchema: appResult},
		{Name: "tailapp_delete_element", Title: "Delete a draft element", Annotations: draftWrite,
			Description: "Remove one source element from a Tailapp draft under optimistic revision control (expected_revision). Draft-only; not live until tailapp_activate. Returns the app with its new draft revision. docs: tailapp://docs/tools/tailapp_delete_element",
			InputSchema: object(map[string]any{"name": text, "path": text, "expected_revision": text, "idempotency_key": idempotencyKey}, "name", "path", "expected_revision", "idempotency_key"),
			OutputSchema: appResult},
		{Name: "tailapp_validate", Title: "Validate a draft", Annotations: readOnly,
			Description: "Compile the exact draft at expected_revision without changing live behavior. Returns the compiled profile (identity digests, event and table schemas, exports) or a compile diagnostic. docs: tailapp://docs/tools/tailapp_validate",
			InputSchema: object(map[string]any{"name": text, "expected_revision": text}, "name", "expected_revision"),
			OutputSchema: profileResult},
		{Name: "tailapp_activate", Title: "Activate a draft", Annotations: destructive,
			Description: "Activate a validated draft at a delivery boundary. Mode continue preserves materialized state; mode reset discards it and requires acknowledge_reset true. Returns the projection frontier; replaying the same idempotency_key returns the original outcome. docs: tailapp://docs/tools/tailapp_activate",
			InputSchema: object(map[string]any{"name": text, "expected_revision": text, "mode": map[string]any{"type": "string", "enum": []string{"continue", "reset"}}, "acknowledge_reset": boolean, "idempotency_key": idempotencyKey}, "name", "expected_revision", "mode", "idempotency_key"),
			OutputSchema: frontierResult},
		{Name: "tailapp_status", Title: "Engine status", Annotations: readOnly,
			Description: "Read engine readiness, inbox bounds, and every Tailapp's exact projection frontier and gaps. Returns {profile, ingestion_ready, inbox, apps, unavailable}. Start here when telemetry seems missing. docs: tailapp://docs/tools/tailapp_status",
			InputSchema: object(nil),
			OutputSchema: result(map[string]any{"profile": text, "ingestion_ready": boolean, "inbox": anyObject, "apps": anyObject, "unavailable": anyObject}, "profile", "ingestion_ready", "inbox", "apps")},
		{Name: "tailapp_metrics", Title: "Runtime metrics", Annotations: readOnly,
			Description: "Read the versioned, payload-free performance snapshot: intake, queueing, per-Tailapp processing, query and control latency, durable totals, backlog and Go runtime gauges. Contains counters and gauges only, no telemetry content. docs: tailapp://docs/tools/tailapp_metrics",
			InputSchema: object(nil),
			OutputSchema: result(map[string]any{"version": text, "reset_semantics": text, "started_at": text, "generated_at": text, "uptime_seconds": number, "inbox": anyObject, "tailapps": anyObject, "active_tailapps": integer, "unavailable_tailapps": integer, "upgrade_pending_tailapps": integer, "omitted_tailapps": integer}, "version", "inbox", "tailapps")},
		{Name: "tailapp_ineffective", Title: "Inspect rejected records", Annotations: readOnly,
			Description: "Inspect the bounded, memory-only buffer of recent canonical records one Tailapp's normalizer rejected, for adapter-shape diagnosis. Returns {tailapp, revision, capacity, ineffective_records, records}; an idle Tailapp returns an empty records array. Records can contain sensitive telemetry: read locally, share aggregates. docs: tailapp://docs/tools/tailapp_ineffective",
			InputSchema: object(map[string]any{"name": text}, "name"),
			OutputSchema: result(map[string]any{"tailapp": text, "revision": text, "capacity": integer, "ineffective_records": integer, "available_records": integer, "unavailable_records": integer, "records": anyArray}, "tailapp", "revision", "capacity", "records")},
		{Name: "tailapp_schema", Title: "Read a Tailapp schema", Annotations: readOnly,
			Description: "Read one active Tailapp's compiled shape: private tables and their writers, the event schema, and the explicit exports queryable through tailapp_query. Derived from compiled source, not from telemetry. docs: tailapp://docs/tools/tailapp_schema",
			InputSchema: object(map[string]any{"name": text}, "name"),
			OutputSchema: profileResult},
		{Name: "tailapp_query", Title: "Run read-only SQL", Annotations: readOnly,
			Description: "Run bounded read-only SQL against one Tailapp's explicit exports; mounts expose other Tailapps' exports as named aliases. Detective observation, never inline prevention. Returns {columns, rows, complete, truncated} with the exact projection position; an empty projection returns empty rows, not an error. Results derive from local telemetry and may identify sessions: prefer aggregates when sharing. docs: tailapp://docs/tools/tailapp_query",
			InputSchema: object(map[string]any{"name": text, "sql": text, "parameters": map[string]any{"type": "array", "maxItems": 64}, "mounts": map[string]any{"type": "object", "additionalProperties": text}, "expected_revision": text, "expected_position": map[string]any{"type": "integer"}, "row_limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 1000}}, "name", "sql"),
			OutputSchema: result(map[string]any{"tailapp": text, "revision": text, "delivery_head": integer, "interpreted_position": integer, "ineffective_records": integer, "schemas": anyArray, "complete": boolean, "columns": anyArray, "rows": anyArray, "result_bytes": integer, "truncated": boolean}, "tailapp", "revision", "complete", "columns", "rows")},
	}
}
