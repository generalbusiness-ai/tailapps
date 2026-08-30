// Package mcp adapts the tailapp engine's private control service to MCP
// over stdio. It owns no engine state and never opens a writable database.
//
// Since stage 4 of the first-encounter plan the protocol machine is the
// official MCP Go SDK, which serves both eras: legacy initialize-based
// clients (2024-11-05 through 2025-11-25) and the 2026-07-28 stateless
// discovery flow, per the adopted adapter verdict. This package owns what
// the SDK does not: the tool and resource metadata, the engine dispatch,
// result wrapping and schema-driven normalization, error text, and the
// derived skill.
package mcp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync/atomic"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/generalbusiness-ai/tailapps/internal/buildinfo"
	"github.com/generalbusiness-ai/tailapps/internal/control"
)

type tool struct {
	Name         string
	Title        string
	Description  string
	InputSchema  map[string]any
	OutputSchema map[string]any
	Annotations  annotations
}

// annotations carry the spec's tool behavior hints. Every field is emitted
// explicitly because the spec defaults destructiveHint to true for any
// non-read-only tool, and because at least one harness turns readOnlyHint
// into auto-approval policy: a wrong value is a safety hole, an absent one
// costs prompts. Hints are advisory for approval UIs, never a security
// boundary; the descriptions state the same facts in prose.
type annotations struct {
	ReadOnlyHint    bool
	DestructiveHint bool
	IdempotentHint  bool
	OpenWorldHint   bool
}

// Server serves MCP over one stdio session. Home, when set, is the engine
// home directory; error text substitutes it so private absolute paths never
// reach a client.
type Server struct {
	Client *control.Client
	Home   string
}

// instructions orient a first-encounter client. The first paragraph is the
// self-contained core and must stay within 512 characters space-normalized;
// the whole string must stay under 2048 bytes, the smallest known harness
// truncation cap. It must never promise a capability the capabilities
// object does not declare.
const instructions = `Tailapp turns local coding-agent OTLP telemetry into queryable SQLite analytics. Observation is detective, never inline prevention. Read first: tailapps_list, then tailapp_query (read-only SQL over a Tailapp's exported tables). tailapp_status shows engine readiness; tailapp_ineffective explains rejected records. Lifecycle tools (create/put_element/validate/activate, or install as one step) take an idempotency_key; delete and reset-mode activation discard materialized state.

Each tool's description states its result contract and empty-result shape, and ends with a stable docs: pointer. The resource catalog (resources/list) carries an overview and one Markdown page per tool: read tailapp://docs/overview before authoring. Query, schema, and ineffective results derive from local telemetry and may contain session identifiers and file paths: treat them as sensitive, and prefer aggregate queries when sharing output.`

// serverImplementation is the identity both eras advertise.
func serverImplementation() *sdk.Implementation {
	implementation := &sdk.Implementation{
		Name:        "tailapp",
		Title:       "Tailapp",
		Description: "Local OTLP telemetry analytics for coding agents",
		Version:     buildinfo.Version(),
	}
	if url := buildinfo.SourceURL(); url != "" {
		implementation.WebsiteURL = url
	}
	return implementation
}

// sdkServer assembles the SDK server: identity, instructions, every tool
// with its schemas and complete annotations, and the resource catalog.
func (server *Server) sdkServer() *sdk.Server {
	machine := sdk.NewServer(serverImplementation(), &sdk.ServerOptions{Instructions: instructions})
	for _, item := range tools() {
		item := item
		hints := &sdk.ToolAnnotations{
			ReadOnlyHint:    item.Annotations.ReadOnlyHint,
			DestructiveHint: boolPointer(item.Annotations.DestructiveHint),
			IdempotentHint:  item.Annotations.IdempotentHint,
			OpenWorldHint:   boolPointer(item.Annotations.OpenWorldHint),
		}
		definition := &sdk.Tool{
			Name:        item.Name,
			Title:       item.Title,
			Description: item.Description,
			InputSchema: item.InputSchema,
			Annotations: hints,
		}
		if item.OutputSchema != nil {
			definition.OutputSchema = item.OutputSchema
		}
		machine.AddTool(definition, server.toolHandler(item.Name))
	}
	for _, entry := range resourceCatalog() {
		entry := entry
		machine.AddResource(&sdk.Resource{
			URI:         entry.URI,
			Name:        entry.Name,
			Title:       entry.Title,
			Description: entry.Description,
			MIMEType:    entry.MimeType,
			Annotations: resourceSDKAnnotations(entry),
		}, func(ctx context.Context, request *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			text, known := readResourceText(entry.URI)
			if !known {
				return nil, sdk.ResourceNotFoundError(entry.URI)
			}
			return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{
				URI: entry.URI, MIMEType: entry.MimeType, Text: text,
			}}}, nil
		})
	}
	return machine
}

func resourceSDKAnnotations(entry resourceEntry) *sdk.Annotations {
	audience := make([]sdk.Role, 0, len(entry.Annotations.Audience))
	for _, role := range entry.Annotations.Audience {
		audience = append(audience, sdk.Role(role))
	}
	return &sdk.Annotations{Audience: audience, Priority: entry.Annotations.Priority}
}

// toolHandler dispatches one tool to the engine, preserving the stage-1
// result contract: named-object wrapping for non-object results,
// schema-driven normalization of null-valued declared arrays, a serialized
// text block beside structuredContent, and tool-naming home-redacting error
// text as an isError tool result rather than a protocol error.
func (server *Server) toolHandler(name string) sdk.ToolHandler {
	return func(ctx context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		arguments, err := json.Marshal(request.Params.Arguments)
		if err != nil {
			return nil, err
		}
		var result any
		if err := server.Client.Call(ctx, operations[name], json.RawMessage(arguments), &result); err != nil {
			return &sdk.CallToolResult{
				IsError: true,
				Content: []sdk.Content{&sdk.TextContent{Text: server.engineErrorText(name, err.Error())}},
			}, nil
		}
		if field, wraps := wrappedResults[name]; wraps {
			if result == nil {
				result = []any{}
			}
			result = map[string]any{field: result}
		}
		if object, ok := result.(map[string]any); ok {
			normalizeResult(outputSchemas[name], object)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		reply := &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(encoded)}}}
		if object, ok := result.(map[string]any); ok {
			reply.StructuredContent = object
		}
		return reply, nil
	}
}

// Serve runs one MCP session over the given streams until they close. A
// client closing its end of the pipe is the normal way a stdio session
// ends; the SDK reports that as an error whose chain does not expose
// io.EOF, so the input is tracked directly - if our reader genuinely
// reached end of input, the session ending is a clean shutdown however
// the protocol machine phrases it.
func (server *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	tracked := &eofTrackingReader{reader: input}
	transport := &sdk.IOTransport{Reader: io.NopCloser(tracked), Writer: nopWriteCloser{output}}
	err := server.sdkServer().Run(ctx, transport)
	if err != nil && tracked.sawEOF.Load() {
		return nil
	}
	return err
}

// eofTrackingReader is read by the SDK's own goroutine, so the flag is
// atomic.
type eofTrackingReader struct {
	reader io.Reader
	sawEOF atomic.Bool
}

func (tracked *eofTrackingReader) Read(buffer []byte) (int, error) {
	read, err := tracked.reader.Read(buffer)
	if err == io.EOF {
		tracked.sawEOF.Store(true)
	}
	return read, err
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func boolPointer(value bool) *bool { return &value }

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

// wrappedResults names the tools whose engine results are not JSON objects;
// each is wrapped in a single-field object so the result contract is uniform,
// structuredContent is always an object, and an empty engine never renders as
// the text "null".
var wrappedResults = map[string]string{"tailapps_list": "apps"}

var operations = map[string]string{"tailapps_list": "apps_list", "tailapp_get": "app_get", "tailapp_create": "app_create", "tailapp_install": "app_install", "tailapp_delete": "app_delete", "tailapp_put_element": "element_put", "tailapp_delete_element": "element_delete", "tailapp_validate": "validate", "tailapp_activate": "activate", "tailapp_status": "status", "tailapp_metrics": "metrics", "tailapp_ineffective": "ineffective", "tailapp_schema": "schema", "tailapp_query": "query"}
