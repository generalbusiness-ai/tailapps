// Package definition persists mutable Tailapp drafts, immutable compiled
// source revisions and active revision pointers in the control database.
package definition

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"sort"
	"time"

	"github.com/ncruces/go-sqlite3"
	sqlitedriver "github.com/ncruces/go-sqlite3/driver"
)

var ErrRevisionChanged = errors.New("draft revision changed")
var ErrIdempotencyConflict = errors.New("idempotency key is already bound to a different mutation")
var ErrIdempotencyInDoubt = errors.New("idempotency key has an incomplete prior mutation")

type MutationRecord struct {
	Response     []byte
	ErrorCode    string
	ErrorMessage string
}

type App struct {
	Name           string  `json:"name"`
	DraftRevision  string  `json:"draft_revision"`
	ActiveRevision *string `json:"active_revision,omitempty"`
	RuntimeProfile *string `json:"runtime_profile,omitempty"`
	ActivationMode *string `json:"activation_mode,omitempty"`
	Boundary       *int64  `json:"boundary_position,omitempty"`
}

type ActivationJournal struct {
	Name          string
	NewRevision   string
	Runtime       string
	Mode          string
	Boundary      int64
	ExpectedDraft string
	OldRevision   *string
}

type Registry struct{ db *sql.DB }

func Open(path string) (*Registry, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: absolute}).String()
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
		return nil, err
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	registry := &Registry{db: database}
	if err := registry.initialize(context.Background()); err != nil {
		database.Close()
		return nil, err
	}
	return registry, nil
}

func (r *Registry) initialize(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS definition_tailapps (name TEXT PRIMARY KEY,draft_revision TEXT NOT NULL,active_revision TEXT,runtime_profile TEXT,activation_mode TEXT,boundary_position INTEGER,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS definition_elements (tailapp TEXT NOT NULL REFERENCES definition_tailapps(name) ON DELETE CASCADE,path TEXT NOT NULL,content BLOB NOT NULL,digest TEXT NOT NULL,PRIMARY KEY(tailapp,path))`,
		`CREATE TABLE IF NOT EXISTS definition_revisions (digest TEXT PRIMARY KEY,tailapp TEXT NOT NULL,runtime_profile TEXT NOT NULL,source_json BLOB NOT NULL,created_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS definition_activation_journal (tailapp TEXT PRIMARY KEY,new_revision TEXT NOT NULL,runtime_profile TEXT NOT NULL,mode TEXT NOT NULL,boundary_position INTEGER NOT NULL,expected_draft TEXT NOT NULL,old_revision TEXT,created_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS definition_mutation_idempotency (idempotency_key TEXT PRIMARY KEY,operation TEXT NOT NULL,request_digest TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN ('pending','complete')),response_json BLOB,error_code TEXT,error_message TEXT,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) Close() error { return r.db.Close() }

// BeginMutation durably binds a caller-provided key to one exact mutation.
// A complete prior call is replayed. A pending record can only survive a
// process/storage failure between binding and completion, so it is reported as
// indeterminate instead of risking a duplicate destructive effect.
func (r *Registry) BeginMutation(ctx context.Context, key, operation, requestDigest string) (MutationRecord, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return MutationRecord{}, false, err
	}
	defer tx.Rollback()
	var storedOperation, storedDigest, state string
	var response []byte
	var errorCode, errorMessage sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT operation,request_digest,state,response_json,error_code,error_message FROM definition_mutation_idempotency WHERE idempotency_key=?`, key).
		Scan(&storedOperation, &storedDigest, &state, &response, &errorCode, &errorMessage)
	if err == nil {
		if storedOperation != operation || storedDigest != requestDigest {
			return MutationRecord{}, false, ErrIdempotencyConflict
		}
		if state != "complete" {
			return MutationRecord{}, false, ErrIdempotencyInDoubt
		}
		return MutationRecord{Response: append([]byte(nil), response...), ErrorCode: errorCode.String, ErrorMessage: errorMessage.String}, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return MutationRecord{}, false, err
	}
	now := time.Now().UnixNano()
	if _, err := tx.ExecContext(ctx, `INSERT INTO definition_mutation_idempotency(idempotency_key,operation,request_digest,state,created_at,updated_at) VALUES(?,?,?,'pending',?,?)`, key, operation, requestDigest, now, now); err != nil {
		return MutationRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return MutationRecord{}, false, err
	}
	return MutationRecord{}, false, nil
}

func (r *Registry) CompleteMutation(ctx context.Context, key, operation, requestDigest string, record MutationRecord) error {
	result, err := r.db.ExecContext(ctx, `UPDATE definition_mutation_idempotency SET state='complete',response_json=?,error_code=?,error_message=?,updated_at=? WHERE idempotency_key=? AND operation=? AND request_digest=? AND state='pending'`,
		record.Response, nullableString(record.ErrorCode), nullableString(record.ErrorMessage), time.Now().UnixNano(), key, operation, requestDigest)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrIdempotencyConflict
	}
	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (r *Registry) Create(ctx context.Context, name string, sources map[string][]byte) (App, error) {
	revision := draftDigest(sources)
	now := time.Now().UnixNano()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return App{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO definition_tailapps(name,draft_revision,created_at,updated_at) VALUES(?,?,?,?)`, name, revision, now, now); err != nil {
		return App{}, err
	}
	for path, content := range sources {
		sum := sha256.Sum256(content)
		if _, err := tx.ExecContext(ctx, `INSERT INTO definition_elements(tailapp,path,content,digest) VALUES(?,?,?,?)`, name, path, content, "sha256:"+hex.EncodeToString(sum[:])); err != nil {
			return App{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return App{}, err
	}
	return App{Name: name, DraftRevision: revision}, nil
}

func (r *Registry) List(ctx context.Context) ([]App, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT name,draft_revision,active_revision,runtime_profile,activation_mode,boundary_position FROM definition_tailapps ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []App
	for rows.Next() {
		app, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, app)
	}
	return result, rows.Err()
}
func (r *Registry) Get(ctx context.Context, name string) (App, error) {
	return scanApp(r.db.QueryRowContext(ctx, `SELECT name,draft_revision,active_revision,runtime_profile,activation_mode,boundary_position FROM definition_tailapps WHERE name=?`, name))
}

type scanner interface{ Scan(...any) error }

func scanApp(row scanner) (App, error) {
	var app App
	var active, runtime, mode sql.NullString
	var boundary sql.NullInt64
	if err := row.Scan(&app.Name, &app.DraftRevision, &active, &runtime, &mode, &boundary); err != nil {
		return App{}, err
	}
	if active.Valid {
		app.ActiveRevision = &active.String
	}
	if runtime.Valid {
		app.RuntimeProfile = &runtime.String
	}
	if mode.Valid {
		app.ActivationMode = &mode.String
	}
	if boundary.Valid {
		app.Boundary = &boundary.Int64
	}
	return app, nil
}

func (r *Registry) Sources(ctx context.Context, name string) (map[string][]byte, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT path,content FROM definition_elements WHERE tailapp=? ORDER BY path`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string][]byte{}
	for rows.Next() {
		var path string
		var content []byte
		if err := rows.Scan(&path, &content); err != nil {
			return nil, err
		}
		result[path] = append([]byte(nil), content...)
	}
	return result, rows.Err()
}

func (r *Registry) Put(ctx context.Context, name, path string, content []byte, expected string) (App, error) {
	return r.mutate(ctx, name, expected, func(s map[string][]byte) error { s[path] = append([]byte(nil), content...); return nil })
}
func (r *Registry) DeleteElement(ctx context.Context, name, path, expected string) (App, error) {
	return r.mutate(ctx, name, expected, func(s map[string][]byte) error {
		if _, ok := s[path]; !ok {
			return sql.ErrNoRows
		}
		delete(s, path)
		return nil
	})
}

func (r *Registry) mutate(ctx context.Context, name, expected string, change func(map[string][]byte) error) (App, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return App{}, err
	}
	defer tx.Rollback()
	var current string
	if err := tx.QueryRowContext(ctx, `SELECT draft_revision FROM definition_tailapps WHERE name=?`, name).Scan(&current); err != nil {
		return App{}, err
	}
	if current != expected {
		return App{}, ErrRevisionChanged
	}
	rows, err := tx.QueryContext(ctx, `SELECT path,content FROM definition_elements WHERE tailapp=?`, name)
	if err != nil {
		return App{}, err
	}
	sources := map[string][]byte{}
	for rows.Next() {
		var path string
		var content []byte
		if err := rows.Scan(&path, &content); err != nil {
			rows.Close()
			return App{}, err
		}
		sources[path] = content
	}
	rows.Close()
	if err := change(sources); err != nil {
		return App{}, err
	}
	next := draftDigest(sources)
	if _, err := tx.ExecContext(ctx, `DELETE FROM definition_elements WHERE tailapp=?`, name); err != nil {
		return App{}, err
	}
	for path, content := range sources {
		sum := sha256.Sum256(content)
		if _, err := tx.ExecContext(ctx, `INSERT INTO definition_elements VALUES(?,?,?,?)`, name, path, content, "sha256:"+hex.EncodeToString(sum[:])); err != nil {
			return App{}, err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE definition_tailapps SET draft_revision=?,updated_at=? WHERE name=? AND draft_revision=?`, next, time.Now().UnixNano(), name, expected)
	if err != nil {
		return App{}, err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return App{}, ErrRevisionChanged
	}
	if err := tx.Commit(); err != nil {
		return App{}, err
	}
	return r.Get(ctx, name)
}

func (r *Registry) RecordRevision(ctx context.Context, name, digest, runtime string, sources map[string][]byte) error {
	encoded, err := json.Marshal(sources)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO definition_revisions(digest,tailapp,runtime_profile,source_json,created_at) VALUES(?,?,?,?,?) ON CONFLICT(digest) DO NOTHING`, digest, name, runtime, encoded, time.Now().UnixNano())
	return err
}
func (r *Registry) RevisionSources(ctx context.Context, digest string) (map[string][]byte, string, error) {
	var encoded []byte
	var runtime string
	if err := r.db.QueryRowContext(ctx, `SELECT source_json,runtime_profile FROM definition_revisions WHERE digest=?`, digest).Scan(&encoded, &runtime); err != nil {
		return nil, "", err
	}
	var sources map[string][]byte
	if err := json.Unmarshal(encoded, &sources); err != nil {
		return nil, "", err
	}
	return sources, runtime, nil
}
func (r *Registry) Activate(ctx context.Context, name, digest, runtime, mode string, boundary int64, expectedDraft string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE definition_tailapps SET active_revision=?,runtime_profile=?,activation_mode=?,boundary_position=?,updated_at=? WHERE name=? AND draft_revision=?`, digest, runtime, mode, boundary, time.Now().UnixNano(), name, expectedDraft)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrRevisionChanged
	}
	return nil
}

func (r *Registry) BeginActivation(ctx context.Context, journal ActivationJournal) error {
	var old any
	if journal.OldRevision != nil {
		old = *journal.OldRevision
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO definition_activation_journal(tailapp,new_revision,runtime_profile,mode,boundary_position,expected_draft,old_revision,created_at) VALUES(?,?,?,?,?,?,?,?)`,
		journal.Name, journal.NewRevision, journal.Runtime, journal.Mode, journal.Boundary, journal.ExpectedDraft, old, time.Now().UnixNano())
	return err
}

func (r *Registry) ActivationJournals(ctx context.Context) ([]ActivationJournal, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT tailapp,new_revision,runtime_profile,mode,boundary_position,expected_draft,old_revision FROM definition_activation_journal ORDER BY tailapp`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ActivationJournal
	for rows.Next() {
		var item ActivationJournal
		var old sql.NullString
		if err := rows.Scan(&item.Name, &item.NewRevision, &item.Runtime, &item.Mode, &item.Boundary, &item.ExpectedDraft, &old); err != nil {
			return nil, err
		}
		if old.Valid {
			item.OldRevision = &old.String
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Registry) FinishActivation(ctx context.Context, journal ActivationJournal) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE definition_tailapps SET active_revision=?,runtime_profile=?,activation_mode=?,boundary_position=?,updated_at=? WHERE name=? AND draft_revision=?`,
		journal.NewRevision, journal.Runtime, journal.Mode, journal.Boundary, time.Now().UnixNano(), journal.Name, journal.ExpectedDraft)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrRevisionChanged
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM definition_activation_journal WHERE tailapp=? AND new_revision=?`, journal.Name, journal.NewRevision); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Registry) AbortActivation(ctx context.Context, name, revision string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM definition_activation_journal WHERE tailapp=? AND new_revision=?`, name, revision)
	return err
}
func (r *Registry) Delete(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM definition_tailapps WHERE name=?`, name)
	return err
}

func draftDigest(sources map[string][]byte) string {
	hash := sha256.New()
	keys := make([]string, 0, len(sources))
	for key := range sources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		hash.Write([]byte{0})
		hash.Write([]byte(key))
		hash.Write([]byte{0})
		hash.Write(sources[key])
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}
