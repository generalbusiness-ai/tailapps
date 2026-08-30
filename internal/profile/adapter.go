package profile

import (
	"io/fs"

	"github.com/generalbusiness-ai/tailapps/jsonataddl"
)

// LoadViaCore compiles a source set through the extracted jsonataddl core
// under the Tailapp dialect and the current legacy runtime identity. The
// returned Profile carries the same inspection data and identity digests as
// Load produces, and its Evaluate delegates to the immutable core handle.
// This is the narrow adapter of migration stage 4: the conformance corpus
// runs against both implementations differentially through it, and later
// stages move callers onto core handles before the duplicate rules are
// removed.
func LoadViaCore(files fs.FS, root, name string) (*Profile, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	application, err := jsonataddl.LoadApplication(files, root, name, jsonataddl.Tailapp(), RuntimeID)
	if err != nil {
		return nil, err
	}
	return profileFromCore(application), nil
}

func profileFromCore(application *jsonataddl.Application) *Profile {
	result := &Profile{
		Name:                 application.Name(),
		Revision:             application.Revision(),
		RuntimeProfile:       application.RuntimeProfile(),
		StorageSchemaDigest:  application.StorageSchemaDigest(),
		ExportContractDigest: application.ExportContractDigest(),
		Event:                eventFromCore(application.Event()),
		Normalizer:           programFromCore(application.NormalizerProgram()),
		Tables:               make(map[string]Table),
		Views:                make(map[string]View),
		Exports:              make(map[string]Export),
		SchemaSQL:            application.SchemaSQL(),
		ReplaceableSQL:       application.ReplaceableSQL(),
		Sources:              application.Sources(),
		core:                 application,
	}
	for _, fold := range application.Folds() {
		result.Folds = append(result.Folds, programFromCore(fold))
	}
	for name, table := range application.Tables() {
		result.Tables[name] = tableFromCore(table)
	}
	for name, view := range application.Views() {
		result.Views[name] = View{Name: view.Name, SQL: view.SQL, Dependencies: view.Dependencies}
	}
	for name, export := range application.Exports() {
		columns := make([]ExportColumn, len(export.Columns))
		for index, column := range export.Columns {
			columns[index] = ExportColumn{Name: column.Name, Type: LogicalType(column.Type)}
		}
		result.Exports[name] = Export{Name: export.Name, SQL: export.SQL, Columns: columns, ContractDigest: export.ContractDigest}
	}
	return result
}

func eventFromCore(event jsonataddl.Event) Event {
	return Event{Name: event.Name, Columns: columnsFromCore(event.Columns)}
}

func columnsFromCore(columns []jsonataddl.Column) []Column {
	result := make([]Column, len(columns))
	for index, column := range columns {
		result[index] = Column{Name: column.Name, Type: LogicalType(column.Type), NotNull: column.NotNull, PrimaryKey: column.PrimaryKey}
	}
	return result
}

func tableFromCore(table jsonataddl.Table) Table {
	return Table{
		Name:         table.Name,
		Columns:      columnsFromCore(table.Columns),
		PrimaryKey:   table.PrimaryKey,
		UniqueKeys:   table.UniqueKeys,
		StorageShape: table.StorageShape,
		SQL:          table.SQL,
		Writer:       table.Writer,
	}
}

func programFromCore(program jsonataddl.Program) Program {
	result := Program{
		Name:       program.Name,
		Event:      program.Event,
		Path:       program.Path,
		Writes:     program.Writes,
		Emits:      program.Emits,
		Normalizer: program.Normalizer,
	}
	for _, read := range program.Reads {
		result.Reads = append(result.Reads, Read{
			Name:        read.Name,
			Cardinality: Cardinality(read.Cardinality),
			Limit:       read.Limit,
			SQL:         read.SQL,
			Table:       read.Table,
			Columns:     read.Columns,
			Parameters:  read.Parameters,
			OrderBy:     read.OrderBy,
		})
	}
	return result
}

func evaluateViaCore(application *jsonataddl.Application, programName string, input EvaluationInput) (EvaluationResult, error) {
	coreResult, err := application.Evaluate(programName, jsonataddl.EvaluationInput{Meta: input.Meta, Event: input.Event, Rows: input.Rows})
	if err != nil {
		return EvaluationResult{}, err
	}
	result := EvaluationResult{Decision: coreResult.Decision, Facts: coreResult.Facts, Events: coreResult.Events, Tables: make(map[string]TableChanges, len(coreResult.Tables))}
	for name, changes := range coreResult.Tables {
		result.Tables[name] = TableChanges{Insert: changes.Insert, Upsert: changes.Upsert, Delete: changes.Delete}
	}
	return result, nil
}
