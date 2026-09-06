package control

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/generalbusiness-ai/tailapps/internal/engine"
	"github.com/generalbusiness-ai/tailapps/internal/profile"
	"github.com/ncruces/go-sqlite3/driver"
)

// Exercise the same Unix-socket control client as CLI/MCP while the real
// resident worker drains. The VM-step regression in inbox supplies the
// deterministic complexity gate; the ordinary client timeout here is only an
// integration liveness check, not a microbenchmark threshold.
func TestControlReadsWhileLargeBacklogDrains(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	resident, err := engine.Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := resident.Install(ctx, "session-cost", "session-cost", nil)
	if err != nil {
		resident.Close()
		t.Fatal(err)
	}
	if err := resident.Close(); err != nil {
		t.Fatal(err)
	}
	// Synthetic records only. Seed a stopped disposable resident using its
	// already initialized schema, exact active revision and pinned Go driver.
	db, err := driver.Open("file:" + filepath.Join(home, "control.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	const count = 72_512
	_, err = tx.ExecContext(ctx, `WITH RECURSIVE positions(p) AS (
 SELECT 1 UNION ALL SELECT p+1 FROM positions WHERE p<?)
 INSERT INTO inbox_events(position,event_id,signal,name,source,content_digest,record_json,canonical_bytes,received_at)
 SELECT p,'local:'||p,'log','synthetic','synthetic','sha256:fixture',
 CAST('{"name":"synthetic","padding":"'||printf('%02064d',0)||'"}' AS BLOB),2097,123 FROM positions`, count)
	if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO inbox_obligations
 SELECT position,'session-cost',?,'pending',NULL FROM inbox_events`, installed.Profile.Revision)
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE inbox_events SET canonical_bytes=length(record_json)`)
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE inbox_state SET delivery_head=?,queued_records=?,queued_bytes=(SELECT SUM(canonical_bytes) FROM inbox_events) WHERE singleton=1`, count, count)
	}
	if err != nil {
		tx.Rollback()
		db.Close()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	resident, err = engine.Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer resident.Close()
	// Darwin's Unix socket path limit is shorter than some testing temp paths.
	socketDir, err := os.MkdirTemp("", "ta-backlog-")
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
	var previous int64
	for round := 0; round < 3; round++ {
		for _, op := range []string{"status", "schema", "metrics"} {
			started := time.Now()
			var result any
			var args any
			var status engine.Status
			var metrics engine.MetricsSnapshot
			var schema profile.Profile
			switch op {
			case "status":
				result = &status
			case "metrics":
				result = &metrics
			case "schema":
				result = &schema
				args = NameArgs{Name: "session-cost"}
			}
			if err := client.Call(ctx, op, args, result); err != nil {
				t.Fatalf("%s while draining: %v", op, err)
			}
			t.Logf("round=%d %s elapsed=%s", round, op, time.Since(started))
			if op == "schema" && schema.Revision != installed.Profile.Revision {
				t.Fatal("active revision changed")
			}
			if op == "status" {
				frontier := status.Apps["session-cost"]
				if !status.IngestionReady || len(status.Unavailable) != 0 || frontier.GapPosition != nil || frontier.Revision != installed.Profile.Revision {
					t.Fatalf("changed runtime/frontier: %+v", status)
				}
				if status.Inbox.Records <= 0 || status.Inbox.Records >= count || frontier.InterpretedPosition <= previous || frontier.InterpretedPosition+status.Inbox.Records != count || status.Inbox.PendingObligations != status.Inbox.Records || status.Inbox.DeliveryHead != count {
					t.Fatalf("not draining intact prefix: %+v previous=%d", status, previous)
				}
				previous = frontier.InterpretedPosition
				t.Logf("frontier=%d remaining=%d bytes=%d", previous, status.Inbox.Records, status.Inbox.CanonicalBytes)
			}
		}
	}
}
