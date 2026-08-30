package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ncruces/go-sqlite3"

	"github.com/generalbusiness-ai/tailapps/jsonataddl"
)

// TestConformanceCorpusProjectionCases runs the corpus cases that exercise
// the extracted core against real SQLite state: the compiled read plan
// executes under the core's default-deny read authorizer, evaluation
// consumes the rows the plan returned, and the validated mutation plan is
// applied by the harness acting as the host, with the final state frozen as
// a golden. Direct-SQL cases freeze the authorizer's denials. These cases
// are core-only: the runtime authorizer is enforcement the extraction adds,
// so there is no prior implementation to run them against.
func TestConformanceCorpusProjectionCases(t *testing.T) {
	entries, err := os.ReadDir(corpusRoot)
	if err != nil {
		t.Fatalf("corpus root: %v", err)
	}
	ran := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		caseDir := filepath.Join(corpusRoot, entry.Name())
		manifest := readManifest(t, caseDir)
		if len(manifest.Projection) == 0 {
			continue
		}
		application, err := jsonataddl.LoadApplication(os.DirFS(filepath.Join(caseDir, manifest.Application)), ".", "corpus-app", jsonataddl.Tailapp(), RuntimeID)
		if err != nil {
			t.Fatalf("%s: compile through core: %v", entry.Name(), err)
		}
		for _, projection := range manifest.Projection {
			ran++
			t.Run(entry.Name()+"/"+projection.Name, func(t *testing.T) {
				runProjectionCase(t, caseDir, application, projection)
			})
		}
	}
	if ran == 0 {
		t.Fatal("corpus contains no projection cases")
	}
}

func runProjectionCase(t *testing.T, caseDir string, application *jsonataddl.Application, projection corpusProjectionCase) {
	connection, err := sqlite3.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	for _, statement := range append(application.SchemaSQL(), application.ReplaceableSQL()...) {
		if err := connection.Exec(statement); err != nil {
			t.Fatalf("apply schema: %v", err)
		}
	}
	if projection.State != "" {
		fixture, err := os.ReadFile(filepath.Join(caseDir, projection.State))
		if err != nil {
			t.Fatal(err)
		}
		if err := connection.Exec(string(fixture)); err != nil {
			t.Fatalf("apply state fixture: %v", err)
		}
	}
	authorizer, found := application.ReadAuthorizer(projection.Program)
	if !found {
		t.Fatalf("program %q has no read authorizer", projection.Program)
	}
	if err := connection.SetAuthorizer(authorizer); err != nil {
		t.Fatal(err)
	}

	if projection.SQL != "" {
		compareGoldenText(t, filepath.Join(caseDir, projection.Expect), attemptSQL(t, connection, projection.SQL))
		return
	}

	var event map[string]any
	if projection.Event != "" {
		decodeFile(t, filepath.Join(caseDir, projection.Event), &event)
	}
	plan, found := application.ReadPlan(projection.Program)
	if !found {
		t.Fatalf("program %q has no read plan", projection.Program)
	}
	outcome := ""
	rows := make(map[string]any, len(plan))
	for _, read := range plan {
		value, err := executePlanRead(connection, read, event)
		if err != nil {
			outcome = "ERROR: read " + read.Name + ": " + err.Error() + "\n"
			break
		}
		rows[read.Name] = value
	}
	if outcome == "" {
		input := jsonataddl.EvaluationInput{Meta: projection.Meta, Event: event, Rows: rows}
		result, err := application.Evaluate(projection.Program, input)
		if err != nil {
			outcome = "ERROR: " + err.Error() + "\n"
		} else {
			encoded, marshalErr := json.MarshalIndent(result, "", " ")
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			outcome = string(encoded) + "\n"
			if projection.FinalState != "" {
				// The harness is the host here: mutations are applied outside
				// the read authorizer, exactly as the projection transaction
				// owns its own writes.
				if err := connection.SetAuthorizer(permitEverything); err != nil {
					t.Fatal(err)
				}
				applyMutationPlan(t, connection, application, result)
				compareGoldenJSON(t, filepath.Join(caseDir, projection.FinalState), dumpTables(t, connection, application))
			}
		}
	}
	compareGoldenText(t, filepath.Join(caseDir, projection.Expect), outcome)
}

func permitEverything(sqlite3.AuthorizerActionCode, string, string, string, string) sqlite3.AuthorizerReturnCode {
	return sqlite3.AUTH_OK
}

// attemptSQL freezes what an arbitrary statement does under the read
// authorizer: the denial diagnostic, or the rows it returned.
func attemptSQL(t *testing.T, connection *sqlite3.Conn, query string) string {
	t.Helper()
	statement, _, err := connection.Prepare(query)
	if err != nil {
		return "ERROR: " + err.Error() + "\n"
	}
	defer statement.Close()
	var rows []map[string]any
	for statement.Step() {
		row := make(map[string]any, statement.ColumnCount())
		for index := 0; index < statement.ColumnCount(); index++ {
			value, err := statementColumnValue(statement, index)
			if err != nil {
				return "ERROR: " + err.Error() + "\n"
			}
			row[statement.ColumnName(index)] = value
		}
		rows = append(rows, row)
	}
	if err := statement.Err(); err != nil {
		return "ERROR: " + err.Error() + "\n"
	}
	encoded, err := json.MarshalIndent(rows, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded) + "\n"
}

// executePlanRead executes one compiled read exactly as a host would:
// parameters bound from the event's scalar fields, rows converted through
// the shared logical value codec, and the declared cardinality enforced.
func executePlanRead(connection *sqlite3.Conn, read jsonataddl.Read, event map[string]any) (any, error) {
	statement, _, err := connection.Prepare(read.SQL)
	if err != nil {
		return nil, err
	}
	defer statement.Close()
	for index, name := range read.Parameters {
		value, exists := event[name]
		if !exists {
			return nil, fmt.Errorf("event parameter %q is absent", name)
		}
		if err := bindStatementValue(statement, index+1, jsonataddl.SQLiteBindValue(value, jsonataddl.LogicalType(""))); err != nil {
			return nil, err
		}
	}
	var values []map[string]any
	for statement.Step() {
		row := make(map[string]any, statement.ColumnCount())
		for index := 0; index < statement.ColumnCount(); index++ {
			value, err := statementColumnValue(statement, index)
			if err != nil {
				return nil, fmt.Errorf("column %q: %w", statement.ColumnName(index), err)
			}
			row[statement.ColumnName(index)] = value
		}
		values = append(values, row)
	}
	if err := statement.Err(); err != nil {
		return nil, err
	}
	switch read.Cardinality {
	case jsonataddl.One:
		if len(values) != 1 {
			return nil, fmt.Errorf("ONE returned %d rows", len(values))
		}
		return values[0], nil
	case jsonataddl.OptionalOne:
		if len(values) > 1 {
			return nil, fmt.Errorf("OPTIONAL ONE returned %d rows", len(values))
		}
		if len(values) == 0 {
			return nil, nil
		}
		return values[0], nil
	case jsonataddl.Many:
		return values, nil
	default:
		return nil, errors.New("unknown read cardinality")
	}
}

func bindStatementValue(statement *sqlite3.Stmt, position int, value any) error {
	switch typed := value.(type) {
	case nil:
		return statement.BindNull(position)
	case string:
		return statement.BindText(position, typed)
	case bool:
		return statement.BindBool(position, typed)
	case int64:
		return statement.BindInt64(position, typed)
	case float64:
		return statement.BindFloat(position, typed)
	case []byte:
		return statement.BindBlob(position, typed)
	default:
		return fmt.Errorf("bind value has unsupported type %T", value)
	}
}

func statementColumnValue(statement *sqlite3.Stmt, index int) (any, error) {
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
		return nil, errors.New("unsupported SQLite column type")
	}
	declared := strings.ToUpper(statement.ColumnDeclType(index))
	if declared == "" {
		declared = "DYNAMIC"
	}
	return jsonataddl.LogicalColumnValue(column, jsonataddl.LogicalType(declared))
}

// applyMutationPlan executes a validated mutation plan the way the host
// projection does: complete-row inserts and upserts with primary-key
// conflict targets, and primary-key deletes.
func applyMutationPlan(t *testing.T, connection *sqlite3.Conn, application *jsonataddl.Application, result jsonataddl.EvaluationResult) {
	t.Helper()
	tables := application.Tables()
	for _, tableName := range sortedKeys(result.Tables) {
		table := tables[tableName]
		changes := result.Tables[tableName]
		for _, row := range changes.Insert {
			applyWriteRow(t, connection, table, row, false)
		}
		for _, row := range changes.Upsert {
			applyWriteRow(t, connection, table, row, true)
		}
		for _, row := range changes.Delete {
			applyDeleteRow(t, connection, table, row)
		}
	}
}

func applyWriteRow(t *testing.T, connection *sqlite3.Conn, table jsonataddl.Table, row map[string]any, upsert bool) {
	t.Helper()
	columns := make([]string, len(table.Columns))
	placeholders := make([]string, len(table.Columns))
	values := make([]any, len(table.Columns))
	for index, column := range table.Columns {
		columns[index] = quoteIdentifier(column.Name)
		placeholders[index] = "?"
		values[index] = jsonataddl.SQLiteBindValue(row[column.Name], column.Type)
	}
	statementSQL := `INSERT INTO ` + quoteIdentifier(table.Name) + ` (` + strings.Join(columns, ",") + `) VALUES (` + strings.Join(placeholders, ",") + `)`
	if upsert {
		keys := make(map[string]bool)
		quotedKeys := make([]string, len(table.PrimaryKey))
		for index, key := range table.PrimaryKey {
			keys[strings.ToLower(key)] = true
			quotedKeys[index] = quoteIdentifier(key)
		}
		var updates []string
		for _, column := range table.Columns {
			if !keys[strings.ToLower(column.Name)] {
				updates = append(updates, quoteIdentifier(column.Name)+`=excluded.`+quoteIdentifier(column.Name))
			}
		}
		statementSQL += ` ON CONFLICT (` + strings.Join(quotedKeys, ",") + `) `
		if len(updates) == 0 {
			statementSQL += `DO NOTHING`
		} else {
			statementSQL += `DO UPDATE SET ` + strings.Join(updates, ",")
		}
	}
	executeChange(t, connection, statementSQL, values)
}

func applyDeleteRow(t *testing.T, connection *sqlite3.Conn, table jsonataddl.Table, row map[string]any) {
	t.Helper()
	columnTypes := make(map[string]jsonataddl.LogicalType)
	for _, column := range table.Columns {
		columnTypes[strings.ToLower(column.Name)] = column.Type
	}
	terms := make([]string, len(table.PrimaryKey))
	values := make([]any, len(table.PrimaryKey))
	for index, name := range table.PrimaryKey {
		terms[index] = quoteIdentifier(name) + `=?`
		values[index] = jsonataddl.SQLiteBindValue(row[name], columnTypes[strings.ToLower(name)])
	}
	executeChange(t, connection, `DELETE FROM `+quoteIdentifier(table.Name)+` WHERE `+strings.Join(terms, ` AND `), values)
}

func executeChange(t *testing.T, connection *sqlite3.Conn, statementSQL string, values []any) {
	t.Helper()
	statement, _, err := connection.Prepare(statementSQL)
	if err != nil {
		t.Fatalf("%s: %v", statementSQL, err)
	}
	defer statement.Close()
	for index, value := range values {
		if err := bindStatementValue(statement, index+1, value); err != nil {
			t.Fatalf("%s: bind %d: %v", statementSQL, index+1, err)
		}
	}
	for statement.Step() {
	}
	if err := statement.Err(); err != nil {
		t.Fatalf("%s: %v", statementSQL, err)
	}
}

// dumpTables renders every application table in primary-key order for the
// final-state golden.
func dumpTables(t *testing.T, connection *sqlite3.Conn, application *jsonataddl.Application) map[string][]map[string]any {
	t.Helper()
	result := make(map[string][]map[string]any)
	tables := application.Tables()
	for _, tableName := range sortedKeys(tables) {
		table := tables[tableName]
		columns := make([]string, len(table.Columns))
		for index, column := range table.Columns {
			columns[index] = quoteIdentifier(column.Name)
		}
		orderBy := make([]string, len(table.PrimaryKey))
		for index, key := range table.PrimaryKey {
			orderBy[index] = quoteIdentifier(key)
		}
		query := `SELECT ` + strings.Join(columns, ",") + ` FROM ` + quoteIdentifier(table.Name) + ` ORDER BY ` + strings.Join(orderBy, ",")
		statement, _, err := connection.Prepare(query)
		if err != nil {
			t.Fatalf("dump %s: %v", tableName, err)
		}
		rows := []map[string]any{}
		for statement.Step() {
			row := make(map[string]any, len(table.Columns))
			for index, column := range table.Columns {
				value, err := statementColumnValue(statement, index)
				if err != nil {
					statement.Close()
					t.Fatalf("dump %s column %s: %v", tableName, column.Name, err)
				}
				row[column.Name] = value
			}
			rows = append(rows, row)
		}
		if err := statement.Err(); err != nil {
			statement.Close()
			t.Fatalf("dump %s: %v", tableName, err)
		}
		statement.Close()
		result[tableName] = rows
	}
	return result
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
