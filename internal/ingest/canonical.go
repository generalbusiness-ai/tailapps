package ingest

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/generalbusiness-ai/tailapps/internal/inbox"
)

const MaxCanonicalRecordBytes = 256 << 10

func flattenLogs(groups []*logsv1.ResourceLogs) ([]inbox.Record, error) {
	var records []inbox.Record
	for _, group := range groups {
		resource, err := canonicalResource(group.GetResource(), group.GetSchemaUrl())
		if err != nil {
			return nil, err
		}
		source := sourceName(resource)
		for _, scoped := range group.GetScopeLogs() {
			scope, err := canonicalScope(scoped.GetScope(), scoped.GetSchemaUrl())
			if err != nil {
				return nil, err
			}
			for _, record := range scoped.GetLogRecords() {
				attributes, err := attributeMap(record.GetAttributes())
				if err != nil {
					return nil, fmt.Errorf("log attributes: %w", err)
				}
				body, err := anyValue(record.GetBody())
				if err != nil {
					return nil, fmt.Errorf("log body: %w", err)
				}
				name := record.GetEventName()
				if name == "" {
					if value, ok := attributes["event.name"].(string); ok {
						name = value
					}
				}
				canonical := map[string]any{
					"attributes": attributes, "body": body, "resource": resource, "scope": scope,
					"severity_number": record.GetSeverityNumber().String(), "severity_text": record.GetSeverityText(),
					"flags": record.GetFlags(), "dropped_attributes_count": record.GetDroppedAttributesCount(),
				}
				result, err := makeRecord("log", name, source, record.GetTimeUnixNano(), record.GetObservedTimeUnixNano(), record.GetTraceId(), record.GetSpanId(), canonical, record)
				if err != nil {
					return nil, err
				}
				records = append(records, result)
			}
		}
	}
	return records, nil
}

func flattenTraces(groups []*tracev1.ResourceSpans) ([]inbox.Record, error) {
	var records []inbox.Record
	for _, group := range groups {
		resource, err := canonicalResource(group.GetResource(), group.GetSchemaUrl())
		if err != nil {
			return nil, err
		}
		source := sourceName(resource)
		for _, scoped := range group.GetScopeSpans() {
			scope, err := canonicalScope(scoped.GetScope(), scoped.GetSchemaUrl())
			if err != nil {
				return nil, err
			}
			for _, span := range scoped.GetSpans() {
				attributes, err := attributeMap(span.GetAttributes())
				if err != nil {
					return nil, fmt.Errorf("span attributes: %w", err)
				}
				canonical := map[string]any{
					"attributes": attributes, "resource": resource, "scope": scope,
					"kind": span.GetKind().String(), "parent_span_id": optionalHex(span.GetParentSpanId()),
					"start_time_unix_nano": uintText(span.GetStartTimeUnixNano()), "end_time_unix_nano": uintText(span.GetEndTimeUnixNano()),
					"trace_state": span.GetTraceState(), "flags": span.GetFlags(),
				}
				result, err := makeRecord("span", span.GetName(), source, span.GetStartTimeUnixNano(), 0, span.GetTraceId(), span.GetSpanId(), canonical, span)
				if err != nil {
					return nil, err
				}
				records = append(records, result)
			}
		}
	}
	return records, nil
}

func flattenMetrics(groups []*metricsv1.ResourceMetrics) ([]inbox.Record, error) {
	var records []inbox.Record
	for _, group := range groups {
		resource, err := canonicalResource(group.GetResource(), group.GetSchemaUrl())
		if err != nil {
			return nil, err
		}
		source := sourceName(resource)
		for _, scoped := range group.GetScopeMetrics() {
			scope, err := canonicalScope(scoped.GetScope(), scoped.GetSchemaUrl())
			if err != nil {
				return nil, err
			}
			for _, metric := range scoped.GetMetrics() {
				metricIdentity := map[string]any{"name": metric.GetName(), "description": metric.GetDescription(), "unit": metric.GetUnit()}
				switch {
				case metric.GetSum() != nil:
					metricIdentity["aggregation_temporality"] = metric.GetSum().GetAggregationTemporality().String()
					metricIdentity["is_monotonic"] = metric.GetSum().GetIsMonotonic()
				case metric.GetHistogram() != nil:
					metricIdentity["aggregation_temporality"] = metric.GetHistogram().GetAggregationTemporality().String()
				case metric.GetExponentialHistogram() != nil:
					metricIdentity["aggregation_temporality"] = metric.GetExponentialHistogram().GetAggregationTemporality().String()
				}
				appendPoint := func(point proto.Message, attributes []*commonv1.KeyValue, start, end uint64, pointType string) error {
					attrs, err := attributeMap(attributes)
					if err != nil {
						return fmt.Errorf("metric point attributes: %w", err)
					}
					identity := make(map[string]any, len(metricIdentity)+1)
					for key, value := range metricIdentity {
						identity[key] = value
					}
					identity["point_type"] = pointType
					canonical := map[string]any{
						"attributes": attrs, "resource": resource, "scope": scope,
						"metric":               identity,
						"start_time_unix_nano": uintText(start), "time_unix_nano": uintText(end),
					}
					result, err := makeRecord("metric", metric.GetName(), source, end, 0, nil, nil, canonical, point)
					if err == nil {
						records = append(records, result)
					}
					return err
				}
				switch {
				case metric.GetGauge() != nil:
					for _, point := range metric.GetGauge().GetDataPoints() {
						if err := appendPoint(point, point.GetAttributes(), point.GetStartTimeUnixNano(), point.GetTimeUnixNano(), "gauge"); err != nil {
							return nil, err
						}
					}
				case metric.GetSum() != nil:
					for _, point := range metric.GetSum().GetDataPoints() {
						if err := appendPoint(point, point.GetAttributes(), point.GetStartTimeUnixNano(), point.GetTimeUnixNano(), "sum"); err != nil {
							return nil, err
						}
					}
				case metric.GetHistogram() != nil:
					for _, point := range metric.GetHistogram().GetDataPoints() {
						if err := appendPoint(point, point.GetAttributes(), point.GetStartTimeUnixNano(), point.GetTimeUnixNano(), "histogram"); err != nil {
							return nil, err
						}
					}
				case metric.GetExponentialHistogram() != nil:
					for _, point := range metric.GetExponentialHistogram().GetDataPoints() {
						if err := appendPoint(point, point.GetAttributes(), point.GetStartTimeUnixNano(), point.GetTimeUnixNano(), "exponential_histogram"); err != nil {
							return nil, err
						}
					}
				case metric.GetSummary() != nil:
					for _, point := range metric.GetSummary().GetDataPoints() {
						if err := appendPoint(point, point.GetAttributes(), point.GetStartTimeUnixNano(), point.GetTimeUnixNano(), "summary"); err != nil {
							return nil, err
						}
					}
				default:
					return nil, fmt.Errorf("metric %q has no supported data points", metric.GetName())
				}
			}
		}
	}
	return records, nil
}

func makeRecord(signal, name, source string, eventTime, observed uint64, traceID, spanID []byte, canonical map[string]any, original proto.Message) (inbox.Record, error) {
	raw, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(original)
	if err != nil {
		return inbox.Record{}, err
	}
	var originalJSON any
	if err := json.Unmarshal(raw, &originalJSON); err != nil {
		return inbox.Record{}, err
	}
	canonical["otel"] = originalJSON
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return inbox.Record{}, err
	}
	if len(encoded) > MaxCanonicalRecordBytes {
		return inbox.Record{}, fmt.Errorf("canonical record exceeds %d bytes", MaxCanonicalRecordBytes)
	}
	digest := sha256.Sum256(encoded)
	return inbox.Record{
		Signal: signal, Name: name, Source: source, TimeUnixNano: optionalUint(eventTime), ObservedUnixNano: optionalUint(observed),
		TraceID: optionalHex(traceID), SpanID: optionalHex(spanID), ContentDigest: "sha256:" + hex.EncodeToString(digest[:]), JSON: encoded,
	}, nil
}

func canonicalResource(resource *resourcev1.Resource, schemaURL string) (map[string]any, error) {
	attributes, err := attributeMap(resource.GetAttributes())
	if err != nil {
		return nil, fmt.Errorf("resource attributes: %w", err)
	}
	return map[string]any{"attributes": attributes, "dropped_attributes_count": resource.GetDroppedAttributesCount(), "schema_url": schemaURL}, nil
}

func canonicalScope(scope *commonv1.InstrumentationScope, schemaURL string) (map[string]any, error) {
	attributes, err := attributeMap(scope.GetAttributes())
	if err != nil {
		return nil, fmt.Errorf("scope attributes: %w", err)
	}
	return map[string]any{"name": scope.GetName(), "version": scope.GetVersion(), "attributes": attributes, "dropped_attributes_count": scope.GetDroppedAttributesCount(), "schema_url": schemaURL}, nil
}

func attributeMap(attributes []*commonv1.KeyValue) (map[string]any, error) {
	result := make(map[string]any, len(attributes))
	for _, attribute := range attributes {
		if attribute == nil || attribute.GetKey() == "" {
			return nil, errors.New("attribute key is empty")
		}
		if _, exists := result[attribute.GetKey()]; exists {
			return nil, fmt.Errorf("attribute key %q is duplicated", attribute.GetKey())
		}
		value, err := anyValue(attribute.GetValue())
		if err != nil {
			return nil, fmt.Errorf("attribute %q: %w", attribute.GetKey(), err)
		}
		result[attribute.GetKey()] = value
	}
	return result, nil
}

func anyValue(value *commonv1.AnyValue) (any, error) {
	if value == nil || value.Value == nil {
		return nil, nil
	}
	switch typed := value.Value.(type) {
	case *commonv1.AnyValue_StringValue:
		return typed.StringValue, nil
	case *commonv1.AnyValue_BoolValue:
		return typed.BoolValue, nil
	case *commonv1.AnyValue_IntValue:
		if typed.IntValue < -(1<<53-1) || typed.IntValue > 1<<53-1 {
			return map[string]any{"integer_decimal": strconv.FormatInt(typed.IntValue, 10)}, nil
		}
		return typed.IntValue, nil
	case *commonv1.AnyValue_DoubleValue:
		if math.IsNaN(typed.DoubleValue) || math.IsInf(typed.DoubleValue, 0) {
			return nil, errors.New("non-finite double")
		}
		return typed.DoubleValue, nil
	case *commonv1.AnyValue_BytesValue:
		return map[string]any{"bytes_base64": base64.StdEncoding.EncodeToString(typed.BytesValue)}, nil
	case *commonv1.AnyValue_ArrayValue:
		result := make([]any, len(typed.ArrayValue.GetValues()))
		for index, item := range typed.ArrayValue.GetValues() {
			converted, err := anyValue(item)
			if err != nil {
				return nil, err
			}
			result[index] = converted
		}
		return result, nil
	case *commonv1.AnyValue_KvlistValue:
		return attributeMap(typed.KvlistValue.GetValues())
	default:
		return nil, errors.New("unsupported OTLP value")
	}
}

func sourceName(resource map[string]any) string {
	attributes, _ := resource["attributes"].(map[string]any)
	for _, key := range []string{"service.name", "gen_ai.agent.name", "telemetry.sdk.name"} {
		if value, ok := attributes[key].(string); ok && value != "" {
			return value
		}
	}
	return "unknown"
}

func optionalUint(value uint64) *string {
	if value == 0 {
		return nil
	}
	result := strconv.FormatUint(value, 10)
	return &result
}

func uintText(value uint64) any {
	if value == 0 {
		return nil
	}
	return strconv.FormatUint(value, 10)
}

func optionalHex(value []byte) *string {
	if len(value) == 0 {
		return nil
	}
	result := hex.EncodeToString(value)
	return &result
}
