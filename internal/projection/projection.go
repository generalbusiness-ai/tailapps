// Package projection materializes one active Tailapp profile into its private
// durable SQLite database. One delivery is one transaction: normalizer reads,
// normalizer writes and emissions, then every analytic fold over each emitted
// event, followed by the exact frontier update.
package projection

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ncruces/go-sqlite3"
	sqlitedriver "github.com/ncruces/go-sqlite3/driver"

	"github.com/generalbusiness-ai/tailapps/internal/inbox"
	"github.com/generalbusiness-ai/tailapps/internal/profile"

	"github.com/generalbusiness-ai/tailapps/jsonataddl"
)

type Frontier struct {
	Revision            string  `json:"revision"`
	ActivationBoundary  int64   `json:"activation_boundary"`
	InterpretedPosition int64   `json:"interpreted_position"`
	LastEventID         string  `json:"last_event_id"`
	Complete            bool    `json:"complete"`
	GapPosition         *int64  `json:"gap_position,omitempty"`
	GapReason           *string `json:"gap_reason,omitempty"`
	GapObservedUnixNano *string `json:"gap_observed_unix_nano,omitempty"`
	LastRecordUnixNano  *string `json:"last_record_time_unix_nano,omitempty"`
}

type Result struct {
	AlreadyApplied bool
	Frontier       Frontier
	Ineffective    bool
	EmittedEvents  int
}

type Stats struct {
	ConsumedRecords    int64 `json:"consumed_records"`
	IneffectiveRecords int64 `json:"ineffective_records"`
	EmittedEvents      int64 `json:"emitted_events"`
}

type Identity struct {
	Name     string
	Revision string
	Runtime  string
}

type Projection struct {
	name    string
	path    string
	profile *profile.Profile
	db      *sql.DB
	guard   *readGuard
	mu      sync.Mutex
}

// readGuard is the connection-level authorizer seat. The connection is
// opened with its callback installed once; host operations (schema,
// mutations, metadata) run with no deny function seated and are
// unrestricted, while executing a program's compiled read plan seats that
// program's default-deny core authorizer for the duration of the reads.
type readGuard struct {
	deny atomic.Pointer[jsonataddl.Authorizer]
}

func (guard *readGuard) callback(action sqlite3.AuthorizerActionCode, name3rd, name4th, schema, inner string) sqlite3.AuthorizerReturnCode {
	if seated := guard.deny.Load(); seated != nil {
		return (*seated)(action, name3rd, name4th, schema, inner)
	}
	return sqlite3.AUTH_OK
}

func (guard *readGuard) seat(authorizer jsonataddl.Authorizer) { guard.deny.Store(&authorizer) }
func (guard *readGuard) release()                              { guard.deny.Store(nil) }

func Create(ctx context.Context, path string, compiled *profile.Profile, activationBoundary int64, mode string) (*Projection, error) {
	if compiled == nil {
		return nil, errors.New("compiled profile is required")
	}
	if mode != "reset" && mode != "continue" {
		return nil, errors.New("activation mode must be reset or continue")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create projection directory: %w", err)
	}
	if _, err := os.Lstat(path); err == nil {
		return nil, errors.New("projection database already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect projection path: %w", err)
	}
	guard := &readGuard{}
	database, err := openDatabase(path, guard)
	if err != nil {
		return nil, err
	}
	projection := &Projection{name: compiled.Name, path: path, profile: compiled, db: database, guard: guard}
	if err := projection.initialize(ctx, activationBoundary, mode); err != nil {
		database.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		database.Close()
		return nil, fmt.Errorf("protect projection database: %w", err)
	}
	return projection, nil
}

func Open(ctx context.Context, path string, compiled *profile.Profile) (*Projection, error) {
	return openExisting(ctx, path, compiled, compiled.Revision, false)
}

// OpenForUpgrade opens an identity-matching old projection for query and
// acknowledged reset only. Its physical identity remains in the database:
// a profile recompiled by the current core does not relabel the stored runtime.
func OpenForUpgrade(ctx context.Context, path string, compiled *profile.Profile, expectedRevision string) (*Projection, error) {
	return openExisting(ctx, path, compiled, expectedRevision, true)
}

func InspectIdentity(ctx context.Context, path string) (Identity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Identity{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Identity{}, errors.New("projection database may not be a symlink")
	}
	database, err := openDatabase(path, &readGuard{})
	if err != nil {
		return Identity{}, err
	}
	defer database.Close()
	var result Identity
	err = database.QueryRowContext(ctx, `SELECT tailapp,revision,runtime_profile FROM tailapp_projection_identity WHERE singleton=1`).Scan(&result.Name, &result.Revision, &result.Runtime)
	return result, err
}

func openExisting(ctx context.Context, path string, compiled *profile.Profile, expectedRevision string, allowLegacyRuntime bool) (*Projection, error) {
	if compiled == nil {
		return nil, errors.New("compiled profile is required")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("projection database may not be a symlink")
	}
	guard := &readGuard{}
	database, err := openDatabase(path, guard)
	if err != nil {
		return nil, err
	}
	projection := &Projection{name: compiled.Name, path: path, profile: compiled, db: database, guard: guard}
	var name, revision, runtime string
	if err := database.QueryRowContext(ctx, `SELECT tailapp,revision,runtime_profile FROM tailapp_projection_identity WHERE singleton=1`).Scan(&name, &revision, &runtime); err != nil {
		database.Close()
		return nil, fmt.Errorf("read projection identity: %w", err)
	}
	if name != compiled.Name || revision != expectedRevision || (!allowLegacyRuntime && runtime != compiled.RuntimeProfile) {
		database.Close()
		return nil, errors.New("projection identity does not match compiled profile")
	}
	if err := ensureFrontierTimestampColumns(ctx, database); err != nil {
		database.Close()
		return nil, err
	}
	return projection, nil
}

func ensureFrontierTimestampColumns(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, `PRAGMA table_info(tailapp_frontier)`)
	if err != nil {
		return fmt.Errorf("inspect projection frontier schema: %w", err)
	}
	present := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		present[name] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, column := range []string{"gap_observed_unix_nano", "last_record_time_unix_nano"} {
		if present[column] {
			continue
		}
		if _, err := database.ExecContext(ctx, `ALTER TABLE tailapp_frontier ADD COLUMN `+quote(column)+` TEXT`); err != nil {
			return fmt.Errorf("add projection frontier column %q: %w", column, err)
		}
	}
	return nil
}

func openDatabase(path string, guard *readGuard) (*sql.DB, error) {
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
		return connection.SetAuthorizer(guard.callback)
	})
	if err != nil {
		return nil, fmt.Errorf("open projection database: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	if _, err := database.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA trusted_schema=OFF;`); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}

func (p *Projection) initialize(ctx context.Context, boundary int64, mode string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range append(append([]string(nil), p.profile.SchemaSQL...), p.profile.ReplaceableSQL...) {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create projection schema: %w", err)
		}
	}
	for _, name := range sortedKeys(p.profile.Exports) {
		exported := p.profile.Exports[name]
		statement := `CREATE VIEW ` + quote("__tailapp_export_"+name) + ` AS ` + exported.SQL
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create query export %q: %w", name, err)
		}
	}
	platformSchema := []string{`CREATE TABLE tailapp_projection_identity (
 singleton INTEGER PRIMARY KEY CHECK (singleton=1), tailapp TEXT NOT NULL,
 revision TEXT NOT NULL, runtime_profile TEXT NOT NULL, activation_mode TEXT NOT NULL
)`, `CREATE TABLE tailapp_frontier (
 singleton INTEGER PRIMARY KEY CHECK (singleton=1), activation_boundary INTEGER NOT NULL,
 interpreted_position INTEGER NOT NULL, last_event_id TEXT NOT NULL,
 complete INTEGER NOT NULL, gap_position INTEGER, gap_reason TEXT,
 gap_observed_unix_nano TEXT, last_record_time_unix_nano TEXT
)`, `CREATE TABLE tailapp_stats (
 singleton INTEGER PRIMARY KEY CHECK (singleton=1), consumed_records INTEGER NOT NULL,
 ineffective_records INTEGER NOT NULL, emitted_events INTEGER NOT NULL
)`}
	for _, statement := range platformSchema {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create projection metadata: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tailapp_projection_identity VALUES (1,?,?,?,?)`, p.name, p.profile.Revision, p.profile.RuntimeProfile, mode); err != nil {
		return fmt.Errorf("create projection identity: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tailapp_frontier VALUES (1,?,?,'',1,NULL,NULL,NULL,NULL)`, boundary, boundary); err != nil {
		return fmt.Errorf("create projection frontier: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tailapp_stats VALUES (1,0,0,0)`); err != nil {
		return fmt.Errorf("create projection stats: %w", err)
	}
	return tx.Commit()
}

func (p *Projection) Close() error              { return p.db.Close() }
func (p *Projection) Database() *sql.DB         { return p.db }
func (p *Projection) Profile() *profile.Profile { return p.profile }
func (p *Projection) Path() string              { return p.path }

// StoredRuntime reads physical identity, including after OpenForUpgrade has
// opened the database with a profile compiled under a different runtime.
func (p *Projection) StoredRuntime(ctx context.Context) (string, error) {
	var runtime string
	if err := p.db.QueryRowContext(ctx, `SELECT runtime_profile FROM tailapp_projection_identity WHERE singleton=1`).Scan(&runtime); err != nil {
		return "", fmt.Errorf("read stored runtime: %w", err)
	}
	return runtime, nil
}

// Continue atomically preserves existing writable tables, adds new tables,
// and replaces derived schema objects within the same runtime at the
// already-drained activation boundary.
func (p *Projection) Continue(ctx context.Context, next *profile.Profile, boundary int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := profile.ContinueCompatible(p.profile, next); err != nil {
		return err
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var storedRuntime string
	if err := tx.QueryRowContext(ctx, `SELECT runtime_profile FROM tailapp_projection_identity WHERE singleton=1`).Scan(&storedRuntime); err != nil {
		return fmt.Errorf("read stored runtime: %w", err)
	}
	if storedRuntime != next.RuntimeProfile {
		return errors.New("stored runtime profile changed; acknowledged reset is required")
	}
	if err := p.checkJSONStorage(ctx, tx); err != nil {
		return err
	}
	for name := range next.Tables {
		if _, exists := p.profile.Tables[name]; exists {
			continue
		}
		if _, err := tx.ExecContext(ctx, next.Tables[name].SQL); err != nil {
			return fmt.Errorf("create additive table %q: %w", name, err)
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT type,name FROM sqlite_master WHERE (type='view' OR type='index') AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return err
	}
	var objects [][2]string
	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			rows.Close()
			return err
		}
		objects = append(objects, [2]string{kind, name})
	}
	rows.Close()
	for _, object := range objects {
		keyword := "VIEW"
		if object[0] == "index" {
			keyword = "INDEX"
		}
		if _, err := tx.ExecContext(ctx, `DROP `+keyword+` `+quote(object[1])); err != nil {
			return err
		}
	}
	for _, statement := range next.ReplaceableSQL {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("replace projection schema: %w", err)
		}
	}
	for _, name := range sortedKeys(next.Exports) {
		if _, err := tx.ExecContext(ctx, `CREATE VIEW `+quote("__tailapp_export_"+name)+` AS `+next.Exports[name].SQL); err != nil {
			return fmt.Errorf("replace query export %q: %w", name, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tailapp_projection_identity SET revision=?,runtime_profile=?,activation_mode='continue' WHERE singleton=1`, next.Revision, next.RuntimeProfile); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tailapp_frontier SET activation_boundary=?,interpreted_position=?,last_event_id='',complete=1,gap_position=NULL,gap_reason=NULL,gap_observed_unix_nano=NULL,last_record_time_unix_nano=NULL WHERE singleton=1`, boundary, boundary); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	p.profile = next
	return nil
}

// Recovery can compile old source using the current core. That profile
// describes the intended new schema, not the physical database it opened.
// Inspect actual columns under the continuation transaction before writes.
func (p *Projection) checkJSONStorage(ctx context.Context, tx *sql.Tx) error {
	for _, name := range sortedKeys(p.profile.Tables) {
		for _, column := range p.profile.Tables[name].Columns {
			if column.Type != profile.TypeJSON {
				continue
			}
			var declared string
			if err := tx.QueryRowContext(ctx, `SELECT type FROM pragma_table_info(?) WHERE name=? COLLATE NOCASE`, name, column.Name).Scan(&declared); err != nil {
				return fmt.Errorf("inspect JSON storage %s.%s: %w", name, column.Name, err)
			}
			if !strings.EqualFold(declared, jsonataddl.SQLiteJSONType) {
				return fmt.Errorf("JSON storage %s.%s is %q; continuation requires %s and cannot migrate existing values", name, column.Name, declared, jsonataddl.SQLiteJSONType)
			}
		}
	}
	return nil
}

func (p *Projection) Process(ctx context.Context, delivery inbox.Delivery) (result Result, processErr error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	frontier, err := p.Frontier(ctx)
	if err != nil {
		return Result{}, err
	}
	if frontier.GapPosition != nil {
		return Result{}, errors.New("projection is gapped")
	}
	if delivery.Position <= frontier.InterpretedPosition {
		if delivery.EventID == frontier.LastEventID || delivery.Position <= frontier.ActivationBoundary {
			return Result{AlreadyApplied: true, Frontier: frontier}, nil
		}
		return Result{}, errors.New("delivery position precedes projection frontier")
	}
	if delivery.Position != frontier.InterpretedPosition+1 {
		return Result{}, fmt.Errorf("delivery position %d is not next after %d", delivery.Position, frontier.InterpretedPosition)
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		_ = tx.Rollback()
		if processErr != nil && !transientProcessError(ctx, processErr) {
			_ = p.recordGap(context.Background(), delivery.Position, processErr.Error())
		}
	}()
	sourceEvent, err := deliveryEvent(delivery)
	if err != nil {
		return Result{}, err
	}
	normalizerInput, err := p.evaluationInput(ctx, tx, p.profile.Normalizer, delivery.Position, delivery.EventID, "otlp_record", sourceEvent)
	if err != nil {
		return Result{}, err
	}
	normalized, err := p.profile.Evaluate(p.profile.Normalizer.Name, normalizerInput)
	if err != nil {
		return Result{}, err
	}
	if err := p.applyChanges(ctx, tx, normalized.Tables); err != nil {
		return Result{}, err
	}
	emitted := normalized.Events["otel_event"]
	for ordinal, event := range emitted {
		for _, fold := range p.profile.Folds {
			eventID := fmt.Sprintf("%s#%d", delivery.EventID, ordinal)
			input, err := p.evaluationInput(ctx, tx, fold, delivery.Position, eventID, "otel_event", event)
			if err != nil {
				return Result{}, err
			}
			folded, err := p.profile.Evaluate(fold.Name, input)
			if err != nil {
				return Result{}, err
			}
			if err := p.applyChanges(ctx, tx, folded.Tables); err != nil {
				return Result{}, err
			}
		}
	}
	ineffective := 0
	if normalized.Decision == "ineffective" {
		ineffective = 1
	}
	lastRecordTime := delivery.TimeUnixNano
	if lastRecordTime == nil {
		lastRecordTime = delivery.ObservedUnixNano
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tailapp_frontier SET interpreted_position=?,last_event_id=?,complete=1,gap_position=NULL,gap_reason=NULL,gap_observed_unix_nano=NULL,last_record_time_unix_nano=? WHERE singleton=1`, delivery.Position, delivery.EventID, pointerValue(lastRecordTime)); err != nil {
		return Result{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tailapp_stats SET consumed_records=consumed_records+1,ineffective_records=ineffective_records+?,emitted_events=emitted_events+? WHERE singleton=1`, ineffective, len(emitted)); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(); err != nil {
		return Result{}, err
	}
	frontier.InterpretedPosition = delivery.Position
	frontier.LastEventID = delivery.EventID
	return Result{Frontier: frontier, Ineffective: ineffective == 1, EmittedEvents: len(emitted)}, nil
}

func (p *Projection) Stats(ctx context.Context) (Stats, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var result Stats
	if err := p.db.QueryRowContext(ctx, `SELECT consumed_records,ineffective_records,emitted_events FROM tailapp_stats WHERE singleton=1`).Scan(
		&result.ConsumedRecords, &result.IneffectiveRecords, &result.EmittedEvents); err != nil {
		return Stats{}, err
	}
	return result, nil
}

func deliveryEvent(delivery inbox.Delivery) (map[string]any, error) {
	var record any
	decoder := json.NewDecoder(bytes.NewReader(delivery.JSON))
	decoder.UseNumber()
	if err := decoder.Decode(&record); err != nil {
		return nil, fmt.Errorf("decode canonical record: %w", err)
	}
	return map[string]any{
		"id": delivery.EventID, "signal": delivery.Signal, "name": delivery.Name, "source": delivery.Source,
		"time_unix_nano": pointerValue(delivery.TimeUnixNano), "observed_unix_nano": pointerValue(delivery.ObservedUnixNano),
		"trace_id": pointerValue(delivery.TraceID), "span_id": pointerValue(delivery.SpanID), "content_digest": delivery.ContentDigest,
		"record": record,
	}, nil
}

func pointerValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func (p *Projection) evaluationInput(ctx context.Context, tx *sql.Tx, program profile.Program, position int64, eventID, eventType string, event map[string]any) (profile.EvaluationInput, error) {
	meta := map[string]any{"position": position, "event_id": eventID, "event_type": eventType}
	if err := p.profile.ValidateProgramInput(program.Name, meta, event); err != nil {
		return profile.EvaluationInput{}, err
	}
	readValues := make(map[string]any, len(program.Reads))
	if len(program.Reads) > 0 {
		// The compiled read plan executes under the core's default-deny
		// authorizer: only the relations the plan names (and their base
		// tables through declared views) are readable while these reads
		// run, enforcing the fold-read policy at execution time rather
		// than only as a compile-time check.
		authorizer, ok := p.profile.ReadAuthorizer(program.Name)
		if !ok {
			return profile.EvaluationInput{}, fmt.Errorf("program %q has no read authorizer", program.Name)
		}
		p.guard.seat(authorizer)
		defer p.guard.release()
	}
	for _, read := range program.Reads {
		value, err := executeRead(ctx, tx, read, event)
		if err != nil {
			return profile.EvaluationInput{}, fmt.Errorf("program %q read %q: %w", program.Name, read.Name, err)
		}
		readValues[read.Name] = value
	}
	return profile.EvaluationInput{Meta: meta, Event: event, Rows: readValues}, nil
}

func executeRead(ctx context.Context, tx *sql.Tx, read profile.Read, event map[string]any) (any, error) {
	args := make([]any, len(read.Parameters))
	for index, name := range read.Parameters {
		value, exists := event[name]
		if !exists {
			return nil, fmt.Errorf("event parameter %q is absent", name)
		}
		args[index] = sqliteValue(value, "")
	}
	rows, err := tx.QueryContext(ctx, read.SQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	values := make([]map[string]any, 0)
	for rows.Next() {
		scans := make([]any, len(columnTypes))
		destinations := make([]any, len(columnTypes))
		for index := range scans {
			destinations[index] = &scans[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(columnTypes))
		for index, column := range columnTypes {
			converted, err := fromSQLite(scans[index], column.DatabaseTypeName())
			if err != nil {
				return nil, fmt.Errorf("column %q: %w", column.Name(), err)
			}
			row[column.Name()] = converted
		}
		values = append(values, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	switch read.Cardinality {
	case profile.One:
		if len(values) != 1 {
			return nil, fmt.Errorf("ONE returned %d rows", len(values))
		}
		return values[0], nil
	case profile.OptionalOne:
		if len(values) > 1 {
			return nil, fmt.Errorf("OPTIONAL ONE returned %d rows", len(values))
		}
		if len(values) == 0 {
			return nil, nil
		}
		return values[0], nil
	case profile.Many:
		return values, nil
	default:
		return nil, errors.New("unknown read cardinality")
	}
}

// fromSQLite delegates the fold-input read conversion to the core codec,
// which owns the legacy-compatible fold-input shape.
func fromSQLite(value any, databaseType string) (any, error) {
	return jsonataddl.ReadRowValue(value, jsonataddl.LogicalType(databaseType))
}

func (p *Projection) applyChanges(ctx context.Context, tx *sql.Tx, changes map[string]profile.TableChanges) error {
	for _, tableName := range sortedKeys(changes) {
		table := p.profile.Tables[tableName]
		change := changes[tableName]
		for _, row := range change.Insert {
			if err := insertRow(ctx, tx, table, row, false); err != nil {
				return fmt.Errorf("insert %s: %w", tableName, err)
			}
		}
		for _, row := range change.Upsert {
			if err := insertRow(ctx, tx, table, row, true); err != nil {
				return fmt.Errorf("upsert %s: %w", tableName, err)
			}
		}
		for _, row := range change.Delete {
			if err := deleteRow(ctx, tx, table, row); err != nil {
				return fmt.Errorf("delete %s: %w", tableName, err)
			}
		}
	}
	return nil
}

func insertRow(ctx context.Context, tx *sql.Tx, table profile.Table, row map[string]any, upsert bool) error {
	columns := make([]string, len(table.Columns))
	placeholders := make([]string, len(table.Columns))
	args := make([]any, len(table.Columns))
	for index, column := range table.Columns {
		columns[index] = quote(column.Name)
		placeholders[index] = "?"
		args[index] = sqliteValue(row[column.Name], column.Type)
	}
	statement := `INSERT INTO ` + quote(table.Name) + ` (` + strings.Join(columns, ",") + `) VALUES (` + strings.Join(placeholders, ",") + `)`
	if upsert {
		var updates []string
		keys := make(map[string]bool)
		for _, key := range table.PrimaryKey {
			keys[strings.ToLower(key)] = true
		}
		for _, column := range table.Columns {
			if !keys[strings.ToLower(column.Name)] {
				updates = append(updates, quote(column.Name)+`=excluded.`+quote(column.Name))
			}
		}
		statement += ` ON CONFLICT (` + joinQuoted(table.PrimaryKey) + `) `
		if len(updates) == 0 {
			statement += `DO NOTHING`
		} else {
			statement += `DO UPDATE SET ` + strings.Join(updates, ",")
		}
	}
	_, err := tx.ExecContext(ctx, statement, args...)
	return err
}

func deleteRow(ctx context.Context, tx *sql.Tx, table profile.Table, row map[string]any) error {
	terms := make([]string, len(table.PrimaryKey))
	args := make([]any, len(table.PrimaryKey))
	columns := make(map[string]profile.Column)
	for _, column := range table.Columns {
		columns[strings.ToLower(column.Name)] = column
	}
	for index, name := range table.PrimaryKey {
		terms[index] = quote(name) + `=?`
		column := columns[strings.ToLower(name)]
		args[index] = sqliteValue(row[name], column.Type)
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM `+quote(table.Name)+` WHERE `+strings.Join(terms, ` AND `), args...)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed > 1 {
		return errors.New("primary-key delete changed multiple rows")
	}
	return nil
}

// sqliteValue delegates write-side conversion to the shared logical value
// codec, which owns the JSON-first affinity rule and the number, boolean,
// blob, text, and null conversions.
func sqliteValue(value any, logical any) any {
	return jsonataddl.SQLiteBindValue(value, jsonataddl.LogicalType(fmt.Sprint(logical)))
}

func transientProcessError(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if jsonataddl.IsEvaluationTimeout(err) {
		return true
	}
	var code sqlite3.ErrorCode
	if !errors.As(err, &code) {
		return false
	}
	switch code {
	case sqlite3.BUSY, sqlite3.LOCKED, sqlite3.NOMEM, sqlite3.READONLY,
		sqlite3.INTERRUPT, sqlite3.IOERR, sqlite3.FULL, sqlite3.CANTOPEN,
		sqlite3.PROTOCOL, sqlite3.SCHEMA:
		return true
	default:
		return false
	}
}

func quote(identifier string) string { return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"` }
func joinQuoted(values []string) string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = quote(value)
	}
	return strings.Join(result, ",")
}
func sortedKeys[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	slicesSort(result)
	return result
}
func slicesSort(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func (p *Projection) Frontier(ctx context.Context) (Frontier, error) {
	var result Frontier
	var complete int
	var gapPosition sql.NullInt64
	var gapReason sql.NullString
	var gapObserved, lastRecordTime sql.NullString
	if err := p.db.QueryRowContext(ctx, `SELECT i.revision,f.activation_boundary,f.interpreted_position,f.last_event_id,f.complete,f.gap_position,f.gap_reason,f.gap_observed_unix_nano,f.last_record_time_unix_nano
 FROM tailapp_projection_identity i CROSS JOIN tailapp_frontier f WHERE i.singleton=1 AND f.singleton=1`).Scan(
		&result.Revision, &result.ActivationBoundary, &result.InterpretedPosition, &result.LastEventID, &complete, &gapPosition, &gapReason, &gapObserved, &lastRecordTime); err != nil {
		return Frontier{}, err
	}
	result.Complete = complete != 0
	if gapPosition.Valid {
		result.GapPosition = &gapPosition.Int64
	}
	if gapReason.Valid {
		result.GapReason = &gapReason.String
	}
	if gapObserved.Valid {
		result.GapObservedUnixNano = &gapObserved.String
	}
	if lastRecordTime.Valid {
		result.LastRecordUnixNano = &lastRecordTime.String
	}
	return result, nil
}

func (p *Projection) recordGap(ctx context.Context, position int64, reason string) error {
	if len(reason) > 1024 {
		reason = reason[:1024]
	}
	observed := strconv.FormatInt(time.Now().UnixNano(), 10)
	_, err := p.db.ExecContext(ctx, `UPDATE tailapp_frontier SET complete=0,gap_position=?,gap_reason=?,gap_observed_unix_nano=? WHERE singleton=1 AND gap_position IS NULL`, position, reason, observed)
	return err
}
