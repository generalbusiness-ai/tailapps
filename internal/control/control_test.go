package control

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/generalbusiness-ai/tailapp/internal/definition"
	"github.com/generalbusiness-ai/tailapp/internal/engine"
)

func TestControlRequestsAreMeasuredAfterCompletion(t *testing.T) {
	ctx := context.Background()
	resident, err := engine.Open(ctx, filepath.Join(t.TempDir(), "tailapp"))
	if err != nil {
		t.Fatal(err)
	}
	defer resident.Close()
	server := &Server{Engine: resident}
	body, _ := json.Marshal(Request{Operation: "health"})
	request := httptest.NewRequest(http.MethodPost, "/v1/control", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("health response = %d %s", response.Code, response.Body.String())
	}
	snapshot, err := resident.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Control["health"].RequestsTotal != 1 || snapshot.Control["health"].Outcomes["ok"] != 1 {
		t.Fatalf("control metrics = %#v", snapshot.Control["health"])
	}
}

func TestMutationIdempotencySurvivesRestartAndBindsRequest(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "tailapp")
	resident, err := engine.Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{Engine: resident}

	create := CreateArgs{Name: "analytics", IdempotencyKey: "create-analytics-v1"}
	first, err := dispatchArgs(ctx, server, "app_create", create)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := dispatchArgs(ctx, server, "app_create", create)
	if err != nil {
		t.Fatal(err)
	}
	if encoded(first) != encoded(replayed) {
		t.Fatalf("replay changed response: %s != %s", encoded(first), encoded(replayed))
	}
	if _, err := dispatchArgs(ctx, server, "app_create", CreateArgs{Name: "different", IdempotencyKey: create.IdempotencyKey}); !errors.Is(err, definition.ErrIdempotencyConflict) {
		t.Fatalf("key reused for another request: %v", err)
	}
	if err := resident.Close(); err != nil {
		t.Fatal(err)
	}

	resident, err = engine.Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer resident.Close()
	server = &Server{Engine: resident}
	afterRestart, err := dispatchArgs(ctx, server, "app_create", create)
	if err != nil {
		t.Fatal(err)
	}
	if encoded(first) != encoded(afterRestart) {
		t.Fatalf("durable replay changed response: %s != %s", encoded(first), encoded(afterRestart))
	}
}

func TestMutationIdempotencyReplaysErrorsWithoutLaterSideEffects(t *testing.T) {
	ctx := context.Background()
	resident, err := engine.Open(ctx, filepath.Join(t.TempDir(), "tailapp"))
	if err != nil {
		t.Fatal(err)
	}
	defer resident.Close()
	server := &Server{Engine: resident}
	if _, err := dispatchArgs(ctx, server, "app_create", CreateArgs{Name: "analytics", IdempotencyKey: "initial-create"}); err != nil {
		t.Fatal(err)
	}

	duplicate := CreateArgs{Name: "analytics", IdempotencyKey: "duplicate-create"}
	_, firstErr := dispatchArgs(ctx, server, "app_create", duplicate)
	if firstErr == nil {
		t.Fatal("duplicate creation unexpectedly succeeded")
	}
	firstCode := errorCode(firstErr)
	if _, err := dispatchArgs(ctx, server, "app_delete", DeleteArgs{Name: "analytics", IdempotencyKey: "delete-analytics"}); err != nil {
		t.Fatal(err)
	}
	_, replayedErr := dispatchArgs(ctx, server, "app_create", duplicate)
	if replayedErr == nil || errorCode(replayedErr) != firstCode || replayedErr.Error() != firstErr.Error() {
		t.Fatalf("error was not replayed: first=%s/%v replay=%s/%v", firstCode, firstErr, errorCode(replayedErr), replayedErr)
	}
	if _, _, err := resident.App(ctx, "analytics"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("replayed failed creation recreated the app: %v", err)
	}
}

func TestMutationRequiresPrintableBoundedKey(t *testing.T) {
	for _, key := range []string{"", " has-space-boundary", "contains space", string(make([]byte, 129))} {
		if err := validateIdempotencyKey(key); err == nil {
			t.Fatalf("accepted invalid key %q", key)
		}
	}
	if err := validateIdempotencyKey("agent:install-01_retry"); err != nil {
		t.Fatal(err)
	}
}

func TestInstallIsOneIdempotentValidatedActivation(t *testing.T) {
	ctx := context.Background()
	resident, err := engine.Open(ctx, filepath.Join(t.TempDir(), "tailapp"))
	if err != nil {
		t.Fatal(err)
	}
	defer resident.Close()
	server := &Server{Engine: resident}
	request := InstallArgs{Name: "session-cost", Bundle: "session-cost", IdempotencyKey: "install-session-cost-v1"}
	first, err := dispatchArgs(ctx, server, "app_install", request)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := dispatchArgs(ctx, server, "app_install", request)
	if err != nil {
		t.Fatal(err)
	}
	if encoded(first) != encoded(replayed) {
		t.Fatalf("install replay changed response: %s != %s", encoded(first), encoded(replayed))
	}
	app, _, err := resident.App(ctx, "session-cost")
	if err != nil {
		t.Fatal(err)
	}
	if app.ActiveRevision == nil || app.ActivationMode == nil || *app.ActivationMode != "reset" {
		t.Fatalf("install left an inactive app: %#v", app)
	}
}

func dispatchArgs(ctx context.Context, server *Server, operation string, args any) (any, error) {
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	return server.dispatch(ctx, Request{Operation: operation, Args: encodedArgs})
}

func encoded(value any) string {
	result, _ := json.Marshal(value)
	return string(result)
}
