package control

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/generalbusiness-ai/tailapps/internal/engine"
	"github.com/generalbusiness-ai/tailapps/internal/profile"
	"github.com/generalbusiness-ai/tailapps/internal/projection"
	"github.com/ncruces/go-sqlite3/driver"
)

// These public definition digests were recorded in the recovery preparation.
// No telemetry or live database is used by this test.
var recoveryRevisions = map[string]string{
	"activity-stats": "sha256:bb951653d838881824a13e41396d957f4f53ea1507d673371d8e0b9e842b6c27",
	"agent-guard":    "sha256:efe35e90119d8410a97f6b109833a019fc908bf3691f4128e9ad12e45d8e6d5b",
	"daily-review":   "sha256:72a8fcbd82c918466a297ee6febd169cb4ecdde398e9220ca706375ced48a37a",
	"session-cost":   "sha256:b2849559fbdefaef388e7917e9ef09e4c639c11508fbbb7a40473e6f14adbf28",
	"signal-counts":  "sha256:1e5c4a7c110e98b6aa9f0e4a149452338301847de75325d6279e379fc413e311",
	"url-reputation": "sha256:8e2d73e6dfc44db5d412066e2ff9da66685f185b064153b08139bbdffef7cae3",
}

const recoveryRuntime = "jsonata-ddl-runtime:sha256:5032bcfe6634db4462fcfe39775e55d1c7e11fcb7663c961f6ebe8860192b8b1"

func TestRecoverySixApplicationBacklogPreservesIdentityAndRestart(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resident, err := engine.Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if resident != nil {
			resident.Close()
		}
	}()
	for name, revision := range recoveryRevisions {
		sources := map[string][]byte{}
		root := filepath.Join("testdata", "recovery-definitions", name)
		if err := fs.WalkDir(os.DirFS(root), ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			content, err := os.ReadFile(filepath.Join(root, path))
			if err != nil {
				return err
			}
			sources[path] = content
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		result, err := resident.Install(ctx, name, "", sources)
		if err != nil {
			t.Fatal(err)
		}
		if result.Profile.Revision != revision || result.Profile.RuntimeProfile != recoveryRuntime {
			t.Fatalf("%s changed deployed identity: revision=%s runtime=%s", name, result.Profile.Revision, result.Profile.RuntimeProfile)
		}
	}
	if err := resident.Close(); err != nil {
		t.Fatal(err)
	}
	resident = nil
	db, err := driver.Open("file:" + filepath.Join(home, "control.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const count = 87_531
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `WITH RECURSIVE positions(p) AS (
 SELECT 1 UNION ALL SELECT p+1 FROM positions WHERE p<?)
 INSERT INTO inbox_events(position,event_id,signal,name,source,content_digest,record_json,canonical_bytes,received_at)
 SELECT p,'local:'||p,'log','synthetic','synthetic','sha256:fixture',
 CAST('{"name":"synthetic","padding":"'||printf('%02064d',0)||'"}' AS BLOB),2097,123 FROM positions`, count)
	if err != nil {
		t.Fatal(err)
	}
	for name, revision := range recoveryRevisions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO inbox_obligations SELECT position,?,?,'pending',NULL FROM inbox_events`, name, revision); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inbox_events SET canonical_bytes=length(record_json)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inbox_state SET delivery_head=?,queued_records=?,queued_bytes=(SELECT SUM(canonical_bytes) FROM inbox_events) WHERE singleton=1`, count, count); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	db.Close()
	definitions := recoveryRows(t, home, "control.sqlite", `SELECT name,draft_revision,active_revision,runtime_profile,activation_mode,boundary_position FROM definition_tailapps ORDER BY name`)
	previous := map[string]int64{}
	// Each cycle closes the real worker, validates its stopped durable state,
	// then opens that same home. This proves restart preservation under backlog.
	for cycle := 0; cycle < 2; cycle++ {
		resident, err = engine.Open(ctx, home)
		if err != nil {
			t.Fatal(err)
		}
		socketDir, err := os.MkdirTemp("", "ta-recovery-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(socketDir)
		socket := filepath.Join(socketDir, "control.sock")
		listener, err := Listen(socket)
		if err != nil {
			t.Fatal(err)
		}
		server := &http.Server{Handler: &Server{Engine: resident}}
		defer server.Close()
		go server.Serve(listener)
		client := NewClient(socket)
		defer client.http.CloseIdleConnections()
		for round := 0; round < 3; round++ {
			start := time.Now()
			var status engine.Status
			if err := client.Call(ctx, "status", nil, &status); err != nil {
				t.Fatal(err)
			}
			if status.Profile != recoveryRuntime || !status.IngestionReady || len(status.Unavailable) != 0 || len(status.Apps) != 6 || status.Inbox.DeliveryHead != count {
				t.Fatalf("identity/availability: %+v", status)
			}
			var pending int64
			minimum := int64(count)
			for name, revision := range recoveryRevisions {
				f := status.Apps[name]
				if f.Revision != revision || !f.Complete || f.GapPosition != nil || f.InterpretedPosition <= previous[name] {
					t.Fatalf("%s lost progress: %+v previous=%d", name, f, previous[name])
				}
				previous[name] = f.InterpretedPosition
				pending += count - f.InterpretedPosition
				if f.InterpretedPosition < minimum {
					minimum = f.InterpretedPosition
				}
			}
			if status.Inbox.PendingObligations != pending || status.Inbox.Records != count-minimum || status.Inbox.Records <= 0 {
				t.Fatalf("queue/frontier conservation: %+v pending=%d minimum=%d", status, pending, minimum)
			}
			t.Logf("cycle=%d round=%d status=%s records=%d pending=%d frontiers=%v", cycle, round, time.Since(start), status.Inbox.Records, pending, previous)
			for name, revision := range recoveryRevisions {
				var schema profile.Profile
				start = time.Now()
				if err := client.Call(ctx, "schema", NameArgs{Name: name}, &schema); err != nil {
					t.Fatal(err)
				}
				if schema.Revision != revision || schema.RuntimeProfile != recoveryRuntime {
					t.Fatalf("%s schema identity changed", name)
				}
				t.Logf("%s schema=%s", name, time.Since(start))
			}
			var metrics engine.MetricsSnapshot
			start = time.Now()
			if err := client.Call(ctx, "metrics", nil, &metrics); err != nil {
				t.Fatal(err)
			}
			if metrics.UpgradePendingTailapps != 0 || metrics.UnavailableTailapps != 0 || metrics.ActiveTailapps != 6 {
				t.Fatalf("runtime gating: %+v", metrics)
			}
			t.Logf("metrics=%s", time.Since(start))
		}
		client.http.CloseIdleConnections()
		server.Close()
		os.RemoveAll(socketDir)
		if err := resident.Close(); err != nil {
			t.Fatal(err)
		}
		resident = nil
		if got := recoveryRows(t, home, "control.sqlite", `SELECT name,draft_revision,active_revision,runtime_profile,activation_mode,boundary_position FROM definition_tailapps ORDER BY name`); !reflect.DeepEqual(got, definitions) {
			t.Fatal("definitions changed across drain/restart")
		}
		for name, revision := range recoveryRevisions {
			projectionPath := filepath.Join("projections", name, "state.sqlite")
			id, err := projection.InspectIdentity(ctx, filepath.Join(home, projectionPath))
			if err != nil {
				t.Fatal(err)
			}
			if id.Name != name || id.Revision != revision || id.Runtime != recoveryRuntime {
				t.Fatalf("changed physical identity: %+v", id)
			}
			db, err := driver.Open("file:" + filepath.Join(home, projectionPath))
			if err != nil {
				t.Fatal(err)
			}
			var frontier, consumed int64
			if err := db.QueryRow(`SELECT interpreted_position FROM tailapp_frontier`).Scan(&frontier); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT consumed_records FROM tailapp_stats`).Scan(&consumed); err != nil {
				t.Fatal(err)
			}
			db.Close()
			if frontier < previous[name] || consumed != frontier {
				t.Fatalf("%s duplicate/reset: frontier=%d consumed=%d previous=%d", name, frontier, consumed, previous[name])
			}
			previous[name] = frontier
			db, err = driver.Open("file:" + filepath.Join(home, "control.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			var pending, bad int64
			if err := db.QueryRow(`SELECT COUNT(*) FROM inbox_obligations WHERE tailapp=? AND state='pending'`, name).Scan(&pending); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT COUNT(*) FROM inbox_obligations WHERE tailapp=? AND (revision<>? OR state='detached' OR (state='pending' AND position<=?) OR (state<>'pending' AND position>?))`, name, revision, frontier, frontier).Scan(&bad); err != nil {
				t.Fatal(err)
			}
			db.Close()
			if pending != count-frontier || bad != 0 {
				t.Fatalf("%s lost/duplicated/detached obligation: frontier=%d pending=%d bad=%d", name, frontier, pending, bad)
			}
			t.Logf("stopped cycle=%d %s revision=%s frontier=%d consumed=%d pending=%d", cycle, name, revision, frontier, consumed, pending)
		}
	}
}

func recoveryRows(t *testing.T, home, path, query string) [][]string {
	t.Helper()
	db, err := driver.Open("file:" + filepath.Join(home, path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var result [][]string
	for rows.Next() {
		values := make([]sql.NullString, len(columns))
		dest := make([]any, len(columns))
		for i := range dest {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			t.Fatal(err)
		}
		v := make([]string, len(values))
		for i, x := range values {
			v[i] = fmt.Sprintf("%t:%s", x.Valid, x.String)
		}
		result = append(result, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}
