package jsonataddl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// EvaluationInput is one program invocation: host metadata, the event under
// interpretation, and the prior rows the host read through the program's
// compiled read plan.
type EvaluationInput struct {
	Meta  map[string]any `json:"meta"`
	Event map[string]any `json:"event"`
	Rows  map[string]any `json:"rows"`
}

// TableChanges is the validated mutation plan for one table. The host
// executes it inside its own transaction; it contains only declared tables,
// operations, columns, and logical values.
type TableChanges struct {
	Insert []map[string]any `json:"insert,omitempty"`
	Upsert []map[string]any `json:"upsert,omitempty"`
	Delete []map[string]any `json:"delete,omitempty"`
}

// EvaluationResult is a program's validated outcome: the decision, bounded
// facts, validated private-event emissions, and the mutation plan.
type EvaluationResult struct {
	Decision string                      `json:"decision"`
	Facts    []map[string]any            `json:"facts"`
	Events   map[string][]map[string]any `json:"events,omitempty"`
	Tables   map[string]TableChanges     `json:"tables"`
}

type rawEvaluationResult struct {
	Decision string                       `json:"decision"`
	Facts    []json.RawMessage            `json:"facts"`
	Events   map[string][]json.RawMessage `json:"events,omitempty"`
	Tables   map[string]json.RawMessage   `json:"tables"`
}

type rawTableChangeSet struct {
	Insert []json.RawMessage `json:"insert,omitempty"`
	Upsert []json.RawMessage `json:"upsert,omitempty"`
	Delete []json.RawMessage `json:"delete,omitempty"`
}

// Evaluate runs one named program over one input and returns its validated
// result. Validation is strict: undeclared outputs, undeclared columns,
// type violations, and limit violations are errors, never partial results.
func (app *Application) Evaluate(programName string, input EvaluationInput) (EvaluationResult, error) {
	program, found := app.lookup(programName)
	if !found {
		return EvaluationResult{}, fmt.Errorf("unknown program %q", programName)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("encode evaluation input: %w", err)
	}
	if len(encoded) > app.dialect.Limits.MaxInputBytes {
		return EvaluationResult{}, fmt.Errorf("evaluation input exceeds %d bytes", app.dialect.Limits.MaxInputBytes)
	}
	output, err := program.expression.evaluate(encoded)
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("evaluate %q: %w", programName, err)
	}
	if len(output) == 0 || len(output) > app.dialect.Limits.MaxOutputBytes {
		return EvaluationResult{}, fmt.Errorf("program %q output is empty or exceeds %d bytes", programName, app.dialect.Limits.MaxOutputBytes)
	}
	result, err := app.validateOutput(program, output)
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("program %q output: %w", programName, err)
	}
	return result, nil
}

func (app *Application) validateOutput(program Program, output []byte) (EvaluationResult, error) {
	limits := app.dialect.Limits
	var raw rawEvaluationResult
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return EvaluationResult{}, fmt.Errorf("must be one JSON object with only decision, facts, events and tables: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return EvaluationResult{}, errors.New("contains trailing JSON values")
	}
	if raw.Decision != "effective" && raw.Decision != "ineffective" {
		return EvaluationResult{}, errors.New("decision must be effective or ineffective")
	}
	if raw.Facts == nil {
		return EvaluationResult{}, errors.New("facts must be an array")
	}
	if len(raw.Facts) > limits.MaxFacts {
		return EvaluationResult{}, fmt.Errorf("facts exceed %d", limits.MaxFacts)
	}
	if raw.Tables == nil {
		return EvaluationResult{}, errors.New("tables must be an object")
	}
	result := EvaluationResult{Decision: raw.Decision, Facts: make([]map[string]any, 0, len(raw.Facts)), Events: make(map[string][]map[string]any), Tables: make(map[string]TableChanges)}
	for index, fact := range raw.Facts {
		object, err := DecodeObject(fact)
		if err != nil {
			return EvaluationResult{}, fmt.Errorf("fact %d: %w", index, err)
		}
		result.Facts = append(result.Facts, object)
	}
	if !program.Normalizer && len(raw.Events) != 0 {
		return EvaluationResult{}, errors.New("analytic folds cannot emit events")
	}
	for eventName, payloads := range raw.Events {
		if !program.Normalizer || eventName != app.dialect.PrivateEvent.Name {
			return EvaluationResult{}, fmt.Errorf("undeclared event output %q", eventName)
		}
		if len(payloads) > limits.MaxEvents {
			return EvaluationResult{}, fmt.Errorf("normalized events exceed %d", limits.MaxEvents)
		}
		for index, payload := range payloads {
			row, err := DecodeObject(payload)
			if err != nil {
				return EvaluationResult{}, fmt.Errorf("event %s[%d]: %w", eventName, index, err)
			}
			if err := validateChangeRow(row, app.event.Columns, true); err != nil {
				return EvaluationResult{}, fmt.Errorf("event %s[%d]: %w", eventName, index, err)
			}
			result.Events[eventName] = append(result.Events[eventName], row)
		}
	}
	writeSet := nameSet(program.Writes...)
	rowChanges := 0
	for tableName, changesJSON := range raw.Tables {
		if !writeSet[tableName] {
			return EvaluationResult{}, fmt.Errorf("write to undeclared table %q", tableName)
		}
		table := app.tables[tableName]
		var rawChanges rawTableChangeSet
		changeDecoder := json.NewDecoder(bytes.NewReader(changesJSON))
		changeDecoder.UseNumber()
		changeDecoder.DisallowUnknownFields()
		if err := changeDecoder.Decode(&rawChanges); err != nil {
			return EvaluationResult{}, fmt.Errorf("table %q changes: %w", tableName, err)
		}
		changes := TableChanges{}
		for operation, rows := range map[string][]json.RawMessage{"insert": rawChanges.Insert, "upsert": rawChanges.Upsert, "delete": rawChanges.Delete} {
			for index, encodedRow := range rows {
				row, err := DecodeObject(encodedRow)
				if err != nil {
					return EvaluationResult{}, fmt.Errorf("table %q %s[%d]: %w", tableName, operation, index, err)
				}
				columns := table.Columns
				complete := true
				if operation == "delete" {
					columns = keyColumnsOf(table)
					complete = false
				}
				if err := validateChangeRow(row, columns, complete); err != nil {
					return EvaluationResult{}, fmt.Errorf("table %q %s[%d]: %w", tableName, operation, index, err)
				}
				switch operation {
				case "insert":
					changes.Insert = append(changes.Insert, row)
				case "upsert":
					changes.Upsert = append(changes.Upsert, row)
				case "delete":
					changes.Delete = append(changes.Delete, row)
				}
				rowChanges++
			}
		}
		result.Tables[tableName] = changes
	}
	if rowChanges > limits.MaxRowChanges {
		return EvaluationResult{}, fmt.Errorf("row changes exceed %d", limits.MaxRowChanges)
	}
	if raw.Decision == "ineffective" && (len(result.Events) != 0 || rowChanges != 0) {
		return EvaluationResult{}, errors.New("ineffective decision cannot emit events or change tables")
	}
	return result, nil
}

func validateChangeRow(row map[string]any, columns []Column, complete bool) error {
	allowed := make(map[string]Column, len(columns))
	for _, column := range columns {
		allowed[column.Name] = column
	}
	for name, value := range row {
		column, exists := allowed[name]
		if !exists {
			return fmt.Errorf("undeclared column %q", name)
		}
		if err := ValidateValue(value, column.Type, column.NotNull || column.PrimaryKey); err != nil {
			return fmt.Errorf("column %q: %w", name, err)
		}
	}
	for _, column := range columns {
		_, exists := row[column.Name]
		if (complete || column.NotNull || column.PrimaryKey) && !exists {
			return fmt.Errorf("missing column %q", column.Name)
		}
	}
	return nil
}
