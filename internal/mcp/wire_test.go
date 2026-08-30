package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// The wire harness drives the SDK-served surface exactly as a client
// process would: newline-delimited JSON-RPC over pipes. Conformance
// assertions stay at the wire, where the contract lives.
type wire struct {
	t       *testing.T
	writer  io.Writer
	scanner *bufio.Scanner
	nextID  int
	cancel  context.CancelFunc
}

func startWire(t *testing.T, server *Server) *wire {
	t.Helper()
	serverReads, clientWrites := io.Pipe()
	clientReads, serverWrites := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = server.Serve(ctx, serverReads, serverWrites)
		_ = serverWrites.Close()
	}()
	scanner := bufio.NewScanner(clientReads)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	session := &wire{t: t, writer: clientWrites, scanner: scanner, cancel: cancel}
	t.Cleanup(func() {
		cancel()
		_ = clientWrites.Close()
		_ = clientReads.Close()
	})
	return session
}

// send writes one raw JSON line.
func (session *wire) send(raw string) {
	session.t.Helper()
	if _, err := fmt.Fprintln(session.writer, raw); err != nil {
		session.t.Fatal(err)
	}
}

// receive reads the next response line as a decoded object, failing after a
// timeout.
func (session *wire) receive() map[string]any {
	session.t.Helper()
	type outcome struct {
		line string
		ok   bool
	}
	results := make(chan outcome, 1)
	go func() {
		ok := session.scanner.Scan()
		results <- outcome{session.scanner.Text(), ok}
	}()
	select {
	case result := <-results:
		if !result.ok {
			session.t.Fatalf("wire closed: %v", session.scanner.Err())
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(result.line), &decoded); err != nil {
			session.t.Fatalf("undecodable wire line %q: %v", result.line, err)
		}
		return decoded
	case <-time.After(10 * time.Second):
		session.t.Fatal("no wire response within 10s")
		return nil
	}
}

// call sends a request and returns its response, skipping any notifications.
func (session *wire) call(method string, params any) map[string]any {
	session.t.Helper()
	session.nextID++
	encodedParams, err := json.Marshal(params)
	if err != nil {
		session.t.Fatal(err)
	}
	session.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, session.nextID, method, encodedParams))
	for {
		reply := session.receive()
		if id, hasID := reply["id"]; hasID && id == float64(session.nextID) {
			return reply
		}
	}
}

// initialize performs the legacy handshake at the given protocol version and
// returns the initialize result.
func (session *wire) initialize(protocolVersion string) map[string]any {
	session.t.Helper()
	reply := session.call("initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "wire-test", "version": "0"},
	})
	if reply["error"] != nil {
		session.t.Fatalf("initialize error: %#v", reply["error"])
	}
	session.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	result, _ := reply["result"].(map[string]any)
	return result
}

// content extracts the single text content item from a tools/call result.
func toolText(t *testing.T, reply map[string]any) (string, map[string]any) {
	t.Helper()
	result, _ := reply["result"].(map[string]any)
	if result == nil {
		t.Fatalf("no result: %#v", reply)
	}
	contents, _ := result["content"].([]any)
	if len(contents) != 1 {
		t.Fatalf("content = %#v, want one text item", result["content"])
	}
	item, _ := contents[0].(map[string]any)
	text, _ := item["text"].(string)
	structured, _ := result["structuredContent"].(map[string]any)
	return text, structured
}

func TestInitializeNegotiatesProtocolVersion(t *testing.T) {
	cases := []struct{ requested, want string }{
		{"2024-11-05", "2024-11-05"},
		{"2025-03-26", "2025-03-26"},
		{"2025-06-18", "2025-06-18"},
		{"2025-11-25", "2025-11-25"},
		// An unknown requested version gets the newest legacy revision the
		// SDK serves on the initialize path.
		{"1999-01-01", "2025-11-25"},
	}
	for _, tc := range cases {
		session := startWire(t, &Server{})
		result := session.initialize(tc.requested)
		if result["protocolVersion"] != tc.want {
			t.Fatalf("requested %q: negotiated %v, want %q", tc.requested, result["protocolVersion"], tc.want)
		}
	}
}

func TestInitializeCarriesIdentityInstructionsAndCapabilities(t *testing.T) {
	session := startWire(t, &Server{})
	result := session.initialize("2025-06-18")
	serverInfo, _ := result["serverInfo"].(map[string]any)
	for _, field := range []string{"name", "title", "version", "description"} {
		if value, _ := serverInfo[field].(string); value == "" {
			t.Fatalf("serverInfo.%s is empty: %#v", field, serverInfo)
		}
	}
	text, _ := result["instructions"].(string)
	if text == "" || len(text) >= 2048 {
		t.Fatalf("instructions missing or over the 2KB cap: %d bytes", len(text))
	}
	core := strings.SplitN(text, "\n\n", 2)[0]
	if normalized := strings.Join(strings.Fields(core), " "); len(normalized) > 512 {
		t.Fatalf("instructions core is %d characters space-normalized", len(normalized))
	}
	for _, needed := range []string{"tailapps_list", "tailapp_query", "never inline prevention", "idempotency_key"} {
		if !strings.Contains(core, needed) {
			t.Fatalf("instructions core must mention %q", needed)
		}
	}
	capabilities, _ := result["capabilities"].(map[string]any)
	for _, capability := range []string{"tools", "resources"} {
		if _, declared := capabilities[capability]; !declared {
			t.Fatalf("capability %q undeclared: %#v", capability, capabilities)
		}
	}
	if _, declared := capabilities["prompts"]; declared {
		t.Fatal("prompts must stay undeclared")
	}
}

func TestModernDiscoveryServesTheSameOrientation(t *testing.T) {
	session := startWire(t, &Server{})
	reply := session.call("server/discover", map[string]any{
		"_meta": map[string]any{
			"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
			"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "wire-test", "version": "0"},
		},
	})
	result, _ := reply["result"].(map[string]any)
	if result == nil {
		t.Fatalf("server/discover failed: %#v", reply)
	}
	versions, _ := result["supportedVersions"].([]any)
	found := false
	for _, version := range versions {
		if version == "2026-07-28" {
			found = true
		}
	}
	if !found {
		t.Fatalf("supportedVersions = %#v, want to include 2026-07-28", versions)
	}
	if text, _ := result["instructions"].(string); !strings.Contains(text, "tailapps_list") {
		t.Fatalf("discovery instructions differ from the initialize-era orientation: %q", text)
	}
}

func TestListToolsCarriesFullOrientationOnTheWire(t *testing.T) {
	session := startWire(t, &Server{})
	session.initialize("2025-06-18")
	reply := session.call("tools/list", map[string]any{})
	result, _ := reply["result"].(map[string]any)
	items, _ := result["tools"].([]any)
	if len(items) != len(tools()) {
		t.Fatalf("tools/list = %d entries, want %d", len(items), len(tools()))
	}
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		name, _ := item["name"].(string)
		if item["title"] == "" || item["description"] == "" {
			t.Fatalf("%s lost orientation fields on the wire", name)
		}
		hints, _ := item["annotations"].(map[string]any)
		for _, hint := range []string{"readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint"} {
			if _, present := hints[hint]; !present {
				t.Fatalf("%s annotations omit %s on the wire: %#v", name, hint, hints)
			}
		}
		if _, present := item["outputSchema"]; !present {
			t.Fatalf("%s outputSchema missing on the wire", name)
		}
	}
}

func TestListResultWrapsIntoAppsObjectOnTheWire(t *testing.T) {
	server := newStubServer(t, "apps_list", `[{"name":"example"}]`)
	session := startWire(t, server)
	session.initialize("2025-06-18")
	text, structured := toolText(t, session.call("tools/call", map[string]any{"name": "tailapps_list", "arguments": map[string]any{}}))
	if text != `{"apps":[{"name":"example"}]}` {
		t.Fatalf("text = %q, want wrapped apps object", text)
	}
	apps, _ := structured["apps"].([]any)
	if len(apps) != 1 {
		t.Fatalf("structuredContent.apps = %#v", structured["apps"])
	}
}

func TestEmptyListResultIsNeverNullTextOnTheWire(t *testing.T) {
	server := newStubServer(t, "apps_list", `null`)
	session := startWire(t, server)
	session.initialize("2025-06-18")
	text, structured := toolText(t, session.call("tools/call", map[string]any{"name": "tailapps_list", "arguments": map[string]any{}}))
	if text != `{"apps":[]}` {
		t.Fatalf("text = %q, want empty apps object, never null", text)
	}
	if apps, ok := structured["apps"].([]any); !ok || len(apps) != 0 {
		t.Fatalf("structuredContent.apps = %#v, want empty array", structured["apps"])
	}
}

func TestUnknownToolCallReturnsInvalidParamsOnTheWire(t *testing.T) {
	session := startWire(t, &Server{})
	session.initialize("2025-06-18")
	reply := session.call("tools/call", map[string]any{"name": "nope", "arguments": map[string]any{}})
	failure, _ := reply["error"].(map[string]any)
	if failure == nil || failure["code"] != float64(-32602) {
		t.Fatalf("unknown tool error = %#v, want -32602", reply["error"])
	}
	if message, _ := failure["message"].(string); !strings.Contains(message, `"nope"`) {
		t.Fatalf("unknown tool message must quote the name: %q", message)
	}
}

// closedWithin reports whether the server side of the wire closes within
// the duration.
func (session *wire) closedWithin(limit time.Duration) bool {
	done := make(chan bool, 1)
	go func() { done <- !session.scanner.Scan() }()
	select {
	case closed := <-done:
		return closed
	case <-time.After(limit):
		return false
	}
}

// The SDK transport treats framing corruption as fatal: an unparseable
// line terminates the session rather than answering a parse error and
// continuing (the hand-written adapter's old behavior). A stdio client
// recovers by restarting the server process.
func TestGarbageLineTerminatesTheSession(t *testing.T) {
	session := startWire(t, &Server{})
	session.initialize("2025-06-18")
	session.send("this is not json")
	if !session.closedWithin(5 * time.Second) {
		t.Fatal("the session neither answered nor terminated after an unparseable line")
	}
}
