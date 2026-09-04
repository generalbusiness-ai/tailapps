// Package query provides the bounded, default-deny SQL surface over durable
// Tailapp projections. Mounts are request-local and expose only compiled export
// views; they never become fold dependencies.
package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/ncruces/go-sqlite3"

	"github.com/generalbusiness-ai/tailapps/internal/profile"
	"github.com/generalbusiness-ai/tailapps/internal/projection"

	"github.com/generalbusiness-ai/tailapps/jsonataddl"
)

const (
	MaxSQLBytes    = 16 << 10
	MaxParameters  = 64
	DefaultRows    = 256
	MaxRows        = 1000
	MaxResultBytes = 1 << 20
	maxSQLiteValue = 256 << 10
	queryTimeout   = 5 * time.Second
	// ncruces installs SQLite's progress callback every 1,000 virtual-machine
	// instructions. The interrupt context below admits this many callback and
	// statement-boundary checks before returning a stable budget error. The
	// value is part of the runtime profile; the deadline remains only a
	// secondary safety net for work outside SQLite's virtual machine.
	queryProgressChecks = 2_048
)

var errQueryBudgetExceeded = errors.New("query_budget_exceeded: SQLite opcode budget exhausted")

type budgetContext struct {
	context.Context
	remaining atomic.Int64
	exceeded  atomic.Bool
}

func newBudgetContext(parent context.Context, checks int64) *budgetContext {
	result := &budgetContext{Context: parent}
	result.remaining.Store(checks)
	return result
}

// Err is called by ncruces at every statement boundary and by its SQLite
// progress handler every 1,000 VM instructions. Counting those fixed checks
// gives the query a deterministic runtime ceiling without modifying the
// pinned driver. Done and Deadline continue to describe the wall safety
// deadline supplied by the embedded parent context.
func (ctx *budgetContext) Err() error {
	if err := ctx.Context.Err(); err != nil {
		return err
	}
	if ctx.exceeded.Load() {
		return errQueryBudgetExceeded
	}
	if ctx.remaining.Add(-1) < 0 {
		ctx.exceeded.Store(true)
		return errQueryBudgetExceeded
	}
	return nil
}

type Namespace struct {
	Path     string
	Profile  *profile.Profile
	Frontier projection.Frontier
	// DeliveryHead is the durable inbox head observed at the query barrier. It
	// may be ahead of this projection's interpreted frontier while work waits.
	DeliveryHead int64
	// IneffectiveRecords is the durable count for the active projection at the
	// same engine query barrier as Frontier and DeliveryHead.
	IneffectiveRecords int64
}

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type NamespaceResult struct {
	Alias               string `json:"alias"`
	Tailapp             string `json:"tailapp"`
	Revision            string `json:"revision"`
	Contract            string `json:"contract"`
	InterpretedPosition int64  `json:"interpreted_position"`
}
type Result struct {
	Tailapp             string            `json:"tailapp"`
	Revision            string            `json:"revision"`
	DeliveryHead        int64             `json:"delivery_head"`
	InterpretedPosition int64             `json:"interpreted_position"`
	IneffectiveRecords  int64             `json:"ineffective_records"`
	Schemas             []NamespaceResult `json:"schemas"`
	Complete            bool              `json:"complete"`
	Columns             []Column          `json:"columns"`
	Rows                [][]any           `json:"rows"`
	ResultBytes         int               `json:"result_bytes"`
	Truncated           bool              `json:"truncated"`
}

type Request struct {
	SQL              string
	Parameters       []any
	Mounts           map[string]Namespace
	ExpectedRevision string
	ExpectedPosition *int64
	RowLimit         int
}

type Sandbox struct {
	connection *sqlite3.Conn
	primary    Namespace
	mounts     map[string]Namespace
	mu         sync.Mutex
}

func Open(primary Namespace, mounts map[string]Namespace) (*Sandbox, error) {
	if primary.Path == "" || primary.Profile == nil {
		return nil, errors.New("primary namespace is incomplete")
	}
	if !primary.Frontier.Complete || primary.Frontier.GapPosition != nil {
		return nil, errors.New("primary projection is incomplete")
	}
	absolute, err := filepath.Abs(primary.Path)
	if err != nil {
		return nil, err
	}
	location := &url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=ro"}
	connection, err := sqlite3.OpenFlags(location.String(), sqlite3.OPEN_READONLY|sqlite3.OPEN_URI)
	if err != nil {
		return nil, err
	}
	closeError := func(err error) (*Sandbox, error) { return nil, errors.Join(err, connection.Close()) }
	if _, err := connection.Config(sqlite3.DBCONFIG_DEFENSIVE, true); err != nil {
		return closeError(err)
	}
	if _, err := connection.Config(sqlite3.DBCONFIG_TRUSTED_SCHEMA, false); err != nil {
		return closeError(err)
	}
	if _, err := connection.Config(sqlite3.DBCONFIG_ENABLE_LOAD_EXTENSION, false); err != nil {
		return closeError(err)
	}
	connection.Limit(sqlite3.LIMIT_LENGTH, maxSQLiteValue)
	connection.Limit(sqlite3.LIMIT_SQL_LENGTH, MaxSQLBytes)
	connection.Limit(sqlite3.LIMIT_COLUMN, 128)
	connection.Limit(sqlite3.LIMIT_EXPR_DEPTH, 32)
	connection.Limit(sqlite3.LIMIT_COMPOUND_SELECT, 16)
	connection.Limit(sqlite3.LIMIT_VDBE_OP, 100_000)
	connection.Limit(sqlite3.LIMIT_FUNCTION_ARG, 32)
	connection.Limit(sqlite3.LIMIT_ATTACHED, 8)
	connection.Limit(sqlite3.LIMIT_VARIABLE_NUMBER, MaxParameters)
	connection.Limit(sqlite3.LIMIT_WORKER_THREADS, 0)
	if err := connection.Exec(`PRAGMA query_only=ON`); err != nil {
		return closeError(err)
	}
	copyMounts := make(map[string]Namespace, len(mounts))
	for alias, namespace := range mounts {
		if !identifier(alias) || strings.EqualFold(alias, "main") || strings.EqualFold(alias, "temp") {
			return closeError(fmt.Errorf("invalid mount alias %q", alias))
		}
		if namespace.Path == "" || namespace.Profile == nil || !namespace.Frontier.Complete || namespace.Frontier.GapPosition != nil {
			return closeError(fmt.Errorf("mount %q is unavailable", alias))
		}
		if namespace.Frontier.InterpretedPosition != primary.Frontier.InterpretedPosition {
			return closeError(fmt.Errorf("mount %q frontier is not aligned", alias))
		}
		mountPath, err := filepath.Abs(namespace.Path)
		if err != nil {
			return closeError(err)
		}
		mountURL := (&url.URL{Scheme: "file", Path: mountPath, RawQuery: "mode=ro"}).String()
		statement := `ATTACH DATABASE '` + strings.ReplaceAll(mountURL, `'`, `''`) + `' AS "` + alias + `"`
		if err := connection.Exec(statement); err != nil {
			return closeError(fmt.Errorf("attach mount %q: %w", alias, err))
		}
		copyMounts[alias] = namespace
	}
	if err := connection.SetAuthorizer(authorizer(primary, copyMounts)); err != nil {
		return closeError(err)
	}
	return &Sandbox{connection: connection, primary: primary, mounts: copyMounts}, nil
}

func (sandbox *Sandbox) Close() error {
	if sandbox == nil {
		return nil
	}
	sandbox.mu.Lock()
	defer sandbox.mu.Unlock()
	if sandbox.connection == nil {
		return nil
	}
	err := sandbox.connection.Close()
	sandbox.connection = nil
	return err
}

func (sandbox *Sandbox) Query(ctx context.Context, request Request) (Result, error) {
	sandbox.mu.Lock()
	defer sandbox.mu.Unlock()
	if sandbox.connection == nil {
		return Result{}, errors.New("query sandbox is closed")
	}
	if len(request.SQL) == 0 || len(request.SQL) > MaxSQLBytes {
		return Result{}, errors.New("query SQL is empty or too large")
	}
	if len(request.Parameters) > MaxParameters {
		return Result{}, errors.New("query has too many parameters")
	}
	if request.ExpectedRevision != "" && request.ExpectedRevision != sandbox.primary.Frontier.Revision {
		return Result{}, errors.New("frontier_changed: revision differs")
	}
	if request.ExpectedPosition != nil && *request.ExpectedPosition != sandbox.primary.Frontier.InterpretedPosition {
		return Result{}, errors.New("frontier_changed: position differs")
	}
	limit := request.RowLimit
	if limit == 0 {
		limit = DefaultRows
	}
	if limit < 1 || limit > MaxRows {
		return Result{}, fmt.Errorf("row limit must be between 1 and %d", MaxRows)
	}
	rewritten, err := rewriteMountExports(request.SQL, sandbox.mounts)
	if err != nil {
		return Result{}, err
	}
	deadlineContext, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	queryContext := newBudgetContext(deadlineContext, queryProgressChecks)
	oldInterrupt := sandbox.connection.SetInterrupt(queryContext)
	defer sandbox.connection.SetInterrupt(oldInterrupt)
	statement, tail, err := sandbox.connection.Prepare(rewritten)
	if err != nil {
		return Result{}, queryError(queryContext, err)
	}
	if statement == nil {
		return Result{}, errors.New("query contains no statement")
	}
	defer statement.Close()
	if strings.TrimSpace(tail) != "" {
		return Result{}, errors.New("query contains more than one statement")
	}
	if !statement.ReadOnly() {
		return Result{}, errors.New("query is not read-only")
	}
	if statement.BindCount() != len(request.Parameters) {
		return Result{}, fmt.Errorf("query expects %d parameters, got %d", statement.BindCount(), len(request.Parameters))
	}
	for index, parameter := range request.Parameters {
		if err := bind(statement, index+1, parameter); err != nil {
			return Result{}, fmt.Errorf("parameter %d: %w", index+1, err)
		}
	}
	deliveryHead := sandbox.primary.DeliveryHead
	if deliveryHead < sandbox.primary.Frontier.InterpretedPosition {
		deliveryHead = sandbox.primary.Frontier.InterpretedPosition
	}
	result := Result{Tailapp: sandbox.primary.Profile.Name, Revision: sandbox.primary.Frontier.Revision,
		DeliveryHead: deliveryHead, InterpretedPosition: sandbox.primary.Frontier.InterpretedPosition,
		IneffectiveRecords: sandbox.primary.IneffectiveRecords, Complete: true}
	for alias, namespace := range sandbox.mounts {
		result.Schemas = append(result.Schemas, NamespaceResult{Alias: alias, Tailapp: namespace.Profile.Name, Revision: namespace.Frontier.Revision, Contract: namespace.Profile.ExportContractDigest, InterpretedPosition: namespace.Frontier.InterpretedPosition})
	}
	sortSchemas(result.Schemas)
	result.Columns = make([]Column, statement.ColumnCount())
	for index := range result.Columns {
		result.Columns[index] = Column{Name: statement.ColumnName(index), Type: columnType(statement, index)}
	}
	bytesUsed := 0
	for statement.Step() {
		if len(result.Rows) == limit {
			result.Truncated = true
			break
		}
		row := make([]any, len(result.Columns))
		for index := range row {
			value, err := columnValue(statement, index, result.Columns[index].Type)
			if err != nil {
				return Result{}, err
			}
			row[index] = value
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return Result{}, err
		}
		if bytesUsed+len(encoded) > MaxResultBytes {
			result.Truncated = true
			break
		}
		bytesUsed += len(encoded)
		result.Rows = append(result.Rows, row)
	}
	if err := statement.Err(); err != nil {
		return Result{}, queryError(queryContext, err)
	}
	result.ResultBytes = bytesUsed
	return result, nil
}

func authorizer(primary Namespace, mounts map[string]Namespace) func(sqlite3.AuthorizerActionCode, string, string, string, string) sqlite3.AuthorizerReturnCode {
	primaryRelations := relationColumns(primary.Profile, false)
	return func(action sqlite3.AuthorizerActionCode, relation, column, schema, inner string) sqlite3.AuthorizerReturnCode {
		switch action {
		case sqlite3.AUTH_SELECT, sqlite3.AUTH_RECURSIVE:
			return sqlite3.AUTH_OK
		case sqlite3.AUTH_READ:
			if schema == "main" || schema == "" {
				if strings.HasPrefix(relation, "tailapp_") || strings.HasPrefix(relation, "sqlite_") || strings.HasPrefix(relation, "__tailapp_export_") {
					return sqlite3.AUTH_DENY
				}
				if columns, ok := primaryRelations[strings.ToLower(relation)]; ok && (column == "" || columns == nil || columns[strings.ToLower(column)]) {
					return sqlite3.AUTH_OK
				}
				return sqlite3.AUTH_DENY
			}
			namespace, exists := mounts[schema]
			if !exists {
				return sqlite3.AUTH_DENY
			}
			if strings.HasPrefix(relation, "tailapp_") || strings.HasPrefix(relation, "sqlite_") {
				return sqlite3.AUTH_DENY
			}
			if strings.HasPrefix(relation, "__tailapp_export_") && inner == "" {
				name := strings.TrimPrefix(relation, "__tailapp_export_")
				exported, ok := namespace.Profile.Exports[name]
				if !ok {
					return sqlite3.AUTH_DENY
				}
				for _, item := range exported.Columns {
					if strings.EqualFold(item.Name, column) {
						return sqlite3.AUTH_OK
					}
				}
				return sqlite3.AUTH_DENY
			}
			if strings.HasPrefix(inner, "__tailapp_export_") {
				return sqlite3.AUTH_OK
			}
			return sqlite3.AUTH_DENY
		case sqlite3.AUTH_FUNCTION:
			switch strings.ToLower(column) {
			case "avg", "coalesce", "count", "ifnull", "max", "min", "sum", "total":
				return sqlite3.AUTH_OK
			}
			return sqlite3.AUTH_DENY
		case sqlite3.AUTH_PRAGMA:
			return sqlite3.AUTH_DENY
		default:
			return sqlite3.AUTH_DENY
		}
	}
}

func relationColumns(compiled *profile.Profile, exportsOnly bool) map[string]map[string]bool {
	result := make(map[string]map[string]bool)
	if !exportsOnly {
		for name, table := range compiled.Tables {
			columns := make(map[string]bool)
			for _, column := range table.Columns {
				columns[strings.ToLower(column.Name)] = true
			}
			result[strings.ToLower(name)] = columns
		}
		for name := range compiled.Views {
			result[strings.ToLower(name)] = allColumns()
		}
	}
	return result
}
func allColumns() map[string]bool { return nil }

func rewriteMountExports(sql string, mounts map[string]Namespace) (string, error) {
	var output strings.Builder
	expectRelation := false
	for index := 0; index < len(sql); {
		if sql[index] == '\'' || sql[index] == '"' || sql[index] == '`' || sql[index] == '[' {
			end, err := copyQuoted(&output, sql, index)
			if err != nil {
				return "", err
			}
			index = end
			continue
		}
		if identifierStart(rune(sql[index])) {
			start := index
			index++
			for index < len(sql) && identifierContinue(rune(sql[index])) {
				index++
			}
			first := sql[start:index]
			if expectRelation && index < len(sql) && sql[index] == '.' {
				secondStart := index + 1
				secondEnd := secondStart
				for secondEnd < len(sql) && identifierContinue(rune(sql[secondEnd])) {
					secondEnd++
				}
				if namespace, ok := mounts[first]; ok && secondEnd > secondStart {
					name := sql[secondStart:secondEnd]
					if _, ok := namespace.Profile.Exports[name]; !ok {
						return "", fmt.Errorf("mount %q has no export %q", first, name)
					}
					output.WriteString(first + ".__tailapp_export_" + name)
					index = secondEnd
					expectRelation = false
					continue
				}
			}
			output.WriteString(first)
			expectRelation = strings.EqualFold(first, "FROM") || strings.EqualFold(first, "JOIN")
			continue
		}
		output.WriteByte(sql[index])
		index++
	}
	return output.String(), nil
}

func copyQuoted(output *strings.Builder, sql string, start int) (int, error) {
	open := sql[start]
	close := open
	if open == '[' {
		close = ']'
	}
	output.WriteByte(open)
	for index := start + 1; index < len(sql); index++ {
		output.WriteByte(sql[index])
		if sql[index] == close {
			if index+1 < len(sql) && sql[index+1] == close {
				output.WriteByte(sql[index+1])
				index++
				continue
			}
			return index + 1, nil
		}
	}
	return 0, errors.New("unterminated quoted SQL token")
}

func bind(statement *sqlite3.Stmt, position int, value any) error {
	switch typed := value.(type) {
	case nil:
		return statement.BindNull(position)
	case string:
		return statement.BindText(position, typed)
	case bool:
		return statement.BindBool(position, typed)
	case int:
		return statement.BindInt64(position, int64(typed))
	case int64:
		return statement.BindInt64(position, typed)
	case float64:
		if math.IsInf(typed, 0) || math.IsNaN(typed) {
			return errors.New("parameter is non-finite")
		}
		return statement.BindFloat(position, typed)
	case []byte:
		return statement.BindBlob(position, typed)
	default:
		return errors.New("parameter has unsupported type")
	}
}

func columnType(statement *sqlite3.Stmt, index int) string {
	declared := string(jsonataddl.SQLiteLogicalType(statement.ColumnDeclType(index)))
	if declared != "" {
		return declared
	}
	return "DYNAMIC"
}

// columnValue delegates read-side conversion to the shared logical value
// codec over a driver-decoupled column, which owns the boolean mapping,
// exact-integer wrapper, JSON decoding, blob wrapper, and finiteness rules.
func columnValue(statement *sqlite3.Stmt, index int, declared string) (any, error) {
	column := jsonataddl.SQLiteColumn{}
	switch statement.ColumnType(index) {
	case sqlite3.NULL:
		column.Kind = jsonataddl.ColumnNull
	case sqlite3.INTEGER:
		column.Kind, column.Int = jsonataddl.ColumnInteger, statement.ColumnInt64(index)
	case sqlite3.FLOAT:
		column.Kind, column.Float = jsonataddl.ColumnFloat, statement.ColumnFloat(index)
	case sqlite3.TEXT:
		column.Kind, column.Text = jsonataddl.ColumnText, statement.ColumnText(index)
	case sqlite3.BLOB:
		column.Kind, column.Blob = jsonataddl.ColumnBlob, statement.ColumnBlob(index, nil)
	default:
		return nil, errors.New("query returned unsupported SQLite type")
	}
	return jsonataddl.LogicalColumnValue(column, jsonataddl.LogicalType(declared))
}

func queryError(ctx *budgetContext, err error) error {
	if ctx.exceeded.Load() {
		return errQueryBudgetExceeded
	}
	if ctx.Context.Err() != nil {
		return fmt.Errorf("query_cancelled: %w", ctx.Context.Err())
	}
	return err
}
func identifier(value string) bool {
	if value == "" || !identifierStart(rune(value[0])) {
		return false
	}
	for _, r := range value[1:] {
		if !identifierContinue(r) {
			return false
		}
	}
	return true
}
func identifierStart(value rune) bool    { return value == '_' || unicode.IsLetter(value) }
func identifierContinue(value rune) bool { return identifierStart(value) || unicode.IsDigit(value) }
func sortSchemas(values []NamespaceResult) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j].Alias < values[j-1].Alias; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
