package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/generalbusiness-ai/tailapps/internal/inbox"
	operationalmetrics "github.com/generalbusiness-ai/tailapps/internal/metrics"
)

func TestReceiverAcceptsJSONLogsAndProtobufTracesAndMetricsInOrder(t *testing.T) {
	queue, err := inbox.Open(filepath.Join(t.TempDir(), "control.sqlite"), inbox.Limits{Records: 20, Bytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	receiver := NewReceiver(queue, func(context.Context) ([]inbox.Consumer, error) {
		return []inbox.Consumer{{Tailapp: "agent-guard", Revision: "r1"}}, nil
	}, ReceiverLimits{})
	metricRegistry := operationalmetrics.New()
	receiver.SetMetrics(metricRegistry)

	logs := logRequest("codex", "codex.tool_result", "session-1")
	jsonBody, err := protojson.Marshal(logs)
	if err != nil {
		t.Fatal(err)
	}
	response := serve(receiver, "/v1/logs", "application/json", jsonBody)
	if response.Code != http.StatusOK || response.Header().Get("X-Tailapp-Position-First") != "1" {
		t.Fatalf("logs response %d %s", response.Code, response.Body.String())
	}

	traces := traceRequest("claude-code")
	protobufBody, _ := proto.Marshal(traces)
	response = serve(receiver, "/v1/traces", "application/x-protobuf", protobufBody)
	if response.Code != http.StatusOK || response.Header().Get("X-Tailapp-Position-First") != "2" {
		t.Fatalf("traces response %d %s", response.Code, response.Body.String())
	}

	metrics := metricRequest("opencode")
	protobufBody, _ = proto.Marshal(metrics)
	response = serve(receiver, "/v1/metrics", "application/x-protobuf", protobufBody)
	if response.Code != http.StatusOK || response.Header().Get("X-Tailapp-Position-First") != "3" {
		t.Fatalf("metrics response %d %s", response.Code, response.Body.String())
	}

	pending, err := queue.Pending(context.Background(), "agent-guard", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 3 || pending[0].Name != "codex.tool_result" || pending[1].Signal != "span" || pending[2].Name != "agent.tokens" {
		t.Fatalf("pending deliveries = %#v", pending)
	}
	var canonical map[string]any
	if err := json.Unmarshal(pending[0].JSON, &canonical); err != nil {
		t.Fatal(err)
	}
	attributes := canonical["attributes"].(map[string]any)
	if attributes["conversation.id"] != "session-1" || pending[0].Source != "codex" || pending[0].TraceID == nil {
		t.Fatalf("canonical log = %#v / %#v", pending[0], canonical)
	}
	if err := json.Unmarshal(pending[2].JSON, &canonical); err != nil {
		t.Fatal(err)
	}
	metric := canonical["metric"].(map[string]any)
	if metric["aggregation_temporality"] != "AGGREGATION_TEMPORALITY_DELTA" || metric["is_monotonic"] != true {
		t.Fatalf("canonical metric identity = %#v", metric)
	}
	snapshot := metricRegistry.Snapshot(nil)
	if snapshot.Intake.RequestsTotal != 3 || snapshot.Intake.RecordsTotal["log"] != 1 || snapshot.Intake.RecordsTotal["span"] != 1 || snapshot.Intake.RecordsTotal["metric"] != 1 || snapshot.Intake.DurableAcceptDuration.Count != 3 {
		t.Fatalf("receiver metrics = %#v", snapshot.Intake)
	}
}

func TestCanonicalBytesAndDigestAreStable(t *testing.T) {
	request := logRequest("codex", "codex.tool_result", "s1")
	request.ResourceLogs[0].ScopeLogs[0].LogRecords[0].Attributes = append(
		request.ResourceLogs[0].ScopeLogs[0].LogRecords[0].Attributes,
		kv("large_integer", &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 1 << 60}}),
	)
	protobufBody, _ := proto.Marshal(request)
	protobufDecoded := new(collectorlogsv1.ExportLogsServiceRequest)
	if err := decodeOTLP(protobufBody, "application/x-protobuf", protobufDecoded); err != nil {
		t.Fatal(err)
	}
	jsonBody, _ := protojson.Marshal(request)
	jsonDecoded := new(collectorlogsv1.ExportLogsServiceRequest)
	if err := decodeOTLP(jsonBody, "application/json", jsonDecoded); err != nil {
		t.Fatal(err)
	}
	first, err := flattenLogs(protobufDecoded.GetResourceLogs())
	if err != nil {
		t.Fatal(err)
	}
	second, err := flattenLogs(jsonDecoded.GetResourceLogs())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first[0].JSON, second[0].JSON) || first[0].ContentDigest != second[0].ContentDigest {
		t.Fatalf("canonicalization changed: %s / %s", first[0].JSON, second[0].JSON)
	}
	if !bytes.Contains(first[0].JSON, []byte(`"integer_decimal":"1152921504606846976"`)) {
		t.Fatalf("large integer lost precision: %s", first[0].JSON)
	}
}

func TestMalformedAndOverLimitBatchesCommitNothing(t *testing.T) {
	queue, err := inbox.Open(filepath.Join(t.TempDir(), "control.sqlite"), inbox.Limits{Records: 20, Bytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	consumer := func(context.Context) ([]inbox.Consumer, error) {
		return []inbox.Consumer{{Tailapp: "guard", Revision: "r1"}}, nil
	}
	receiver := NewReceiver(queue, consumer, ReceiverLimits{Records: 1})
	response := serve(receiver, "/v1/logs", "application/json", []byte(`{"resourceLogs":`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed response = %d", response.Code)
	}

	request := logRequest("codex", "one", "s1")
	record := request.ResourceLogs[0].ScopeLogs[0].LogRecords[0]
	request.ResourceLogs[0].ScopeLogs[0].LogRecords = append(request.ResourceLogs[0].ScopeLogs[0].LogRecords, proto.Clone(record).(*logsv1.LogRecord))
	body, _ := proto.Marshal(request)
	response = serve(receiver, "/v1/logs", "application/x-protobuf", body)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("limit response = %d %s", response.Code, response.Body.String())
	}
	stats, _ := queue.Stats(context.Background())
	if stats.Records != 0 {
		t.Fatalf("partial request committed: %#v", stats)
	}
}

func TestLoopbackValidation(t *testing.T) {
	for _, address := range []string{"127.0.0.1:4318", "[::1]:4318"} {
		if err := ValidateLoopbackAddress(address); err != nil {
			t.Fatalf("%s: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:4318", "192.0.2.1:4318", "localhost:4318"} {
		if err := ValidateLoopbackAddress(address); err == nil {
			t.Fatalf("non-explicit loopback %s accepted", address)
		}
	}
}

func serve(handler http.Handler, path, contentType string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func logRequest(source, name, session string) *collectorlogsv1.ExportLogsServiceRequest {
	return &collectorlogsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{
			{
				Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{kv("service.name", stringValue(source))}},
				ScopeLogs: []*logsv1.ScopeLogs{
					{
						Scope: &commonv1.InstrumentationScope{Name: "test", Version: "1"},
						LogRecords: []*logsv1.LogRecord{
							{
								EventName: name, TimeUnixNano: 100, ObservedTimeUnixNano: 101,
								TraceId: bytes.Repeat([]byte{1}, 16), SpanId: bytes.Repeat([]byte{2}, 8),
								Attributes: []*commonv1.KeyValue{kv("conversation.id", stringValue(session)), kv("tool_name", stringValue("dangerous_shell"))},
							},
						},
					},
				},
			},
		},
	}
}

func traceRequest(source string) *collectortracev1.ExportTraceServiceRequest {
	return &collectortracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{
			{
				Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{kv("service.name", stringValue(source))}},
				ScopeSpans: []*tracev1.ScopeSpans{
					{Spans: []*tracev1.Span{{Name: "agent.run", StartTimeUnixNano: 200, EndTimeUnixNano: 250, TraceId: bytes.Repeat([]byte{3}, 16), SpanId: bytes.Repeat([]byte{4}, 8)}}},
				},
			},
		},
	}
}

func metricRequest(source string) *collectormetricsv1.ExportMetricsServiceRequest {
	return &collectormetricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricsv1.ResourceMetrics{
			{
				Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{kv("service.name", stringValue(source))}},
				ScopeMetrics: []*metricsv1.ScopeMetrics{
					{Metrics: []*metricsv1.Metric{
						{Name: "agent.tokens", Data: &metricsv1.Metric_Sum{Sum: &metricsv1.Sum{AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, IsMonotonic: true, DataPoints: []*metricsv1.NumberDataPoint{
							{TimeUnixNano: 300, Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 9}},
						}}}},
					}},
				},
			},
		},
	}
}

func kv(key string, value *commonv1.AnyValue) *commonv1.KeyValue {
	return &commonv1.KeyValue{Key: key, Value: value}
}
func stringValue(value string) *commonv1.AnyValue {
	return &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: value}}
}
