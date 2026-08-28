package profile

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

type EvaluationInput struct {
	Meta  map[string]any `json:"meta"`
	Event map[string]any `json:"event"`
	Rows  map[string]any `json:"rows"`
}

type TableChanges struct {
	Insert []map[string]any `json:"insert,omitempty"`
	Upsert []map[string]any `json:"upsert,omitempty"`
	Delete []map[string]any `json:"delete,omitempty"`
}

type EvaluationResult struct {
	Decision string                      `json:"decision"`
	Facts    []map[string]any            `json:"facts"`
	Events   map[string][]map[string]any `json:"events,omitempty"`
	Tables   map[string]TableChanges     `json:"tables"`
}

type rawResult struct {
	Decision string                       `json:"decision"`
	Facts    []json.RawMessage            `json:"facts"`
	Events   map[string][]json.RawMessage `json:"events,omitempty"`
	Tables   map[string]json.RawMessage   `json:"tables"`
}

type rawTableChanges struct {
	Insert []json.RawMessage `json:"insert,omitempty"`
	Upsert []json.RawMessage `json:"upsert,omitempty"`
	Delete []json.RawMessage `json:"delete,omitempty"`
}

func (p *Profile) Evaluate(programName string, input EvaluationInput) (EvaluationResult, error) {
	program, expression, ok := p.program(programName)
	if !ok {
		return EvaluationResult{}, fmt.Errorf("unknown program %q", programName)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("encode evaluation input: %w", err)
	}
	if len(encoded) > MaxInputBytes {
		return EvaluationResult{}, fmt.Errorf("evaluation input exceeds %d bytes", MaxInputBytes)
	}
	output, err := expression.Evaluate(encoded, nil)
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("evaluate %q: %w", programName, err)
	}
	if len(output) == 0 || len(output) > MaxOutputBytes {
		return EvaluationResult{}, fmt.Errorf("program %q output is empty or exceeds %d bytes", programName, MaxOutputBytes)
	}
	result, err := p.validateOutput(program, output)
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("program %q output: %w", programName, err)
	}
	return result, nil
}

func (p *Profile) program(name string) (Program, interface {
	Evaluate([]byte, map[string]interface{}) ([]byte, error)
}, bool) {
	if p.Normalizer.Name == name {
		return p.Normalizer, p.Normalizer.expression, true
	}
	for i := range p.Folds {
		if p.Folds[i].Name == name {
			return p.Folds[i], p.Folds[i].expression, true
		}
	}
	return Program{}, nil, false
}

func (p *Profile) validateOutput(program Program, output []byte) (EvaluationResult, error) {
	var raw rawResult
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
	if len(raw.Facts) > MaxFacts {
		return EvaluationResult{}, fmt.Errorf("facts exceed %d", MaxFacts)
	}
	if raw.Tables == nil {
		return EvaluationResult{}, errors.New("tables must be an object")
	}
	result := EvaluationResult{Decision: raw.Decision, Facts: make([]map[string]any, 0, len(raw.Facts)), Events: make(map[string][]map[string]any), Tables: make(map[string]TableChanges)}
	for index, fact := range raw.Facts {
		object, err := decodeObject(fact)
		if err != nil {
			return EvaluationResult{}, fmt.Errorf("fact %d: %w", index, err)
		}
		result.Facts = append(result.Facts, object)
	}
	if !program.Normalizer && len(raw.Events) != 0 {
		return EvaluationResult{}, errors.New("analytic folds cannot emit events")
	}
	for eventName, payloads := range raw.Events {
		if !program.Normalizer || eventName != "otel_event" {
			return EvaluationResult{}, fmt.Errorf("undeclared event output %q", eventName)
		}
		if len(payloads) > MaxEvents {
			return EvaluationResult{}, fmt.Errorf("normalized events exceed %d", MaxEvents)
		}
		for index, payload := range payloads {
			row, err := decodeObject(payload)
			if err != nil {
				return EvaluationResult{}, fmt.Errorf("event %s[%d]: %w", eventName, index, err)
			}
			if err := validateRow(row, p.Event.Columns, true); err != nil {
				return EvaluationResult{}, fmt.Errorf("event %s[%d]: %w", eventName, index, err)
			}
			result.Events[eventName] = append(result.Events[eventName], row)
		}
	}
	writeSet := stringSet(program.Writes...)
	rowChanges := 0
	for tableName, changesJSON := range raw.Tables {
		if !writeSet[tableName] {
			return EvaluationResult{}, fmt.Errorf("write to undeclared table %q", tableName)
		}
		table := p.Tables[tableName]
		var rawChanges rawTableChanges
		changeDecoder := json.NewDecoder(bytes.NewReader(changesJSON))
		changeDecoder.UseNumber()
		changeDecoder.DisallowUnknownFields()
		if err := changeDecoder.Decode(&rawChanges); err != nil {
			return EvaluationResult{}, fmt.Errorf("table %q changes: %w", tableName, err)
		}
		changes := TableChanges{}
		for operation, rows := range map[string][]json.RawMessage{"insert": rawChanges.Insert, "upsert": rawChanges.Upsert, "delete": rawChanges.Delete} {
			for index, encodedRow := range rows {
				row, err := decodeObject(encodedRow)
				if err != nil {
					return EvaluationResult{}, fmt.Errorf("table %q %s[%d]: %w", tableName, operation, index, err)
				}
				columns := table.Columns
				complete := true
				if operation == "delete" {
					columns = keyColumns(table)
					complete = false
				}
				if err := validateRow(row, columns, complete); err != nil {
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
	if rowChanges > MaxRowChanges {
		return EvaluationResult{}, fmt.Errorf("row changes exceed %d", MaxRowChanges)
	}
	if raw.Decision == "ineffective" && (len(result.Events) != 0 || rowChanges != 0) {
		return EvaluationResult{}, errors.New("ineffective decision cannot emit events or change tables")
	}
	return result, nil
}

func decodeObject(encoded []byte) (map[string]any, error) {
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, errors.New("must be a JSON object")
	}
	if result == nil {
		return nil, errors.New("must be a JSON object")
	}
	return result, nil
}

func validateRow(row map[string]any, columns []Column, complete bool) error {
	allowed := make(map[string]Column, len(columns))
	for _, column := range columns {
		allowed[column.Name] = column
	}
	for name, value := range row {
		column, exists := allowed[name]
		if !exists {
			return fmt.Errorf("undeclared column %q", name)
		}
		if err := validateValue(value, column); err != nil {
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

func validateValue(value any, column Column) error {
	if value == nil {
		if column.NotNull || column.PrimaryKey {
			return errors.New("null violates NOT NULL")
		}
		return nil
	}
	switch column.Type {
	case TypeText:
		if _, ok := value.(string); !ok {
			return errors.New("must be text")
		}
	case TypeInteger:
		number, ok := value.(json.Number)
		if !ok {
			return errors.New("must be an integer")
		}
		integer, err := number.Int64()
		if err != nil || integer < -(1<<53-1) || integer > 1<<53-1 {
			return errors.New("must be an exactly representable JSON integer")
		}
	case TypeReal:
		number, ok := value.(json.Number)
		if !ok {
			return errors.New("must be a finite number")
		}
		real, err := number.Float64()
		if err != nil || math.IsInf(real, 0) || math.IsNaN(real) {
			return errors.New("must be a finite number")
		}
	case TypeBoolean:
		if _, ok := value.(bool); !ok {
			return errors.New("must be boolean")
		}
	case TypeBlob:
		text, ok := value.(string)
		if !ok {
			return errors.New("must be base64 text")
		}
		if _, err := base64.StdEncoding.DecodeString(text); err != nil {
			return errors.New("must be base64 text")
		}
	case TypeJSON:
		// Every decoded JSON value is admitted; canonicalization occurs at the
		// evaluator boundary and SQLite stores the encoded representation.
	default:
		return fmt.Errorf("unsupported logical type %q", column.Type)
	}
	return nil
}

func keyColumns(table Table) []Column {
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
