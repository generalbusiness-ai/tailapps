package definition

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestMutationLedgerRefusesIndeterminateAndConflictingRetries(t *testing.T) {
	ctx := context.Background()
	registry, err := Open(filepath.Join(t.TempDir(), "control.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()

	if _, replay, err := registry.BeginMutation(ctx, "key-1", "app_delete", "sha256:one"); err != nil || replay {
		t.Fatalf("bind: replay=%v err=%v", replay, err)
	}
	if _, _, err := registry.BeginMutation(ctx, "key-1", "app_delete", "sha256:one"); !errors.Is(err, ErrIdempotencyInDoubt) {
		t.Fatalf("pending retry: %v", err)
	}
	if _, _, err := registry.BeginMutation(ctx, "key-1", "app_delete", "sha256:other"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting retry: %v", err)
	}
	record := MutationRecord{Response: []byte(`{"deleted":true}`)}
	if err := registry.CompleteMutation(ctx, "key-1", "app_delete", "sha256:one", record); err != nil {
		t.Fatal(err)
	}
	replayed, replay, err := registry.BeginMutation(ctx, "key-1", "app_delete", "sha256:one")
	if err != nil || !replay || string(replayed.Response) != string(record.Response) {
		t.Fatalf("completed retry: replay=%v record=%q err=%v", replay, replayed.Response, err)
	}
}
