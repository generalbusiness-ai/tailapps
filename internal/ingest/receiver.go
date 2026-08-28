// Package ingest implements Tailapp's loopback OTLP/HTTP acceptance boundary.
package ingest

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/generalbusiness-ai/tailapp/internal/inbox"
)

type ConsumerSource func(context.Context) ([]inbox.Consumer, error)

type ReceiverLimits struct {
	CompressedBytes   int64
	DecompressedBytes int64
	Records           int
	Concurrent        int
	Deadline          time.Duration
}

func (limits ReceiverLimits) normalized() ReceiverLimits {
	if limits.CompressedBytes <= 0 {
		limits.CompressedBytes = 4 << 20
	}
	if limits.DecompressedBytes <= 0 {
		limits.DecompressedBytes = 16 << 20
	}
	if limits.Records <= 0 {
		limits.Records = 10_000
	}
	if limits.Concurrent <= 0 {
		limits.Concurrent = 16
	}
	if limits.Deadline <= 0 {
		limits.Deadline = 10 * time.Second
	}
	return limits
}

type Receiver struct {
	queue     *inbox.Queue
	consumers ConsumerSource
	limits    ReceiverLimits
	requests  chan struct{}
}

func NewReceiver(queue *inbox.Queue, consumers ConsumerSource, limits ReceiverLimits) *Receiver {
	limits = limits.normalized()
	if consumers == nil {
		consumers = func(context.Context) ([]inbox.Consumer, error) { return nil, nil }
	}
	return &Receiver{queue: queue, consumers: consumers, limits: limits, requests: make(chan struct{}, limits.Concurrent)}
}

func ValidateLoopbackAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid OTLP address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("OTLP address must be an explicit loopback IP")
	}
	return nil
}

func (receiver *Receiver) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	select {
	case receiver.requests <- struct{}{}:
		defer func() { <-receiver.requests }()
	default:
		writeError(writer, http.StatusServiceUnavailable, "receiver_busy", "OTLP concurrency limit reached")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), receiver.limits.Deadline)
	defer cancel()
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is accepted")
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || (mediaType != "application/x-protobuf" && mediaType != "application/json") {
		writeError(writer, http.StatusUnsupportedMediaType, "unsupported_content_type", "use application/x-protobuf or application/json")
		return
	}
	body, err := receiver.readBody(request)
	if err != nil {
		writeError(writer, http.StatusRequestEntityTooLarge, "request_too_large", err.Error())
		return
	}
	var records []inbox.Record
	switch request.URL.Path {
	case "/v1/logs":
		message := new(collectorlogsv1.ExportLogsServiceRequest)
		if err := decodeOTLP(body, mediaType, message); err == nil {
			records, err = flattenLogs(message.GetResourceLogs())
		} else {
			writeError(writer, http.StatusBadRequest, "invalid_otlp", err.Error())
			return
		}
	case "/v1/traces":
		message := new(collectortracev1.ExportTraceServiceRequest)
		if err := decodeOTLP(body, mediaType, message); err == nil {
			records, err = flattenTraces(message.GetResourceSpans())
		} else {
			writeError(writer, http.StatusBadRequest, "invalid_otlp", err.Error())
			return
		}
	case "/v1/metrics":
		message := new(collectormetricsv1.ExportMetricsServiceRequest)
		if err := decodeOTLP(body, mediaType, message); err == nil {
			records, err = flattenMetrics(message.GetResourceMetrics())
		} else {
			writeError(writer, http.StatusBadRequest, "invalid_otlp", err.Error())
			return
		}
	default:
		writeError(writer, http.StatusNotFound, "not_found", "unknown OTLP endpoint")
		return
	}
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_record", err.Error())
		return
	}
	if len(records) > receiver.limits.Records {
		writeError(writer, http.StatusRequestEntityTooLarge, "too_many_records", "OTLP record limit exceeded")
		return
	}
	consumers, err := receiver.consumers(ctx)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "consumer_snapshot_failed", "active consumers unavailable")
		return
	}
	positions, err := receiver.queue.Enqueue(ctx, records, consumers)
	if errors.Is(err, inbox.ErrFull) {
		writer.Header().Set("Retry-After", "1")
		writeError(writer, http.StatusServiceUnavailable, "inbox_full", "durable inbox capacity exceeded")
		return
	}
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "persist_failed", "durable acceptance failed")
		return
	}
	writer.Header().Set("X-Tailapp-Records", fmt.Sprint(len(positions)))
	if len(positions) > 0 {
		writer.Header().Set("X-Tailapp-Position-First", fmt.Sprint(positions[0]))
		writer.Header().Set("X-Tailapp-Position-Last", fmt.Sprint(positions[len(positions)-1]))
	}
	if mediaType == "application/x-protobuf" {
		writer.Header().Set("Content-Type", "application/x-protobuf")
		writer.WriteHeader(http.StatusOK)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte("{}"))
}

func (receiver *Receiver) readBody(request *http.Request) ([]byte, error) {
	encoding := strings.ToLower(strings.TrimSpace(request.Header.Get("Content-Encoding")))
	var reader io.Reader = request.Body
	switch encoding {
	case "", "identity":
	case "gzip":
		compressed, err := io.ReadAll(io.LimitReader(request.Body, receiver.limits.CompressedBytes+1))
		if err != nil {
			return nil, errors.New("read request body")
		}
		if int64(len(compressed)) > receiver.limits.CompressedBytes {
			return nil, errors.New("compressed request limit exceeded")
		}
		gzipReader, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return nil, errors.New("invalid gzip body")
		}
		defer gzipReader.Close()
		reader = gzipReader
	default:
		return nil, errors.New("unsupported content encoding")
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, receiver.limits.DecompressedBytes+1))
	if err != nil {
		return nil, errors.New("decompress request body")
	}
	if int64(len(decoded)) > receiver.limits.DecompressedBytes {
		return nil, errors.New("decompressed request limit exceeded")
	}
	return decoded, nil
}

func decodeOTLP(body []byte, mediaType string, target proto.Message) error {
	if mediaType == "application/json" {
		return protojson.UnmarshalOptions{DiscardUnknown: false}.Unmarshal(body, target)
	}
	return proto.Unmarshal(body, target)
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"code": code, "message": message})
}
