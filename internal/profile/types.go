// Package profile compiles the deliberately small DDL/JSONata language used
// by Tailapp applications. A compiled Profile is immutable and safe to share
// between the projection and query subsystems.
package profile

import jsonata "github.com/jsonata-go/jsonata/v206"

const (
	// RuntimeID binds the evaluator, DDL grammar, numeric rules and limits that
	// give a revision its meaning. Change it whenever any of those change.
	RuntimeID = "tailapp-otlp-1.8-json-v1-ddl-jsonata-v206-sqlite-3.53.4@2"

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

	// EvaluationWallTimeMilliseconds is only an outer process-safety net. It
	// is deliberately part of RuntimeID, but active programs are also confined
	// to the statically admitted profile; wall time is not a deterministic
	// evaluation budget.
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

	expression *jsonata.Expression
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

func (p *Profile) Expression(program string) (*jsonata.Expression, bool) {
	if p.Normalizer.Name == program {
		return p.Normalizer.expression, true
	}
	for i := range p.Folds {
		if p.Folds[i].Name == program {
			return p.Folds[i].expression, true
		}
	}
	return nil, false
}
