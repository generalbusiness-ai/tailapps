package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ncruces/go-sqlite3"
	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"

	"github.com/generalbusiness-ai/tailapp/internal/definition"
	"github.com/generalbusiness-ai/tailapp/internal/inbox"
	"github.com/generalbusiness-ai/tailapp/internal/profile"
	"github.com/generalbusiness-ai/tailapp/internal/projection"
	"github.com/generalbusiness-ai/tailapp/internal/query"
)

func TestEngineLifecycleIngestionProjectionQueryAndIsolation(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "home")
	engine, err := Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if _, err := Open(ctx, home); err == nil || !strings.Contains(err.Error(), "engine_already_running") {
		t.Fatalf("second resident = %v", err)
	}
	guardApp, err := engine.Create(ctx, "agent-guard", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	costApp, err := engine.Create(ctx, "session-cost", "session-cost")
	if err != nil {
		t.Fatal(err)
	}
	guardCompiled, err := engine.Validate(ctx, "agent-guard", guardApp.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	costCompiled, err := engine.Validate(ctx, "session-cost", costApp.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Activate(ctx, "agent-guard", guardApp.DraftRevision, "reset", true); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Activate(ctx, "session-cost", costApp.DraftRevision, "reset", true); err != nil {
		t.Fatal(err)
	}

	request := otlpRequest()
	body, _ := proto.Marshal(request)
	httpRequest := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(body))
	httpRequest.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	engine.Receiver().ServeHTTP(response, httpRequest)
	if response.Code != http.StatusOK || response.Header().Get("X-Tailapp-Position-Last") != "2" {
		t.Fatalf("OTLP response %d: %s", response.Code, response.Body.String())
	}
	if err := engine.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := engine.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Inbox.Records != 0 || status.Apps["agent-guard"].InterpretedPosition != 2 || status.Apps["session-cost"].InterpretedPosition != 2 {
		t.Fatalf("status = %#v", status)
	}
	expected := int64(2)
	joined, err := engine.Query(ctx, "agent-guard", query.Request{SQL: `SELECT p.session_id,c.input_tokens FROM session_progress p JOIN cost.session_cost c ON c.session_id=p.session_id`, ExpectedPosition: &expected}, map[string]string{"cost": "session-cost"})
	if err != nil {
		t.Fatal(err)
	}
	if len(joined.Rows) != 1 || joined.Rows[0][0] != "s1" {
		t.Fatalf("joined = %#v", joined)
	}

	// Program-only continue preserves rows while switching the exact revision.
	app, sources, err := engine.App(ctx, "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	changedProgram := bytes.Replace(sources["folds/guard.jsonata"], []byte(`"Observed a denied tool"`), []byte(`"Observed denied tool use"`), 1)
	app, err = engine.Put(ctx, "agent-guard", "folds/guard.jsonata", changedProgram, app.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	continued, err := engine.Validate(ctx, "agent-guard", app.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if continued.Revision == guardCompiled.Revision {
		t.Fatal("program change did not change revision")
	}
	if _, err := engine.Activate(ctx, "agent-guard", app.DraftRevision, "continue", false); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Query(ctx, "agent-guard", query.Request{SQL: `SELECT COUNT(*) FROM session_progress`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows[0][0] != int64(1) {
		t.Fatalf("continue lost rows: %#v", result)
	}

	// Stored-table changes refuse continuation and require explicit reset,
	// without changing the independent cost projection.
	app, sources, err = engine.App(ctx, "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	changedDDL := bytes.Replace(sources["application.sql"], []byte("total_actions INTEGER NOT NULL"), []byte("total_actions TEXT NOT NULL"), 1)
	app, err = engine.Put(ctx, "agent-guard", "application.sql", changedDDL, app.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Validate(ctx, "agent-guard", app.DraftRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Activate(ctx, "agent-guard", app.DraftRevision, "continue", false); err == nil {
		t.Fatal("incompatible continuation succeeded")
	}
	if _, err := engine.Activate(ctx, "agent-guard", app.DraftRevision, "reset", false); err == nil {
		t.Fatal("unacknowledged reset succeeded")
	}
	if _, err := engine.Activate(ctx, "agent-guard", app.DraftRevision, "reset", true); err != nil {
		t.Fatal(err)
	}
	status, _ = engine.Status(ctx)
	if status.Apps["session-cost"].Revision != costCompiled.Revision {
		t.Fatal("guard reset changed cost revision")
	}
	if err := engine.Delete(ctx, "agent-guard"); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Query(ctx, "agent-guard", query.Request{SQL: `SELECT 1`}, nil); err == nil {
		t.Fatal("deleted tailapp remained queryable")
	}
	if _, err := engine.Query(ctx, "session-cost", query.Request{SQL: `SELECT COUNT(*) FROM session_cost`}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverySettlesProjectionCommitWithoutDuplicateEvaluation(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "home")
	first, err := Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	app, err := first.Create(ctx, "agent-guard", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := first.Validate(ctx, "agent-guard", app.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Activate(ctx, "agent-guard", app.DraftRevision, "reset", true); err != nil {
		t.Fatal(err)
	}
	record := []byte(`{"attributes":{"conversation.id":"recover","tool_name":"read","target":"/workspace","success":true},"resource":{"attributes":{"service.name":"codex"}}}`)
	eventTime := "1787900000000000000"
	positions, err := first.queue.Enqueue(ctx, []inbox.Record{{Signal: "log", Name: "codex.tool_result", Source: "codex", TimeUnixNano: &eventTime, ContentDigest: "sha256:test", JSON: record}}, []inbox.Consumer{{Tailapp: "agent-guard", Revision: compiled.Revision}})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := first.queue.Pending(ctx, "agent-guard", 1)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%#v err=%v", pending, err)
	}
	if _, err := first.active["agent-guard"].Process(ctx, pending[0]); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := second.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := second.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Inbox.Records != 0 || status.Apps["agent-guard"].InterpretedPosition != positions[0] {
		t.Fatalf("recovered status=%#v", status)
	}
	result, err := second.Query(ctx, "agent-guard", query.Request{SQL: `SELECT total_actions FROM session_progress WHERE session_id='recover'`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0][0] != int64(1) {
		t.Fatalf("event evaluated twice: %#v", result)
	}
}

func TestCancelledDrainLeavesObligationPendingForRetry(t *testing.T) {
	ctx := context.Background()
	resident, err := Open(ctx, filepath.Join(t.TempDir(), "home"))
	if err != nil {
		t.Fatal(err)
	}
	defer resident.Close()
	app, err := resident.Create(ctx, "agent-guard", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := resident.Validate(ctx, "agent-guard", app.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resident.Activate(ctx, "agent-guard", app.DraftRevision, "reset", true); err != nil {
		t.Fatal(err)
	}
	eventTime := "1787900000000000000"
	record := []byte(`{"attributes":{"conversation.id":"retry","tool_name":"read","target":"/workspace","success":true},"resource":{"attributes":{"service.name":"codex"}}}`)
	if _, err := resident.queue.Enqueue(ctx, []inbox.Record{{Signal: "log", Name: "codex.tool_result", Source: "codex", TimeUnixNano: &eventTime, ContentDigest: "sha256:retry", JSON: record}}, []inbox.Consumer{{Tailapp: "agent-guard", Revision: compiled.Revision}}); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := resident.Drain(cancelled); err == nil {
		t.Fatal("cancelled drain succeeded")
	}
	status, err := resident.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	frontier := status.Apps["agent-guard"]
	if status.Inbox.Records != 1 || status.Inbox.PendingObligations != 1 || frontier.GapPosition != nil || !frontier.Complete {
		t.Fatalf("cancelled drain lost retry state: %#v", status)
	}
	if err := resident.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	status, err = resident.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Inbox.Records != 0 || status.Apps["agent-guard"].InterpretedPosition != 1 {
		t.Fatalf("retry did not consume pending delivery: %#v", status)
	}
}

func TestGapDetachesOnlyFailingTailapp(t *testing.T) {
	ctx := context.Background()
	resident, err := Open(ctx, filepath.Join(t.TempDir(), "home"))
	if err != nil {
		t.Fatal(err)
	}
	defer resident.Close()
	failing, err := resident.Create(ctx, "failing", "")
	if err != nil {
		t.Fatal(err)
	}
	ddl := []byte(`CREATE EVENT otel_event (id TEXT NOT NULL);
CREATE TABLE bad (id TEXT PRIMARY KEY, status TEXT NOT NULL CHECK(status='valid'));
CREATE TABLE analytic (id TEXT PRIMARY KEY);
CREATE NORMALIZER normalize ON otlp_record USING 'folds/normalize.jsonata' WRITES bad EMITS otel_event;
CREATE FOLD fold ON otel_event USING 'folds/fold.jsonata' WRITES analytic;
CREATE EXPORT bad AS SELECT id, status FROM bad;`)
	failing, err = resident.Put(ctx, "failing", "application.sql", ddl, failing.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	failing, err = resident.Put(ctx, "failing", "folds/normalize.jsonata", []byte(`{"decision":"effective","facts":[],"events":{"otel_event":[]},"tables":{"bad":{"upsert":[{"id":"x","status":"invalid"}]}}}`), failing.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	failing, err = resident.Put(ctx, "failing", "folds/fold.jsonata", []byte(`{"decision":"effective","facts":[],"tables":{"analytic":{"upsert":[]}}}`), failing.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resident.Validate(ctx, "failing", failing.DraftRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := resident.Activate(ctx, "failing", failing.DraftRevision, "reset", true); err != nil {
		t.Fatal(err)
	}
	cost, err := resident.Create(ctx, "session-cost", "session-cost")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resident.Validate(ctx, "session-cost", cost.DraftRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := resident.Activate(ctx, "session-cost", cost.DraftRevision, "reset", true); err != nil {
		t.Fatal(err)
	}
	record := []byte(`{"attributes":{"conversation.id":"s1","input_tokens":1,"output_tokens":2,"cost_microusd":3},"resource":{"attributes":{"service.name":"codex"}}}`)
	eventTime := "1787900000000000000"
	consumers, err := resident.consumers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resident.queue.Enqueue(ctx, []inbox.Record{{Signal: "log", Name: "codex.api_request", Source: "codex", TimeUnixNano: &eventTime, ContentDigest: "sha256:gap", JSON: record}}, consumers); err != nil {
		t.Fatal(err)
	}
	if err := resident.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := resident.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Apps["failing"].GapPosition == nil || status.Apps["session-cost"].InterpretedPosition != 1 || status.Inbox.Records != 0 {
		t.Fatalf("gap isolation=%#v", status)
	}
	nextConsumers, err := resident.consumers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(nextConsumers) != 1 || nextConsumers[0].Tailapp != "session-cost" {
		t.Fatalf("gapped consumer remained enrolled: %#v", nextConsumers)
	}

	// A storage-compatible continuation repairs the gap without discarding
	// the projection. It explicitly skips the detached interval by advancing
	// to the new activation boundary, then rejoins future delivery.
	failing, _, err = resident.App(ctx, "failing")
	if err != nil {
		t.Fatal(err)
	}
	failing, err = resident.Put(ctx, "failing", "folds/normalize.jsonata", []byte(`{"decision":"effective","facts":[],"events":{"otel_event":[]},"tables":{"bad":{"upsert":[{"id":"x","status":"valid"}]}}}`), failing.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resident.Validate(ctx, "failing", failing.DraftRevision); err != nil {
		t.Fatal(err)
	}
	repaired, err := resident.Activate(ctx, "failing", failing.DraftRevision, "continue", false)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.GapPosition != nil || !repaired.Complete || repaired.InterpretedPosition != 1 {
		t.Fatalf("repaired frontier=%#v", repaired)
	}
	consumers, err = resident.consumers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(consumers) != 2 {
		t.Fatalf("repaired consumer set=%#v", consumers)
	}
	if _, err := resident.queue.Enqueue(ctx, []inbox.Record{{Signal: "log", Name: "codex.api_request", Source: "codex", TimeUnixNano: &eventTime, ContentDigest: "sha256:gap-repair", JSON: record}}, consumers); err != nil {
		t.Fatal(err)
	}
	if err := resident.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	status, err = resident.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Apps["failing"].InterpretedPosition != 2 || status.Apps["failing"].GapPosition != nil {
		t.Fatalf("repaired app did not resume=%#v", status.Apps["failing"])
	}
}

func TestUnavailableProjectionDoesNotPreventResidentRecovery(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "home")
	first, err := Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"agent-guard", "session-cost"} {
		app, err := first.Create(ctx, name, name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := first.Validate(ctx, name, app.DraftRevision); err != nil {
			t.Fatal(err)
		}
		if _, err := first.Activate(ctx, name, app.DraftRevision, "reset", true); err != nil {
			t.Fatal(err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(home, "projections", "agent-guard", "state.sqlite")); err != nil {
		t.Fatal(err)
	}

	recovered, err := Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	status, err := recovered.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := status.Unavailable["agent-guard"]; !ok || !status.IngestionReady {
		t.Fatalf("unavailable isolation=%#v", status)
	}
	if _, err := recovered.Query(ctx, "session-cost", query.Request{SQL: `SELECT COUNT(*) FROM session_cost`}, nil); err != nil {
		t.Fatalf("healthy projection query: %v", err)
	}
	if _, err := recovered.Query(ctx, "agent-guard", query.Request{SQL: `SELECT 1`}, nil); !errors.Is(err, ErrProjectionUnavailable) {
		t.Fatalf("unavailable query error=%v", err)
	}
	guard, _, err := recovered.App(ctx, "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recovered.Activate(ctx, "agent-guard", guard.DraftRevision, "reset", true); err != nil {
		t.Fatalf("explicit reset repair: %v", err)
	}
	status, err = recovered.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := status.Unavailable["agent-guard"]; ok {
		t.Fatalf("reset did not repair unavailable projection: %#v", status)
	}
}

func TestRecoveryFinishesResetJournalAfterFileSwitch(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "home")
	first, err := Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	app, err := first.Create(ctx, "agent-guard", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Validate(ctx, "agent-guard", app.DraftRevision); err != nil {
		t.Fatal(err)
	}
	oldFrontier, err := first.Activate(ctx, "agent-guard", app.DraftRevision, "reset", true)
	if err != nil {
		t.Fatal(err)
	}
	app, sources, err := first.App(ctx, "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(sources["folds/guard.jsonata"], []byte("Observed a denied tool"), []byte("Observed denied tool use"), 1)
	app, err = first.Put(ctx, "agent-guard", "folds/guard.jsonata", changed, app.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := first.Validate(ctx, "agent-guard", app.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	journal := definition.ActivationJournal{Name: "agent-guard", NewRevision: compiled.Revision, Runtime: profile.RuntimeID, Mode: "reset", Boundary: 0, ExpectedDraft: app.DraftRevision, OldRevision: &oldFrontier.Revision}
	if err := first.registry.BeginActivation(ctx, journal); err != nil {
		t.Fatal(err)
	}
	stable, candidate, previous := first.activationPaths("agent-guard")
	next, err := projection.Create(ctx, candidate, compiled, 0, "reset")
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.active["agent-guard"].Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stable, previous); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(candidate, stable); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	recovered, err := Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	current, _, err := recovered.App(ctx, "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	if current.ActiveRevision == nil || *current.ActiveRevision != compiled.Revision {
		t.Fatalf("journal did not finalize registry pointer: %#v", current)
	}
	if _, err := os.Stat(previous); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("activation backup retained: %v", err)
	}
}

func TestExpectedRevisionRaceHasOneWinner(t *testing.T) {
	ctx := context.Background()
	resident, err := Open(ctx, filepath.Join(t.TempDir(), "home"))
	if err != nil {
		t.Fatal(err)
	}
	defer resident.Close()
	app, err := resident.Create(ctx, "race", "")
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errorsSeen := make(chan error, 2)
	for index := 0; index < 2; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := resident.Put(ctx, "race", fmt.Sprintf("folds/%d.jsonata", index), []byte(`{}`), app.DraftRevision)
			errorsSeen <- err
		}()
	}
	wait.Wait()
	close(errorsSeen)
	winners, conflicts := 0, 0
	for err := range errorsSeen {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, definition.ErrRevisionChanged):
			conflicts++
		default:
			t.Fatalf("unexpected mutation error: %v", err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("race winners=%d conflicts=%d", winners, conflicts)
	}
}

func TestRuntimeProfileUpgradeKeepsQueryAndControlButClosesIngestion(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "home")
	first, err := Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	app, err := first.Create(ctx, "agent-guard", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Validate(ctx, "agent-guard", app.DraftRevision); err != nil {
		t.Fatal(err)
	}
	active, err := first.Activate(ctx, "agent-guard", app.DraftRevision, "reset", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	// Simulate an older binary's durable identity without changing its source
	// or materialized data. Recovery must not reinterpret that identity under
	// the current profile.
	controlDB, err := sqlite3.Open(filepath.Join(home, "control.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if err := controlDB.Exec(`UPDATE definition_tailapps SET runtime_profile='legacy-profile'; UPDATE definition_revisions SET runtime_profile='legacy-profile' WHERE digest='` + active.Revision + `'`); err != nil {
		t.Fatal(err)
	}
	if err := controlDB.Close(); err != nil {
		t.Fatal(err)
	}
	projectionDB, err := sqlite3.Open(filepath.Join(home, "projections", "agent-guard", "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if err := projectionDB.Exec(`UPDATE tailapp_projection_identity SET runtime_profile='legacy-profile'`); err != nil {
		t.Fatal(err)
	}
	if err := projectionDB.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	status, err := upgraded.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.IngestionReady {
		t.Fatal("profile-mismatched engine reported ingestion ready")
	}
	if _, err := upgraded.Query(ctx, "agent-guard", query.Request{SQL: `SELECT COUNT(*) FROM session_progress`}, nil); err != nil {
		t.Fatalf("query during upgrade: %v", err)
	}

	body, _ := proto.Marshal(otlpRequest())
	request := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	upgraded.Receiver().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "ingestion_not_ready") {
		t.Fatalf("upgrade OTLP response %d: %s", response.Code, response.Body.String())
	}

	current, _, err := upgraded.App(ctx, "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := upgraded.Activate(ctx, "agent-guard", current.DraftRevision, "continue", false); err != nil {
		t.Fatal(err)
	}
	status, err = upgraded.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !status.IngestionReady {
		t.Fatal("current-profile activation did not reopen ingestion")
	}
}

func otlpRequest() *collectorlogsv1.ExportLogsServiceRequest {
	attrs := func(values map[string]*commonv1.AnyValue) []*commonv1.KeyValue {
		result := make([]*commonv1.KeyValue, 0, len(values))
		for key, value := range values {
			result = append(result, &commonv1.KeyValue{Key: key, Value: value})
		}
		return result
	}
	stringValue := func(value string) *commonv1.AnyValue {
		return &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: value}}
	}
	intValue := func(value int64) *commonv1.AnyValue {
		return &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: value}}
	}
	boolValue := func(value bool) *commonv1.AnyValue {
		return &commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: value}}
	}
	return &collectorlogsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{
			{
				Resource: &resourcev1.Resource{Attributes: attrs(map[string]*commonv1.AnyValue{"service.name": stringValue("codex")})},
				ScopeLogs: []*logsv1.ScopeLogs{
					{LogRecords: []*logsv1.LogRecord{
						{EventName: "codex.tool_result", TimeUnixNano: 100, Attributes: attrs(map[string]*commonv1.AnyValue{"conversation.id": stringValue("s1"), "tool_name": stringValue("read"), "target": stringValue("/workspace"), "success": boolValue(true)})},
						{EventName: "codex.api_request", TimeUnixNano: 200, Attributes: attrs(map[string]*commonv1.AnyValue{"conversation.id": stringValue("s1"), "input_tokens": intValue(100), "output_tokens": intValue(25), "cost_microusd": intValue(7)})},
					}},
				},
			},
		},
	}
}
