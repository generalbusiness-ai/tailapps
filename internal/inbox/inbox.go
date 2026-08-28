// Package inbox owns Tailapp's bounded durable delivery queue. The queue is
// operational state: event content disappears as soon as every consumer
// captured at acceptance has consumed or detached from the record.
package inbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/ncruces/go-sqlite3"
	sqlitedriver "github.com/ncruces/go-sqlite3/driver"
)

var ErrFull = errors.New("inbox capacity exceeded")

type Limits struct {
	Records int64
	Bytes   int64
}

func (l Limits) normalized() Limits {
	if l.Records <= 0 {
		l.Records = 100_000
	}
	if l.Bytes <= 0 {
		l.Bytes = 256 << 20
	}
	return l
}

type Consumer struct {
	Tailapp  string
	Revision string
}

type Record struct {
	Signal           string
	Name             string
	Source           string
	TimeUnixNano     *string
	ObservedUnixNano *string
	TraceID          *string
	SpanID           *string
	ContentDigest    string
	JSON             []byte
	ReceivedAt       time.Time
}

type Delivery struct {
	Position         int64
	EventID          string
	Signal           string
	Name             string
	Source           string
	TimeUnixNano     *string
	ObservedUnixNano *string
	TraceID          *string
	SpanID           *string
	ContentDigest    string
	JSON             []byte
	Revision         string
}

type Stats struct {
	Records            int64
	CanonicalBytes     int64
	DeliveryHead       int64
	OldestPosition     *int64
	NewestPosition     *int64
	PendingObligations int64
}

type Queue struct {
	db     *sql.DB
	limits Limits
}

func Open(path string, limits Limits) (*Queue, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create inbox directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("control database may not be a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect control database: %w", err)
	}
	dsn := (&url.URL{Scheme: "file", Path: path}).String()
	database, err := sqlitedriver.Open(dsn, func(connection *sqlite3.Conn) error {
		if _, err := connection.Config(sqlite3.DBCONFIG_DEFENSIVE, true); err != nil {
			return err
		}
		if _, err := connection.Config(sqlite3.DBCONFIG_TRUSTED_SCHEMA, false); err != nil {
			return err
		}
		if _, err := connection.Config(sqlite3.DBCONFIG_ENABLE_LOAD_EXTENSION, false); err != nil {
			return err
		}
		if err := connection.Exec("PRAGMA foreign_keys=ON"); err != nil {
			return err
		}
		return connection.Exec("PRAGMA trusted_schema=OFF")
	})
	if err != nil {
		return nil, fmt.Errorf("open control database: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	queue := &Queue{db: database, limits: limits.normalized()}
	if err := queue.initialize(context.Background()); err != nil {
		database.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		database.Close()
		return nil, fmt.Errorf("protect control database: %w", err)
	}
	return queue, nil
}

func (q *Queue) initialize(ctx context.Context) error {
	const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
PRAGMA trusted_schema=OFF;
CREATE TABLE IF NOT EXISTS inbox_events (
  position INTEGER PRIMARY KEY,
  event_id TEXT NOT NULL UNIQUE,
  signal TEXT NOT NULL CHECK (signal IN ('log', 'span', 'metric')),
  name TEXT NOT NULL,
  source TEXT NOT NULL,
  time_unix_nano TEXT,
  observed_unix_nano TEXT,
  trace_id TEXT,
  span_id TEXT,
  content_digest TEXT NOT NULL,
  record_json BLOB NOT NULL,
  canonical_bytes INTEGER NOT NULL CHECK (canonical_bytes >= 0),
  received_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS inbox_state (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  delivery_head INTEGER NOT NULL CHECK (delivery_head >= 0),
  queued_records INTEGER NOT NULL CHECK (queued_records >= 0),
  queued_bytes INTEGER NOT NULL CHECK (queued_bytes >= 0)
);
INSERT INTO inbox_state(singleton, delivery_head, queued_records, queued_bytes) VALUES (1, 0, 0, 0)
  ON CONFLICT(singleton) DO NOTHING;
CREATE TABLE IF NOT EXISTS inbox_obligations (
  position INTEGER NOT NULL REFERENCES inbox_events(position) ON DELETE CASCADE,
  tailapp TEXT NOT NULL,
  revision TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('pending', 'consumed', 'detached')),
  error_code TEXT,
  PRIMARY KEY (position, tailapp)
);
CREATE INDEX IF NOT EXISTS inbox_obligations_outstanding
  ON inbox_obligations(tailapp, state, position);
CREATE INDEX IF NOT EXISTS inbox_obligations_position
  ON inbox_obligations(position, state);
`
	if _, err := q.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("initialize inbox: %w", err)
	}
	return nil
}

func (q *Queue) Close() error { return q.db.Close() }

// Enqueue atomically admits one decoded OTLP request and captures its active
// consumer set. It returns the assigned positions in request order.
func (q *Queue) Enqueue(ctx context.Context, records []Record, consumers []Consumer) ([]int64, error) {
	if len(records) == 0 {
		return nil, nil
	}
	var incomingBytes int64
	for index := range records {
		if len(records[index].JSON) == 0 {
			return nil, fmt.Errorf("record %d has empty canonical JSON", index)
		}
		incomingBytes += int64(len(records[index].JSON))
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var count, bytes, maximum int64
	if err := tx.QueryRowContext(ctx, `SELECT delivery_head,queued_records,queued_bytes FROM inbox_state WHERE singleton=1`).Scan(&maximum, &count, &bytes); err != nil {
		return nil, err
	}
	if count+int64(len(records)) > q.limits.Records || bytes+incomingBytes > q.limits.Bytes {
		return nil, ErrFull
	}
	positions := make([]int64, len(records))
	for index, record := range records {
		position := maximum + int64(index) + 1
		eventID := fmt.Sprintf("local:%d", position)
		received := record.ReceivedAt
		if received.IsZero() {
			received = time.Now()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO inbox_events
 (position,event_id,signal,name,source,time_unix_nano,observed_unix_nano,trace_id,span_id,content_digest,record_json,canonical_bytes,received_at)
 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, position, eventID, record.Signal, record.Name, record.Source,
			record.TimeUnixNano, record.ObservedUnixNano, record.TraceID, record.SpanID,
			record.ContentDigest, record.JSON, len(record.JSON), received.UnixNano()); err != nil {
			return nil, fmt.Errorf("insert inbox record %d: %w", index, err)
		}
		for _, consumer := range consumers {
			if consumer.Tailapp == "" || consumer.Revision == "" {
				return nil, errors.New("consumer tailapp and revision are required")
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO inbox_obligations(position,tailapp,revision,state) VALUES (?,?,?,'pending')`, position, consumer.Tailapp, consumer.Revision); err != nil {
				return nil, fmt.Errorf("capture consumer %q: %w", consumer.Tailapp, err)
			}
		}
		positions[index] = position
	}
	retainedRecords, retainedBytes := int64(len(records)), incomingBytes
	if len(consumers) == 0 {
		retainedRecords, retainedBytes = 0, 0
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inbox_state SET delivery_head=?,queued_records=queued_records+?,queued_bytes=queued_bytes+? WHERE singleton=1`, positions[len(positions)-1], retainedRecords, retainedBytes); err != nil {
		return nil, err
	}
	if len(consumers) == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM inbox_events WHERE position >= ? AND position <= ?`, positions[0], positions[len(positions)-1]); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return positions, nil
}

// Pending returns the oldest unsettled deliveries for one tailapp.
func (q *Queue) Pending(ctx context.Context, tailapp string, limit int) ([]Delivery, error) {
	if limit <= 0 || limit > 1024 {
		return nil, errors.New("pending limit must be between 1 and 1024")
	}
	rows, err := q.db.QueryContext(ctx, `SELECT e.position,e.event_id,e.signal,e.name,e.source,e.time_unix_nano,
 e.observed_unix_nano,e.trace_id,e.span_id,e.content_digest,e.record_json,o.revision
 FROM inbox_obligations o JOIN inbox_events e ON e.position=o.position
 WHERE o.tailapp=? AND o.state='pending' ORDER BY e.position LIMIT ?`, tailapp, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deliveries []Delivery
	for rows.Next() {
		var delivery Delivery
		var eventTime, observed, traceID, spanID sql.NullString
		if err := rows.Scan(&delivery.Position, &delivery.EventID, &delivery.Signal, &delivery.Name, &delivery.Source,
			&eventTime, &observed, &traceID, &spanID, &delivery.ContentDigest, &delivery.JSON, &delivery.Revision); err != nil {
			return nil, err
		}
		delivery.TimeUnixNano = nullableString(eventTime)
		delivery.ObservedUnixNano = nullableString(observed)
		delivery.TraceID = nullableString(traceID)
		delivery.SpanID = nullableString(spanID)
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func (q *Queue) Complete(ctx context.Context, tailapp string, position int64) error {
	return q.settle(ctx, tailapp, position, "consumed", "")
}

func (q *Queue) Detach(ctx context.Context, tailapp string, position int64, code string) error {
	return q.settle(ctx, tailapp, position, "detached", code)
}

func (q *Queue) settle(ctx context.Context, tailapp string, position int64, state, code string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE inbox_obligations SET state=?, error_code=NULLIF(?, '') WHERE position=? AND tailapp=? AND state='pending'`, state, code, position, tailapp)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		var current string
		if err := tx.QueryRowContext(ctx, `SELECT state FROM inbox_obligations WHERE position=? AND tailapp=?`, position, tailapp).Scan(&current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return tx.Commit()
			}
			return fmt.Errorf("obligation not pending: %w", err)
		}
		if current != state {
			return fmt.Errorf("obligation already %s", current)
		}
	}
	var removedBytes int64
	err = tx.QueryRowContext(ctx, `DELETE FROM inbox_events WHERE position=? AND NOT EXISTS (
 SELECT 1 FROM inbox_obligations WHERE position=? AND state='pending') RETURNING canonical_bytes`, position, position).Scan(&removedBytes)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		if _, err := tx.ExecContext(ctx, `UPDATE inbox_state SET queued_records=queued_records-1,queued_bytes=queued_bytes-? WHERE singleton=1`, removedBytes); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DetachAll relinquishes every pending obligation for one unavailable app and
// deletes records whose remaining consumers have all settled.
func (q *Queue) DetachAll(ctx context.Context, tailapp, code string) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE inbox_obligations SET state='detached', error_code=? WHERE tailapp=? AND state='pending'`, code, tailapp); err != nil {
		return err
	}
	var removedRecords, removedBytes int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(canonical_bytes),0) FROM inbox_events WHERE NOT EXISTS (
 SELECT 1 FROM inbox_obligations WHERE inbox_obligations.position=inbox_events.position AND state='pending')`).Scan(&removedRecords, &removedBytes); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM inbox_events WHERE NOT EXISTS (
 SELECT 1 FROM inbox_obligations WHERE inbox_obligations.position=inbox_events.position AND state='pending')`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inbox_state SET queued_records=queued_records-?,queued_bytes=queued_bytes-? WHERE singleton=1`, removedRecords, removedBytes); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Queue) Stats(ctx context.Context) (Stats, error) {
	var result Stats
	var oldest, newest sql.NullInt64
	if err := q.db.QueryRowContext(ctx, `SELECT queued_records,queued_bytes,delivery_head FROM inbox_state WHERE singleton=1`).Scan(
		&result.Records, &result.CanonicalBytes, &result.DeliveryHead); err != nil {
		return Stats{}, err
	}
	if err := q.db.QueryRowContext(ctx, `SELECT MIN(position),MAX(position) FROM inbox_events`).Scan(&oldest, &newest); err != nil {
		return Stats{}, err
	}
	if oldest.Valid {
		result.OldestPosition = &oldest.Int64
	}
	if newest.Valid {
		result.NewestPosition = &newest.Int64
	}
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox_obligations WHERE state='pending'`).Scan(&result.PendingObligations); err != nil {
		return Stats{}, err
	}
	return result, nil
}
