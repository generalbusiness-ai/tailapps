package query

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/generalbusiness-ai/tailapps/internal/inbox"
	"github.com/generalbusiness-ai/tailapps/internal/profile"
	"github.com/generalbusiness-ai/tailapps/internal/projection"
	"github.com/generalbusiness-ai/tailapps/tailapps"
)

func TestJSONQueryPreservesLogicalTypeAndNumbers(t *testing.T) {
	ctx := context.Background()
	app, err := profile.LoadCurrent(os.DirFS("../../jsonataddl/corpus/v1/basic/app"), ".", "json-query")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.sqlite")
	p, err := projection.Create(ctx, path, app, 0, "reset")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err = p.Database().Exec("INSERT INTO totals (key,total,extra) VALUES ('a',0,?)", "9007199254740993"); err != nil {
		t.Fatal(err)
	}
	frontier, err := p.Frontier(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := Open(Namespace{Path: path, Profile: app, Frontier: frontier}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sandbox.Close()
	result, err := sandbox.Query(ctx, Request{SQL: "SELECT extra AS payload FROM totals"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Columns) != 1 || result.Columns[0].Type != "JSON" || result.Columns[0].Name != "payload" {
		t.Fatalf("query exposed a physical type: %#v", result.Columns)
	}
	if len(result.Rows) != 1 || result.Rows[0][0] != json.Number("9007199254740993") {
		t.Fatalf("query lost JSON number precision: %#v", result.Rows)
	}
}

func TestBoundedQueryJoinsOnlyExplicitExportsAtAlignedFrontier(t *testing.T) {
	guardProfile, err := tailapps.Load("agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	costProfile, err := tailapps.Load("session-cost")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	guard, err := projection.Create(context.Background(), filepath.Join(root, "guard.sqlite"), guardProfile, 0, "reset")
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	cost, err := projection.Create(context.Background(), filepath.Join(root, "cost.sqlite"), costProfile, 0, "reset")
	if err != nil {
		t.Fatal(err)
	}
	defer cost.Close()

	deliveries := []inbox.Delivery{
		delivery(1, "codex.tool_result", map[string]any{"conversation.id": "s1", "tool_name": "read", "target": "/workspace", "success": true}),
		delivery(2, "codex.api_request", map[string]any{"conversation.id": "s1", "input_tokens": 100, "output_tokens": 25, "cost_microusd": 7}),
	}
	for _, item := range deliveries {
		if _, err := guard.Process(context.Background(), item); err != nil {
			t.Fatal(err)
		}
		if _, err := cost.Process(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	guardFrontier, _ := guard.Frontier(context.Background())
	costFrontier, _ := cost.Frontier(context.Background())
	sandbox, err := Open(Namespace{Path: filepath.Join(root, "guard.sqlite"), Profile: guardProfile, Frontier: guardFrontier, DeliveryHead: 5, IneffectiveRecords: 1}, map[string]Namespace{
		"cost": {Path: filepath.Join(root, "cost.sqlite"), Profile: costProfile, Frontier: costFrontier},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sandbox.Close()
	expected := int64(2)
	result, err := sandbox.Query(context.Background(), Request{SQL: `
SELECT progress.session_id, progress.total_actions, costrow.input_tokens, costrow.cost_microusd
FROM session_progress AS progress
JOIN cost.session_cost AS costrow ON costrow.session_id = progress.session_id
ORDER BY progress.session_id`, ExpectedRevision: guardProfile.Revision, ExpectedPosition: &expected})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || fmt.Sprint(result.Rows[0]) != "[s1 1 100 7]" || len(result.Schemas) != 1 || result.Schemas[0].Contract != costProfile.ExportContractDigest || result.DeliveryHead != 5 || result.InterpretedPosition != 2 || result.IneffectiveRecords != 1 {
		t.Fatalf("joined result = %#v", result)
	}
	if result.ResultBytes == 0 {
		t.Fatal("query did not report encoded result bytes")
	}

	for name, sql := range map[string]string{
		"write":                  `DELETE FROM session_progress`,
		"pragma":                 `PRAGMA query_only`,
		"attach":                 `ATTACH DATABASE ':memory:' AS x`,
		"unsafe function":        `SELECT random() FROM session_progress`,
		"platform relation":      `SELECT revision FROM tailapp_projection_identity`,
		"private mount relation": `SELECT * FROM cost.tailapp_stats`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := sandbox.Query(context.Background(), Request{SQL: sql}); err == nil {
				t.Fatal("unsafe query succeeded")
			}
		})
	}
	stale := int64(1)
	if _, err := sandbox.Query(context.Background(), Request{SQL: `SELECT session_id FROM session_progress`, ExpectedPosition: &stale}); err == nil || !strings.Contains(err.Error(), "frontier_changed") {
		t.Fatalf("stale frontier = %v", err)
	}
}

func TestMountRewriteDoesNotTouchQuotedText(t *testing.T) {
	namespace := Namespace{Profile: &stubProfile}
	rewritten, err := rewriteMountExports(`SELECT 'cost.session_cost', cost.session_cost.session_id FROM cost.session_cost`, map[string]Namespace{"cost": namespace})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rewritten, `'cost.session_cost'`) || strings.Count(rewritten, "__tailapp_export_session_cost") != 1 {
		t.Fatalf("rewrite = %s", rewritten)
	}
}

func TestQueryHasDeterministicRuntimeBudget(t *testing.T) {
	guardProfile, err := tailapps.Load("agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "guard.sqlite")
	guard, err := projection.Create(context.Background(), path, guardProfile, 0, "reset")
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	frontier, err := guard.Frontier(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := Open(Namespace{Path: path, Profile: guardProfile, Frontier: frontier}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sandbox.Close()

	_, err = sandbox.Query(context.Background(), Request{SQL: `
WITH RECURSIVE sequence(value) AS (
  VALUES(1) UNION ALL SELECT value + 1 FROM sequence WHERE value < 100000000
)
SELECT sum(value) FROM sequence`})
	if err == nil || !strings.Contains(err.Error(), "query_budget_exceeded") {
		t.Fatalf("unbounded query error = %v", err)
	}
}

var stubProfile = func() (result profile.Profile) {
	result.Exports = map[string]profile.Export{"session_cost": {Name: "session_cost"}}
	return result
}()

func delivery(position int64, name string, attributes map[string]any) inbox.Delivery {
	record, _ := json.Marshal(map[string]any{"attributes": attributes, "resource": map[string]any{"attributes": map[string]any{"service.name": "codex"}}})
	time := fmt.Sprintf("%019d", position*100)
	return inbox.Delivery{Position: position, EventID: fmt.Sprintf("local:%d", position), Signal: "log", Name: name, Source: "codex", TimeUnixNano: &time,
		ContentDigest: fmt.Sprintf("sha256:%064d", position), JSON: record}
}
