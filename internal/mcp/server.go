// Package mcp adapts Tailapp's private control service to MCP JSON-RPC over
// stdio. It owns no engine state and never opens a writable database.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"

	"github.com/generalbusiness-ai/tailapp/internal/control"
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
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type Server struct{ Client *control.Client }

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
		reply.Result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "tailapp", "version": "0.1.0"}}
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
			reply.Error = &rpcError{Code: -32601, Message: "unknown Tailapp tool"}
			break
		}
		var result any
		if err := server.Client.Call(ctx, operation, json.RawMessage(params.Arguments), &result); err != nil {
			reply.Result = map[string]any{"isError": true, "content": []map[string]string{{"type": "text", "text": err.Error()}}}
			break
		}
		encoded, _ := json.Marshal(result)
		reply.Result = map[string]any{"content": []map[string]string{{"type": "text", "text": string(encoded)}}, "structuredContent": result}
	default:
		reply.Error = &rpcError{Code: -32601, Message: "method not found"}
	}
	return reply
}

var operations = map[string]string{"tailapps_list": "apps_list", "tailapp_get": "app_get", "tailapp_create": "app_create", "tailapp_install": "app_install", "tailapp_delete": "app_delete", "tailapp_put_element": "element_put", "tailapp_delete_element": "element_delete", "tailapp_validate": "validate", "tailapp_activate": "activate", "tailapp_status": "status", "tailapp_metrics": "metrics", "tailapp_schema": "schema", "tailapp_query": "query"}

func tools() []tool {
	object := func(properties map[string]any, required ...string) map[string]any {
		schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	text := map[string]any{"type": "string"}
	idempotencyKey := map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": `^[!-~]+$`}
	boolean := map[string]any{"type": "boolean"}
	sourceContent := map[string]any{"type": "string", "contentEncoding": "base64"}
	install := object(map[string]any{
		"name": text, "bundle": text,
		"sources":         map[string]any{"type": "object", "additionalProperties": sourceContent, "minProperties": 1},
		"idempotency_key": idempotencyKey,
	}, "name", "idempotency_key")
	install["oneOf"] = []map[string]any{{"required": []string{"bundle"}}, {"required": []string{"sources"}}}
	return []tool{
		{"tailapps_list", "List local Tailapps and their draft/active revisions.", object(nil)},
		{"tailapp_get", "Read one Tailapp draft and exact source revision. Draft edits are not live until activation.", object(map[string]any{"name": text}, "name")},
		{"tailapp_create", "Create a Tailapp draft, optionally from a bundled source set. Reusing the same idempotency key replays the original outcome.", object(map[string]any{"name": text, "bundle": text, "idempotency_key": idempotencyKey}, "name", "idempotency_key")},
		{"tailapp_install", "Install and first-activate one new Tailapp from a complete source map or bundled example in one validated operation. Existing Tailapps are never replaced.", install},
		{"tailapp_delete", "Delete one Tailapp definition and detach only its projection. Reusing the same idempotency key replays the original outcome.", object(map[string]any{"name": text, "idempotency_key": idempotencyKey}, "name", "idempotency_key")},
		{"tailapp_put_element", "Put a bounded source element using optimistic draft revision control; this does not activate it.", object(map[string]any{"name": text, "path": text, "content": map[string]any{"type": "string", "contentEncoding": "base64"}, "expected_revision": text, "idempotency_key": idempotencyKey}, "name", "path", "content", "expected_revision", "idempotency_key")},
		{"tailapp_delete_element", "Delete a draft element using optimistic revision control; this does not activate it.", object(map[string]any{"name": text, "path": text, "expected_revision": text, "idempotency_key": idempotencyKey}, "name", "path", "expected_revision", "idempotency_key")},
		{"tailapp_validate", "Compile the exact draft without changing live behavior.", object(map[string]any{"name": text, "expected_revision": text}, "name", "expected_revision")},
		{"tailapp_activate", "Activate a validated draft at a delivery boundary. Reset discards prior materialized state and requires acknowledgement.", object(map[string]any{"name": text, "expected_revision": text, "mode": map[string]any{"type": "string", "enum": []string{"continue", "reset"}}, "acknowledge_reset": boolean, "idempotency_key": idempotencyKey}, "name", "expected_revision", "mode", "idempotency_key")},
		{"tailapp_status", "Read engine readiness, inbox bounds, exact projection frontiers and gaps.", object(nil)},
		{"tailapp_metrics", "Read the versioned, payload-free active-use performance snapshot: intake, queueing, per-Tailapp processing, query/control latency, durable totals, backlog gauges, and Go runtime gauges.", object(nil)},
		{"tailapp_schema", "Read one active Tailapp's private schema, writers, event and explicit exports.", object(map[string]any{"name": text}, "name")},
		{"tailapp_query", "Run bounded read-only SQL. Mounted aliases expose only explicit exports; this is detective observation, not inline prevention.", object(map[string]any{"name": text, "sql": text, "parameters": map[string]any{"type": "array", "maxItems": 64}, "mounts": map[string]any{"type": "object", "additionalProperties": text}, "expected_revision": text, "expected_position": map[string]any{"type": "integer"}, "row_limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 1000}}, "name", "sql")},
	}
}
