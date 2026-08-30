package jsonataddl

import (
	"strings"

	"github.com/ncruces/go-sqlite3"
)

// Authorizer is a SQLite authorizer callback in the pinned driver's shape.
// The host installs it on the connection that executes a compiled read
// plan; the core never opens or owns that connection.
type Authorizer = func(action sqlite3.AuthorizerActionCode, relation, column, schema, inner string) sqlite3.AuthorizerReturnCode

// ReadAuthorizer returns the default-deny authorizer for executing the
// named program's compiled read plan. The allowlist derives from the plan
// and the compiled schema: only the relations the plan names, plus the base
// tables they resolve to through declared views, are readable; table reads
// are confined to declared columns; SQL functions, pragmas, writes, other
// schemas, and every other action are denied. This enforces the fold-read
// policy at execution time, not only as a compile-time textual check.
func (app *Application) ReadAuthorizer(programName string) (Authorizer, bool) {
	program, found := app.lookup(programName)
	if !found {
		return nil, false
	}
	// admitted maps a lowercased relation name to its admitted column set;
	// a nil set admits any column (views, whose projected columns are not
	// part of the stored schema).
	admitted := make(map[string]map[string]bool)
	for _, read := range program.Reads {
		app.admitRelation(admitted, read.Table, nil)
	}
	return func(action sqlite3.AuthorizerActionCode, relation, column, schema, inner string) sqlite3.AuthorizerReturnCode {
		switch action {
		case sqlite3.AUTH_SELECT:
			return sqlite3.AUTH_OK
		case sqlite3.AUTH_READ:
			if schema != "" && schema != "main" {
				return sqlite3.AUTH_DENY
			}
			columns, ok := admitted[strings.ToLower(relation)]
			if !ok {
				return sqlite3.AUTH_DENY
			}
			if column == "" || columns == nil || columns[strings.ToLower(column)] {
				return sqlite3.AUTH_OK
			}
			return sqlite3.AUTH_DENY
		default:
			return sqlite3.AUTH_DENY
		}
	}, true
}

// admitRelation admits one relation and, through declared views, the base
// tables it resolves to. Cycles cannot occur in a compiled application;
// the seen set still bounds the walk defensively.
func (app *Application) admitRelation(admitted map[string]map[string]bool, name string, seen map[string]bool) {
	key := strings.ToLower(name)
	if seen == nil {
		seen = make(map[string]bool)
	}
	if seen[key] {
		return
	}
	seen[key] = true
	if table, exists := app.tables[name]; exists {
		columns := make(map[string]bool, len(table.Columns))
		for _, column := range table.Columns {
			columns[strings.ToLower(column.Name)] = true
		}
		admitted[key] = columns
		return
	}
	view, exists := app.views[name]
	if !exists {
		return
	}
	admitted[key] = nil
	for _, dependency := range view.Dependencies {
		app.admitRelation(admitted, dependency, seen)
	}
}
