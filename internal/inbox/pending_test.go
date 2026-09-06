package inbox

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
)

// Use the production schema and pinned Go driver, with the observed backlog
// size. Constant timestamps and shared positions across consumers must not
// affect the delivery order.
func pendingBacklog(t *testing.T, count int) *Queue {
	t.Helper()
	q, err := Open(filepath.Join(t.TempDir(), "control.sqlite"), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { q.Close() })
	_, err = q.db.ExecContext(context.Background(), `WITH RECURSIVE positions(p) AS (
 SELECT 1 UNION ALL SELECT p+1 FROM positions WHERE p<?)
 INSERT INTO inbox_events(position,event_id,signal,name,source,content_digest,record_json,canonical_bytes,received_at)
 SELECT p,'local:'||p,'log','same','synthetic','sha256:test',CAST('{}' AS BLOB),2,123 FROM positions`, count)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO inbox_obligations SELECT position,'alpha',CASE WHEN position%2=0 THEN 'r2' ELSE 'r1' END,'pending',NULL FROM inbox_events`,
		`INSERT INTO inbox_obligations SELECT position,'beta','other','pending',NULL FROM inbox_events`,
	} {
		if _, err := q.db.ExecContext(context.Background(), query); err != nil {
			t.Fatal(err)
		}
	}
	if count >= 87_531 {
		for _, name := range []string{"gamma", "delta", "epsilon", "zeta"} {
			if _, err := q.db.ExecContext(context.Background(), `INSERT INTO inbox_obligations SELECT position,?,'other','pending',NULL FROM inbox_events`, name); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := q.db.ExecContext(context.Background(), `UPDATE inbox_state SET delivery_head=?,queued_records=?,queued_bytes=? WHERE singleton=1`, count, count, count*2); err != nil {
		t.Fatal(err)
	}
	return q
}

type pendingCost struct {
	Steps, Sorts, Rows int
	Elapsed            time.Duration
	Digest             string
}

func measurePending(t *testing.T, q *Queue, query string, limit int) pendingCost {
	t.Helper()
	conn, err := q.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var cost pendingCost
	err = conn.Raw(func(raw any) error {
		stmt, _, err := raw.(driver.Conn).Raw().Prepare(query)
		if err != nil {
			return err
		}
		defer stmt.Close()
		if err = stmt.BindText(1, "alpha"); err != nil {
			return err
		}
		if err = stmt.BindInt(2, limit); err != nil {
			return err
		}
		hash := sha256.New()
		started := time.Now()
		for stmt.Step() {
			for column := 0; column < stmt.ColumnCount(); column++ {
				fmt.Fprintf(hash, "%d:%q;", stmt.ColumnType(column), stmt.ColumnText(column))
			}
			hash.Write([]byte{10})
			cost.Rows++
		}
		cost.Elapsed = time.Since(started)
		cost.Digest = fmt.Sprintf("%x", hash.Sum(nil))
		cost.Steps = stmt.Status(sqlite3.STMTSTATUS_VM_STEP, false)
		cost.Sorts = stmt.Status(sqlite3.STMTSTATUS_SORT, false)
		return stmt.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return cost
}

func TestPendingBacklogUsesBoundedIndexWalk(t *testing.T) {
	q := pendingBacklog(t, 87_531)
	var version string
	if err := q.db.QueryRow(`SELECT sqlite_version()`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	t.Logf("pinned Go SQLite version=%s backlog=87531", version)
	// Historical comparison is diagnostic, not the regression oracle. The
	// production query must avoid a sort and a backlog-sized VM walk itself.
	legacy := strings.Replace(pendingSQL, "ORDER BY o.position", "ORDER BY e.position", 1)
	legacyDigests := map[int]string{}
	for _, item := range []struct{ name, sql string }{{"legacy", legacy}, {"production", pendingSQL}} {
		rows, err := q.db.Query("EXPLAIN QUERY PLAN "+item.sql, "alpha", 1)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			t.Logf("%s plan: %s", item.name, detail)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		for _, limit := range []int{1, 1024} {
			cost := measurePending(t, q, item.sql, limit)
			t.Logf("%s limit=%d rows=%d sort=%d VM_steps=%d elapsed=%s", item.name, limit, cost.Rows, cost.Sorts, cost.Steps, cost.Elapsed)
			if item.name == "legacy" {
				legacyDigests[limit] = cost.Digest
			} else if cost.Digest != legacyDigests[limit] {
				t.Fatalf("changed exact selected rows at limit%d", limit)
			}
			if cost.Rows != limit {
				t.Fatalf("rows=%d want%d", cost.Rows, limit)
			}
			// Wide structural bound, independent of CPU, disk or race instrumentation.
			if item.name == "production" && (cost.Sorts != 0 || cost.Steps > 100*(limit+1)) {
				t.Errorf("next pending scans/sorts backlog: %+v limit=%d", cost, limit)
			}
		}
	}
	deliveries, err := q.Pending(context.Background(), "alpha", 1024)
	if err != nil || len(deliveries) != 1024 {
		t.Fatalf("public Pending: %d %v", len(deliveries), err)
	}
	for i, d := range deliveries {
		if d.Position != int64(i+1) {
			t.Fatalf("position %d = %d", i, d.Position)
		}
	}
}

func TestPendingSparseConsumersRevisionsAndCancellation(t *testing.T) {
	q := pendingBacklog(t, 8)
	ctx := context.Background()
	if err := q.Complete(ctx, "alpha", 1); err != nil {
		t.Fatal(err)
	}
	if err := q.Detach(ctx, "alpha", 3, "test"); err != nil {
		t.Fatal(err)
	}
	if err := q.Complete(ctx, "alpha", 4); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name      string
		limit     int
		positions []int64
		revisions []string
	}{
		{"alpha", 3, []int64{2, 5, 6}, []string{"r2", "r1", "r2"}},
		{"beta", 3, []int64{1, 2, 3}, []string{"other", "other", "other"}},
		{"missing", 1, nil, nil},
	} {
		got, err := q.Pending(ctx, item.name, item.limit)
		if err != nil {
			t.Fatal(err)
		}
		var positions []int64
		var revisions []string
		for _, d := range got {
			positions = append(positions, d.Position)
			revisions = append(revisions, d.Revision)
			if d.EventID != fmt.Sprintf("local:%d", d.Position) || string(d.JSON) != "{}" {
				t.Fatalf("changed content: %+v", d)
			}
		}
		if !reflect.DeepEqual(positions, item.positions) || !reflect.DeepEqual(revisions, item.revisions) {
			t.Fatalf("%s: positions=%v revisions=%v", item.name, positions, revisions)
		}
	}
	for _, limit := range []int{0, -1, 1025} {
		if _, err := q.Pending(ctx, "alpha", limit); err == nil {
			t.Fatalf("accepted limit %d", limit)
		}
	}
	before, err := q.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := q.Pending(cancelled, "alpha", 1); err == nil {
		t.Fatal("cancelled read succeeded")
	}
	after, err := q.Stats(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("read changed queue: before=%+v after=%+v err=%v", before, after, err)
	}
	if _, err := q.DetachAll(ctx, "beta", "done"); err != nil {
		t.Fatal(err)
	}
	after, err = q.Stats(ctx)
	if err != nil || after.Records != 5 || after.PendingObligations != 5 || after.DeliveryHead != 8 {
		t.Fatalf("retention/head: %+v %v", after, err)
	}
}
