// Package profile is the Tailapp host façade over the extracted jsonataddl
// core: it selects the Tailapp dialect, assembles the composed runtime
// identity, and presents compiled applications as immutable Profile values
// whose evaluation delegates to the core handle. The compile-and-evaluate
// behavior itself lives in the core; since migration stage 6 this package
// carries no duplicate of it.
package profile

import "github.com/generalbusiness-ai/tailapps/jsonataddl"

const (
	// RuntimeID is the legacy runtime identity: the repository-global
	// string every pre-switchover revision and projection recorded. It
	// remains only so those projections stay recognizable and queryable
	// through the retained legacy resolver (Load) until their explicit
	// continue-or-reset upgrade; new identity is CurrentRuntimeID.
	RuntimeID = "tailapp-otlp-1.8-json-v1-ddl-jsonata-v206-sqlite-3.53.4@5"

	// The evaluation and source bounds, mirrored from the Tailapp dialect
	// (jsonataddl.Tailapp); the consistency tests bind them to the values
	// the runtime enforces.
	MaxElementBytes = 64 << 10
	MaxSourceBytes  = 512 << 10
	MaxProgramBytes = 64 << 10
	MaxInputBytes   = 256 << 10
	MaxOutputBytes  = 256 << 10
	MaxDepth        = 64
	MaxRange        = 4096
	MaxEvents       = 256
	MaxFacts        = 64
	MaxRowChanges   = 1024
	MaxManyRows     = 1024

	// EvaluationWallTimeMilliseconds is only an outer process-safety net,
	// enforced by the core evaluator. It is deliberately part of runtime
	// identity; wall time is not a deterministic evaluation budget.
	EvaluationWallTimeMilliseconds = 250
)

type LogicalType string

const (
	TypeText    LogicalType = "TEXT"
	TypeInteger LogicalType = "INTEGER"
	TypeReal    LogicalType = "REAL"
	TypeBlob    LogicalType = "BLOB"
	TypeBoolean LogicalType = "BOOLEAN"
	TypeJSON    LogicalType = "JSON"
)

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

type Profile struct {
	// core is the compiled application handle every Profile is backed by;
	// evaluation and the read authorizer delegate to it.
	core *jsonataddl.Application

	Name                 string            `json:"name"`
	Revision             string            `json:"revision"`
	RuntimeProfile       string            `json:"runtime_profile"`
	StorageSchemaDigest  string            `json:"storage_schema_digest"`
	ExportContractDigest string            `json:"export_contract_digest"`
	Event                Event             `json:"event"`
	Normalizer           Program           `json:"normalizer"`
	Folds                []Program         `json:"folds"`
	Tables               map[string]Table  `json:"tables"`
	Views                map[string]View   `json:"views"`
	Exports              map[string]Export `json:"exports"`
	SchemaSQL            []string          `json:"schema_sql"`
	ReplaceableSQL       []string          `json:"replaceable_sql"`
	Sources              map[string][]byte `json:"-"`
}

// ReadAuthorizer is the default-deny SQLite authorizer for executing the
// named program's compiled read plan, derived by the core from the plan
// and schema.
func (p *Profile) ReadAuthorizer(program string) (jsonataddl.Authorizer, bool) {
	if p.core == nil {
		return nil, false
	}
	return p.core.ReadAuthorizer(program)
}
