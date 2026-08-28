// Package metrics records bounded, payload-free operational measurements for
// one Tailapp resident. Measurements are cumulative for the process lifetime;
// callers build time series by sampling snapshots through CLI or MCP.
package metrics

import (
	"runtime/metrics"
	"strconv"
	"sync/atomic"
	"time"
)

const SnapshotVersion = "tailapp.metrics/v1"

var durationBoundsMS = [...]uint64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

type Bucket struct {
	LEMilliseconds string `json:"le_milliseconds"`
	Count          uint64 `json:"count"`
}
type Histogram struct {
	Count           uint64   `json:"count"`
	SumMilliseconds float64  `json:"sum_milliseconds"`
	Buckets         []Bucket `json:"buckets"`
}
type RequestMetrics struct {
	RequestsTotal uint64            `json:"requests_total"`
	ErrorsTotal   uint64            `json:"errors_total"`
	Outcomes      map[string]uint64 `json:"outcomes"`
	Duration      Histogram         `json:"duration_milliseconds"`
}
type IntakeMetrics struct {
	RequestMetrics
	DurableAcceptDuration       Histogram         `json:"durable_accept_duration_milliseconds"`
	RecordsTotal                map[string]uint64 `json:"records_total"`
	CanonicalBytesTotal         map[string]uint64 `json:"canonical_bytes_total"`
	ObligationsTotal            uint64            `json:"obligations_total"`
	UnroutedRecordsTotal        uint64            `json:"unrouted_records_total"`
	UnroutedRecordsBySignal     map[string]uint64 `json:"unrouted_records_by_signal"`
	DetachedObligationsTotal    uint64            `json:"detached_obligations_total"`
	DetachedObligationsByReason map[string]uint64 `json:"detached_obligations_by_reason"`
}
type ProcessingMetrics struct {
	AttemptsTotal            uint64            `json:"attempts_total"`
	ErrorsTotal              uint64            `json:"errors_total"`
	IneffectiveTotal         uint64            `json:"ineffective_records_total"`
	EmittedTotal             uint64            `json:"emitted_events_total"`
	LastSuccessAt            *string           `json:"last_success_at,omitempty"`
	Outcomes                 map[string]uint64 `json:"outcomes"`
	DetachedObligationsTotal map[string]uint64 `json:"detached_obligations_total"`
	QueueDelay               Histogram         `json:"queue_delay_milliseconds"`
	Duration                 Histogram         `json:"duration_milliseconds"`
}
type QueryMetrics struct {
	RequestMetrics
	EngineLockWait   Histogram `json:"engine_lock_wait_milliseconds"`
	RowsTotal        uint64    `json:"rows_total"`
	ResultBytesTotal uint64    `json:"result_bytes_total"`
	TruncatedTotal   uint64    `json:"truncated_total"`
}
type RuntimeMetrics struct {
	Goroutines       uint64 `json:"goroutines"`
	HeapObjectsBytes uint64 `json:"heap_objects_bytes"`
	TotalMemoryBytes uint64 `json:"total_memory_bytes"`
	GCCycles         uint64 `json:"gc_cycles"`
}
type Snapshot struct {
	Version                      string                       `json:"version"`
	ResetSemantics               string                       `json:"reset_semantics"`
	StartedAt                    string                       `json:"started_at"`
	GeneratedAt                  string                       `json:"generated_at"`
	UptimeSeconds                float64                      `json:"uptime_seconds"`
	SnapshotDurationMilliseconds float64                      `json:"snapshot_duration_milliseconds"`
	ClockRegressionsTotal        uint64                       `json:"clock_regressions_total"`
	Intake                       IntakeMetrics                `json:"intake"`
	Processing                   map[string]ProcessingMetrics `json:"processing"`
	Queries                      QueryMetrics                 `json:"queries"`
	Control                      map[string]RequestMetrics    `json:"control"`
	Runtime                      RuntimeMetrics               `json:"runtime"`
}

type histogram struct {
	sumMicros atomic.Uint64
	buckets   [len(durationBoundsMS) + 1]atomic.Uint64
}

func (h *histogram) observe(duration time.Duration) {
	if duration < 0 {
		duration = 0
	}
	h.sumMicros.Add(uint64(duration / time.Microsecond))
	index := len(durationBoundsMS)
	for candidate, bound := range durationBoundsMS {
		if duration <= time.Duration(bound)*time.Millisecond {
			index = candidate
			break
		}
	}
	h.buckets[index].Add(1)
}
func (h *histogram) snapshot() Histogram {
	buckets := make([]Bucket, 0, len(durationBoundsMS)+1)
	var cumulative uint64
	for index, bound := range durationBoundsMS {
		cumulative += h.buckets[index].Load()
		buckets = append(buckets, Bucket{LEMilliseconds: strconv.FormatUint(bound, 10), Count: cumulative})
	}
	cumulative += h.buckets[len(durationBoundsMS)].Load()
	buckets = append(buckets, Bucket{LEMilliseconds: "+Inf", Count: cumulative})
	return Histogram{Count: cumulative, SumMilliseconds: float64(h.sumMicros.Load()) / 1000, Buckets: buckets}
}

type requestState struct {
	requests atomic.Uint64
	errors   atomic.Uint64
	outcomes map[string]*atomic.Uint64
	duration histogram
}

func newRequestState(outcomes ...string) requestState {
	return requestState{outcomes: atomicCounts(outcomes...)}
}
func (state *requestState) observe(outcome string, duration time.Duration) {
	state.requests.Add(1)
	if outcome != "ok" {
		state.errors.Add(1)
	}
	state.outcomes[outcome].Add(1)
	state.duration.observe(duration)
}
func (state *requestState) snapshot() RequestMetrics {
	return RequestMetrics{RequestsTotal: state.requests.Load(), ErrorsTotal: state.errors.Load(), Outcomes: snapshotCounts(state.outcomes), Duration: state.duration.snapshot()}
}

type intakeState struct {
	requestState
	acceptDuration histogram
	records        map[string]*atomic.Uint64
	bytes          map[string]*atomic.Uint64
	obligations    atomic.Uint64
	unrouted       atomic.Uint64
	unroutedSignal map[string]*atomic.Uint64
	detached       atomic.Uint64
	detachedReason map[string]*atomic.Uint64
}
type queryState struct {
	requestState
	lockWait    histogram
	rows        atomic.Uint64
	resultBytes atomic.Uint64
	truncated   atomic.Uint64
}

// Processing is an atomic per-active-Tailapp handle. The engine owns its name
// and lifetime, so deleted names leave no tombstones in the metrics registry.
type Processing struct {
	attempts            atomic.Uint64
	errors              atomic.Uint64
	ineffective         atomic.Uint64
	emitted             atomic.Uint64
	lastSuccessUnixNano atomic.Int64
	outcomes            map[string]*atomic.Uint64
	detached            map[string]*atomic.Uint64
	queueDelay          histogram
	duration            histogram
}

func NewProcessing() *Processing {
	return &Processing{outcomes: atomicCounts("ok", "gap", "retry", "error"), detached: atomicCounts("projection_gap", "runtime_upgrade", "tailapp_deleted", "other")}
}
func (processing *Processing) Observe(emitted int, ineffective bool, outcome string, queueDelay, duration time.Duration) {
	outcome = processingOutcome(outcome)
	processing.attempts.Add(1)
	if outcome != "ok" {
		processing.errors.Add(1)
	} else {
		processing.lastSuccessUnixNano.Store(time.Now().UnixNano())
	}
	if ineffective {
		processing.ineffective.Add(1)
	}
	if emitted > 0 {
		processing.emitted.Add(uint64(emitted))
	}
	processing.outcomes[outcome].Add(1)
	processing.queueDelay.observe(queueDelay)
	processing.duration.observe(duration)
}
func (processing *Processing) ObserveDetached(reason string, count int64) {
	if count > 0 {
		processing.detached[detachedReason(reason)].Add(uint64(count))
	}
}
func (processing *Processing) Snapshot() ProcessingMetrics {
	result := ProcessingMetrics{AttemptsTotal: processing.attempts.Load(), ErrorsTotal: processing.errors.Load(), IneffectiveTotal: processing.ineffective.Load(), EmittedTotal: processing.emitted.Load(), Outcomes: snapshotCounts(processing.outcomes), DetachedObligationsTotal: snapshotCounts(processing.detached), QueueDelay: processing.queueDelay.snapshot(), Duration: processing.duration.snapshot()}
	if value := processing.lastSuccessUnixNano.Load(); value != 0 {
		formatted := time.Unix(0, value).UTC().Format(time.RFC3339Nano)
		result.LastSuccessAt = &formatted
	}
	return result
}

type Registry struct {
	started          time.Time
	intake           intakeState
	queries          queryState
	control          map[string]*requestState
	clockRegressions atomic.Uint64
}

func New() *Registry {
	registry := &Registry{started: time.Now(), control: map[string]*requestState{}}
	registry.intake.requestState = newRequestState("ok", "busy", "invalid", "limit", "backpressure", "not_ready", "error")
	registry.intake.records = atomicCounts("log", "span", "metric", "unknown")
	registry.intake.bytes = atomicCounts("log", "span", "metric", "unknown")
	registry.intake.unroutedSignal = atomicCounts("log", "span", "metric", "unknown")
	registry.intake.detachedReason = atomicCounts("projection_gap", "runtime_upgrade", "tailapp_deleted", "other")
	registry.queries.requestState = newRequestState("ok", "not_found", "unavailable", "budget", "frontier_changed", "deadline", "cancelled", "error")
	for _, operation := range []string{"health", "status", "metrics", "ineffective", "apps_list", "app_create", "app_install", "app_get", "app_delete", "element_put", "element_delete", "validate", "activate", "schema", "query", "unknown"} {
		state := newRequestState("ok", "not_found", "revision_changed", "idempotency_conflict", "idempotency_in_doubt", "projection_unavailable", "deadline_exceeded", "frontier_changed", "query_budget_exceeded", "operation_failed")
		registry.control[operation] = &state
	}
	return registry
}
func (registry *Registry) ObserveIntake(signal string, records int, canonicalBytes int64, outcome string, requestDuration time.Duration, durableAcceptDuration *time.Duration) {
	signal = intakeSignal(signal)
	outcome = intakeOutcome(outcome)
	registry.intake.observe(outcome, requestDuration)
	if durableAcceptDuration != nil {
		registry.intake.acceptDuration.observe(*durableAcceptDuration)
	}
	if outcome != "ok" {
		return
	}
	if records > 0 {
		registry.intake.records[signal].Add(uint64(records))
	}
	if canonicalBytes > 0 {
		registry.intake.bytes[signal].Add(uint64(canonicalBytes))
	}
}
func (registry *Registry) ObserveRouting(recordsBySignal map[string]int, obligations int) {
	if obligations > 0 {
		registry.intake.obligations.Add(uint64(obligations))
	}
	if obligations != 0 {
		return
	}
	for signal, records := range recordsBySignal {
		if records > 0 {
			registry.intake.unrouted.Add(uint64(records))
			registry.intake.unroutedSignal[intakeSignal(signal)].Add(uint64(records))
		}
	}
}
func (registry *Registry) ObserveDetachedObligations(reason string, count int64) {
	if count > 0 {
		registry.intake.detached.Add(uint64(count))
		registry.intake.detachedReason[detachedReason(reason)].Add(uint64(count))
	}
}
func (registry *Registry) ObserveClockRegression() {
	registry.clockRegressions.Add(1)
}
func (registry *Registry) ObserveQuery(rows, resultBytes int, truncated bool, outcome string, lockWait, duration time.Duration) {
	outcome = queryOutcome(outcome)
	registry.queries.observe(outcome, duration)
	registry.queries.lockWait.observe(lockWait)
	if rows > 0 {
		registry.queries.rows.Add(uint64(rows))
	}
	if resultBytes > 0 {
		registry.queries.resultBytes.Add(uint64(resultBytes))
	}
	if truncated {
		registry.queries.truncated.Add(1)
	}
}
func (registry *Registry) ObserveControl(operation, outcome string, duration time.Duration) {
	registry.control[controlOperation(operation)].observe(controlOutcome(outcome), duration)
}
func (registry *Registry) Snapshot(processing map[string]*Processing) Snapshot {
	started := time.Now()
	now := time.Now()
	result := Snapshot{Version: SnapshotVersion, ResetSemantics: "process counters and histograms reset when the resident restarts; per-projection durable totals do not", StartedAt: registry.started.UTC().Format(time.RFC3339Nano), GeneratedAt: now.UTC().Format(time.RFC3339Nano), UptimeSeconds: now.Sub(registry.started).Seconds(), ClockRegressionsTotal: registry.clockRegressions.Load(), Processing: map[string]ProcessingMetrics{}, Control: map[string]RequestMetrics{}}
	result.Intake = IntakeMetrics{RequestMetrics: registry.intake.requestState.snapshot(), DurableAcceptDuration: registry.intake.acceptDuration.snapshot(), RecordsTotal: snapshotCounts(registry.intake.records), CanonicalBytesTotal: snapshotCounts(registry.intake.bytes), ObligationsTotal: registry.intake.obligations.Load(), UnroutedRecordsTotal: registry.intake.unrouted.Load(), UnroutedRecordsBySignal: snapshotCounts(registry.intake.unroutedSignal), DetachedObligationsTotal: registry.intake.detached.Load(), DetachedObligationsByReason: snapshotCounts(registry.intake.detachedReason)}
	for app, handle := range processing {
		result.Processing[app] = handle.Snapshot()
	}
	result.Queries = QueryMetrics{RequestMetrics: registry.queries.requestState.snapshot(), EngineLockWait: registry.queries.lockWait.snapshot(), RowsTotal: registry.queries.rows.Load(), ResultBytesTotal: registry.queries.resultBytes.Load(), TruncatedTotal: registry.queries.truncated.Load()}
	for operation, state := range registry.control {
		result.Control[operation] = state.snapshot()
	}
	result.Runtime = runtimeSnapshot()
	result.SnapshotDurationMilliseconds = float64(time.Since(started)/time.Microsecond) / 1000
	return result
}
func runtimeSnapshot() RuntimeMetrics {
	samples := []metrics.Sample{{Name: "/sched/goroutines:goroutines"}, {Name: "/memory/classes/heap/objects:bytes"}, {Name: "/memory/classes/total:bytes"}, {Name: "/gc/cycles/total:gc-cycles"}}
	metrics.Read(samples)
	return RuntimeMetrics{Goroutines: samples[0].Value.Uint64(), HeapObjectsBytes: samples[1].Value.Uint64(), TotalMemoryBytes: samples[2].Value.Uint64(), GCCycles: samples[3].Value.Uint64()}
}
func atomicCounts(keys ...string) map[string]*atomic.Uint64 {
	result := make(map[string]*atomic.Uint64, len(keys))
	for _, key := range keys {
		result[key] = &atomic.Uint64{}
	}
	return result
}
func snapshotCounts(source map[string]*atomic.Uint64) map[string]uint64 {
	result := make(map[string]uint64, len(source))
	for key, value := range source {
		result[key] = value.Load()
	}
	return result
}
func intakeSignal(value string) string {
	switch value {
	case "log", "span", "metric":
		return value
	default:
		return "unknown"
	}
}
func intakeOutcome(value string) string {
	switch value {
	case "ok", "busy", "invalid", "limit", "backpressure", "not_ready":
		return value
	default:
		return "error"
	}
}
func processingOutcome(value string) string {
	switch value {
	case "ok", "gap", "retry":
		return value
	default:
		return "error"
	}
}
func queryOutcome(value string) string {
	switch value {
	case "ok", "not_found", "unavailable", "budget", "frontier_changed", "deadline", "cancelled":
		return value
	default:
		return "error"
	}
}
func detachedReason(value string) string {
	switch value {
	case "projection_gap", "runtime_upgrade", "tailapp_deleted":
		return value
	default:
		return "other"
	}
}
func controlOutcome(value string) string {
	switch value {
	case "ok", "not_found", "revision_changed", "idempotency_conflict", "idempotency_in_doubt", "projection_unavailable", "deadline_exceeded", "frontier_changed", "query_budget_exceeded":
		return value
	default:
		return "operation_failed"
	}
}
func controlOperation(value string) string {
	switch value {
	case "health", "status", "metrics", "ineffective", "apps_list", "app_create", "app_install", "app_get", "app_delete", "element_put", "element_delete", "validate", "activate", "schema", "query":
		return value
	default:
		return "unknown"
	}
}
