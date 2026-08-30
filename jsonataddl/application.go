package jsonataddl

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	jsonata "github.com/jsonata-go/jsonata/v206"
)

// The schema and program types mirror the logical application definition.
// Their JSON encodings are load-bearing: the storage-schema and export
// contract digests are computed over them, so field names and order must
// stay stable.

type Column struct {
	Name       string      `json:"name"`
	Type       LogicalType `json:"type"`
	NotNull    bool        `json:"not_null"`
	PrimaryKey bool        `json:"primary_key"`
}

type Table struct {
	Name         string     `json:"name"`
	Columns      []Column   `json:"columns"`
	PrimaryKey   []string   `json:"primary_key"`
	UniqueKeys   [][]string `json:"unique_keys"`
	StorageShape string     `json:"storage_shape"`
	SQL          string     `json:"sql"`
	Writer       string     `json:"writer"`
}

type Event struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
}

type View struct {
	Name         string   `json:"name"`
	SQL          string   `json:"sql"`
	Dependencies []string `json:"dependencies"`
}

type Cardinality string

const (
	One         Cardinality = "ONE"
	OptionalOne Cardinality = "OPTIONAL ONE"
	Many        Cardinality = "MANY"
)

type Read struct {
	Name        string      `json:"name"`
	Cardinality Cardinality `json:"cardinality"`
	Limit       int         `json:"limit,omitempty"`
	SQL         string      `json:"sql"`
	Table       string      `json:"table"`
	Columns     []string    `json:"columns"`
	Parameters  []string    `json:"parameters"`
	OrderBy     []string    `json:"order_by,omitempty"`
}

type Program struct {
	Name       string   `json:"name"`
	Event      string   `json:"event"`
	Path       string   `json:"path"`
	Reads      []Read   `json:"reads"`
	Writes     []string `json:"writes"`
	Emits      string   `json:"emits,omitempty"`
	Normalizer bool     `json:"normalizer"`

	expression *compiledExpression
}

// compiledExpression owns one compiled JSONata program together with the
// lock that makes evaluation safe for concurrent callers: the pinned
// evaluator mutates per-expression state (environment frames, timestamp,
// timeout start, depth) during Evaluate, so evaluations of one program
// must serialize. Program values copy the pointer, so every copy shares
// the same lock and the guarantee holds across accessor copies.
type compiledExpression struct {
	mu         sync.Mutex
	expression *jsonata.Expression
}

func (compiled *compiledExpression) evaluate(input []byte) ([]byte, error) {
	compiled.mu.Lock()
	defer compiled.mu.Unlock()
	return compiled.expression.Evaluate(input, nil)
}

type ExportColumn struct {
	Name string      `json:"name"`
	Type LogicalType `json:"type"`
}

type Export struct {
	Name           string         `json:"name"`
	SQL            string         `json:"sql"`
	Columns        []ExportColumn `json:"columns"`
	ContractDigest string         `json:"contract_digest"`
}

// Application is an immutable compiled handle: inspection data, evaluation
// methods, runtime identity, compatibility information, and read plus
// mutation plans. Every accessor returns an independent copy, so no caller
// can mutate compiled state; the handle is safe for concurrent use, with
// evaluations of one program serializing on that program's lock (see
// compiledExpression).
type Application struct {
	name                 string
	revision             string
	runtimeProfile       string
	storageSchemaDigest  string
	exportContractDigest string
	dialect              Dialect
	event                Event
	normalizer           Program
	folds                []Program
	tables               map[string]Table
	views                map[string]View
	exports              map[string]Export
	schemaSQL            []string
	replaceableSQL       []string
	sources              map[string][]byte
}

func (app *Application) Name() string                 { return app.name }
func (app *Application) Revision() string             { return app.revision }
func (app *Application) RuntimeProfile() string       { return app.runtimeProfile }
func (app *Application) StorageSchemaDigest() string  { return app.storageSchemaDigest }
func (app *Application) ExportContractDigest() string { return app.exportContractDigest }
func (app *Application) Dialect() Dialect             { return app.dialect }

func (app *Application) Event() Event { return copyEvent(app.event) }

func (app *Application) NormalizerProgram() Program { return copyProgram(app.normalizer) }

func (app *Application) Folds() []Program {
	result := make([]Program, len(app.folds))
	for index, fold := range app.folds {
		result[index] = copyProgram(fold)
	}
	return result
}

func (app *Application) Tables() map[string]Table {
	result := make(map[string]Table, len(app.tables))
	for name, table := range app.tables {
		result[name] = copyTable(table)
	}
	return result
}

func (app *Application) Views() map[string]View {
	result := make(map[string]View, len(app.views))
	for name, view := range app.views {
		result[name] = View{Name: view.Name, SQL: view.SQL, Dependencies: copyStrings(view.Dependencies)}
	}
	return result
}

func (app *Application) Exports() map[string]Export {
	result := make(map[string]Export, len(app.exports))
	for name, export := range app.exports {
		result[name] = copyExport(export)
	}
	return result
}

func (app *Application) SchemaSQL() []string      { return copyStrings(app.schemaSQL) }
func (app *Application) ReplaceableSQL() []string { return copyStrings(app.replaceableSQL) }

func (app *Application) Sources() map[string][]byte {
	result := make(map[string][]byte, len(app.sources))
	for name, content := range app.sources {
		result[name] = append([]byte(nil), content...)
	}
	return result
}

// ReadPlan returns the named program's immutable compiled read plan.
func (app *Application) ReadPlan(programName string) ([]Read, bool) {
	program, found := app.lookup(programName)
	if !found {
		return nil, false
	}
	return copyReads(program.Reads), true
}

func (app *Application) lookup(name string) (Program, bool) {
	if app.normalizer.Name == name {
		return app.normalizer, true
	}
	for _, fold := range app.folds {
		if fold.Name == name {
			return fold, true
		}
	}
	return Program{}, false
}

// ContinueCompatible checks the storage rule for a continue activation:
// every existing writable table must retain its complete stored shape. The
// next revision may add tables; the host creates those empty at its
// delivery boundary.
func ContinueCompatible(existing, next *Application) error {
	if existing == nil || next == nil {
		return fmt.Errorf("both existing and next applications are required")
	}
	for name, prior := range existing.tables {
		candidate, ok := next.tables[name]
		if !ok {
			return fmt.Errorf("existing writable table %q was removed", name)
		}
		prior.SQL, prior.Writer = "", ""
		candidate.SQL, candidate.Writer = "", ""
		priorJSON, _ := json.Marshal(prior)
		candidateJSON, _ := json.Marshal(candidate)
		if string(priorJSON) != string(candidateJSON) {
			return fmt.Errorf("existing writable table %q changed stored shape", name)
		}
	}
	return nil
}

func copyEvent(event Event) Event {
	return Event{Name: event.Name, Columns: append([]Column(nil), event.Columns...)}
}

func copyTable(table Table) Table {
	result := table
	result.Columns = append([]Column(nil), table.Columns...)
	result.PrimaryKey = copyStrings(table.PrimaryKey)
	result.UniqueKeys = make([][]string, len(table.UniqueKeys))
	for index, key := range table.UniqueKeys {
		result.UniqueKeys[index] = copyStrings(key)
	}
	return result
}

func copyProgram(program Program) Program {
	result := program
	result.Reads = copyReads(program.Reads)
	result.Writes = copyStrings(program.Writes)
	return result
}

func copyReads(reads []Read) []Read {
	result := make([]Read, len(reads))
	for index, read := range reads {
		copied := read
		copied.Columns = copyStrings(read.Columns)
		copied.Parameters = copyStrings(read.Parameters)
		copied.OrderBy = copyStrings(read.OrderBy)
		result[index] = copied
	}
	return result
}

func copyExport(export Export) Export {
	result := export
	result.Columns = append([]ExportColumn(nil), export.Columns...)
	return result
}

func copyStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func keyColumnsOf(table Table) []Column {
	byName := make(map[string]Column, len(table.Columns))
	for _, column := range table.Columns {
		byName[strings.ToLower(column.Name)] = column
	}
	result := make([]Column, 0, len(table.PrimaryKey))
	for _, name := range table.PrimaryKey {
		result = append(result, byName[strings.ToLower(name)])
	}
	return result
}
