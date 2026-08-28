package metrics

import (
	"testing"
	"time"
)

func TestRegistrySnapshotsBoundedCountersAndCumulativeBuckets(t *testing.T) {
	registry := New()
	acceptDuration := 2 * time.Millisecond
	registry.ObserveIntake("log", 2, 120, "ok", 5*time.Millisecond, &acceptDuration)
	registry.ObserveIntake("invented", 99, 999, "invented", 20*time.Second, nil)
	registry.ObserveRouting(2, 4)
	registry.ObserveRouting(3, 0)
	registry.ObserveDetachedObligations(2)
	processingHandle := NewProcessing()
	processingHandle.Observe(3, true, "ok", 2*time.Millisecond, 12*time.Millisecond)
	processingHandle.Observe(0, false, "retry", 20*time.Millisecond, 25*time.Millisecond)
	registry.ObserveQuery(4, 256, true, "budget", 3*time.Millisecond, 1100*time.Millisecond)
	registry.ObserveControl("query", "query_budget_exceeded", 1100*time.Millisecond)
	registry.ObserveControl("attacker-selected-operation", "attacker-selected-outcome", time.Millisecond)

	snapshot := registry.Snapshot(map[string]*Processing{"guard": processingHandle})
	if snapshot.Intake.RequestsTotal != 2 || snapshot.Intake.ErrorsTotal != 1 || snapshot.Intake.RecordsTotal["log"] != 2 || snapshot.Intake.CanonicalBytesTotal["log"] != 120 {
		t.Fatalf("intake snapshot = %#v", snapshot.Intake)
	}
	if snapshot.Intake.ObligationsTotal != 4 || snapshot.Intake.UnroutedRecordsTotal != 3 || snapshot.Intake.DetachedObligationsTotal != 2 {
		t.Fatalf("routing metrics = %#v", snapshot.Intake)
	}
	if snapshot.Intake.RecordsTotal["unknown"] != 0 {
		t.Fatal("failed intake contributed accepted records")
	}
	processing := snapshot.Processing["guard"]
	if processing.AttemptsTotal != 2 || processing.ErrorsTotal != 1 || processing.IneffectiveTotal != 1 || processing.EmittedTotal != 3 {
		t.Fatalf("processing snapshot = %#v", processing)
	}
	if processing.Duration.Buckets[len(processing.Duration.Buckets)-1].Count != processing.Duration.Count {
		t.Fatalf("histogram is not cumulative: %#v", processing.Duration)
	}
	if snapshot.Queries.RequestsTotal != 1 || snapshot.Queries.ErrorsTotal != 1 || snapshot.Queries.RowsTotal != 4 || snapshot.Queries.ResultBytesTotal != 256 || snapshot.Queries.TruncatedTotal != 1 {
		t.Fatalf("query snapshot = %#v", snapshot.Queries)
	}
	if snapshot.Control["query"].Outcomes["query_budget_exceeded"] != 1 || snapshot.Control["unknown"].Outcomes["operation_failed"] != 1 {
		t.Fatalf("control snapshot = %#v", snapshot.Control)
	}
	if snapshot.Runtime.Goroutines < 1 || snapshot.StartedAt == "" || snapshot.GeneratedAt == "" || snapshot.UptimeSeconds < 0 {
		t.Fatalf("scope/runtime snapshot = %#v", snapshot)
	}
}

func TestProcessingNamesAreOwnedBySnapshot(t *testing.T) {
	registry := New()
	handle := NewProcessing()
	handle.Observe(0, false, "ok", 0, 0)
	if _, exists := registry.Snapshot(nil).Processing["temporary"]; exists {
		t.Fatal("snapshot invented a deleted Tailapp name")
	}
}

func TestHistogramDoesNotTruncateAcrossBucketBoundary(t *testing.T) {
	var measured histogram
	measured.observe(1500 * time.Microsecond)
	snapshot := measured.snapshot()
	if snapshot.Buckets[0].Count != 0 || snapshot.Buckets[1].Count != 1 {
		t.Fatalf("1.5ms buckets = %#v", snapshot.Buckets[:2])
	}
}
