package engine

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/generalbusiness-ai/tailapps/internal/inbox"
	"github.com/generalbusiness-ai/tailapps/internal/query"
)

func TestURLReputationContinuePreservesMaterializedRows(t *testing.T) {
	ctx := context.Background()
	resident, err := Open(ctx, filepath.Join(t.TempDir(), "home"))
	if err != nil {
		t.Fatal(err)
	}
	defer resident.Close()
	app, err := resident.Create(ctx, "url-reputation", "url-reputation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resident.Validate(ctx, app.Name, app.DraftRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := resident.Activate(ctx, app.Name, app.DraftRevision, "reset", true); err != nil {
		t.Fatal(err)
	}
	timestamp := "1788192000000000001"
	record := []byte(`{"attributes":{"tailapp.url.observed_full":"https://example.com/path","tailapp.url.host":"example.com"},"resource":{"attributes":{"service.name":"codex"}}}`)
	consumers, err := resident.consumers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resident.queue.Enqueue(ctx, []inbox.Record{{
		Signal: "log", Name: "tailapp.url.observed", Source: "codex",
		TimeUnixNano: &timestamp, ContentDigest: "sha256:url-continue", JSON: record,
	}}, consumers); err != nil {
		t.Fatal(err)
	}
	if err := resident.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	app, sources, err := resident.App(ctx, app.Name)
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(sources["folds/normalize.jsonata"], []byte(`"facts": [],`), []byte(`"facts":[],`), 1)
	if bytes.Equal(changed, sources["folds/normalize.jsonata"]) {
		t.Fatal("normalizer fixture did not contain expected formatting")
	}
	app, err = resident.Put(ctx, app.Name, "folds/normalize.jsonata", changed, app.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resident.Validate(ctx, app.Name, app.DraftRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := resident.Activate(ctx, app.Name, app.DraftRevision, "continue", false); err != nil {
		t.Fatal(err)
	}
	result, err := resident.Query(ctx, app.Name, query.Request{SQL: `SELECT observed_full, observation_count FROM url_observations`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0][0] != "https://example.com/path" || result.Rows[0][1] != int64(1) {
		t.Fatalf("continue lost URL state: %#v", result.Rows)
	}
}
