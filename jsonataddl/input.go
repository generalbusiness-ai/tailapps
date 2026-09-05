package jsonataddl

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ValidateProgramInput checks metadata and the complete event before a host
// binds read parameters. Evaluate repeats these checks on the full encoded
// input and also checks the read results; this method grants no bypass.
func (app *Application) ValidateProgramInput(programName string, meta, event map[string]any) error {
	program, found := app.lookup(programName)
	if !found {
		return fmt.Errorf("unknown program %q", programName)
	}
	encoded, err := json.Marshal(struct {
		Meta  map[string]any `json:"meta"`
		Event map[string]any `json:"event"`
	}{meta, event})
	if err != nil {
		return fmt.Errorf("encode program input: %w", err)
	}
	return app.validateInput(program, encoded, false)
}

func (app *Application) validateInput(program Program, encoded []byte, withRows bool) error {
	if len(encoded) > app.dialect.Limits.MaxInputBytes {
		return fmt.Errorf("evaluation input exceeds %d bytes", app.dialect.Limits.MaxInputBytes)
	}
	if err := inputDepth(encoded, app.dialect.Limits.MaxInputDepth); err != nil {
		return err
	}
	input, err := DecodeObject(encoded)
	if err != nil {
		return fmt.Errorf("decode evaluation input: %w", err)
	}
	if err := validateInputObject(input["meta"], app.dialect.Input.Meta.fields, app.dialect.Input.Meta.nullable); err != nil {
		return fmt.Errorf("input meta: %w", err)
	}
	fields, nullable := columnInputFields(app.event.Columns), false
	if program.Normalizer {
		fields = append(scalarInputFields(app.dialect.HostEvent.Fields()), app.dialect.Input.Event.fields...)
		nullable = app.dialect.Input.Event.nullable
	}
	if err := validateInputObject(input["event"], fields, nullable); err != nil {
		return fmt.Errorf("input event: %w", err)
	}
	if withRows {
		return app.validateInputRows(program, input["rows"])
	}
	return nil
}

// The encoding is produced by json.Marshal, which rejects invalid JSON from
// custom marshalers. Count its containers without recursively decoding it, so
// depth is checked before either the JSON decoder or the contract walk.
func inputDepth(encoded []byte, maximum int) error {
	depth, quoted, escaped := 0, false, false
	for _, b := range encoded {
		if quoted {
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				quoted = false
			}
			continue
		}
		switch b {
		case '"':
			quoted = true
		case '{', '[':
			depth++
			if depth > maximum {
				return fmt.Errorf("evaluation input exceeds depth %d", maximum)
			}
		case '}', ']':
			depth--
		}
	}
	return nil
}

func scalarInputFields(fields []EnvelopeField) []InputField {
	result := make([]InputField, len(fields))
	for i, field := range fields {
		result[i] = InputField{Name: field.Name, Kind: InputScalar, Type: field.Type, Optional: field.Optional, Nullable: field.Nullable}
	}
	return result
}

func columnInputFields(columns []Column) []InputField {
	result := make([]InputField, len(columns))
	for i, column := range columns {
		result[i] = InputField{Name: column.Name, Kind: InputScalar, Type: string(column.Type), Nullable: !column.NotNull}
	}
	return result
}

func validateInputObject(value any, fields []InputField, nullable bool) error {
	if value == nil && nullable {
		return nil
	}
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return fmt.Errorf("must be a non-null object")
	}
	// Declaration order does not enter identity; refusal order must not depend
	// on it either. Copy only the top-level slice; nested declarations stay read-only.
	ordered := append([]InputField(nil), fields...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	known := make(map[string]bool, len(fields))
	for _, field := range ordered {
		known[field.Name] = true
		member, present := object[field.Name]
		if !present {
			if field.Optional {
				continue
			}
			return fmt.Errorf("field %q is required", field.Name)
		}
		if member == nil && field.Nullable {
			continue
		}
		if err := validateInputMember(member, field); err != nil {
			return fmt.Errorf("field %q: %w", field.Name, err)
		}
	}
	for _, name := range sortedInputKeys(object) {
		if !known[name] {
			return fmt.Errorf("unknown field %q", name)
		}
	}
	return nil
}

func validateInputMember(value any, field InputField) error {
	switch field.Kind {
	case InputScalar:
		return ValidateValue(value, LogicalType(field.Type), !field.Nullable)
	case InputStringArray:
		array, ok := value.([]any)
		if !ok || array == nil {
			return fmt.Errorf("must be a non-null string array")
		}
		for _, member := range array {
			if _, ok := member.(string); !ok {
				return fmt.Errorf("must contain only strings")
			}
		}
		return nil
	case InputScalarObject:
		return validateInputObject(value, scalarInputFields(field.Members), field.Nullable)
	case InputJSONObject:
		if object, ok := value.(map[string]any); !ok || object == nil {
			return fmt.Errorf("must be a non-null JSON object")
		}
		// Nested members were already decoded as byte/depth-bounded JSON.
		// Do not impose scalar finite-number rules on opaque JSON numbers.
		return nil
	default:
		return fmt.Errorf("unsupported input form %q", field.Kind)
	}
}

func sortedInputKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (app *Application) validateInputRows(program Program, value any) error {
	rows, ok := value.(map[string]any)
	if !ok || rows == nil {
		return fmt.Errorf("input rows must be a non-null object")
	}
	known := make(map[string]bool, len(program.Reads))
	for _, read := range program.Reads {
		known[read.Name] = true
		value, present := rows[read.Name]
		if !present {
			return fmt.Errorf("input rows: read %q is required", read.Name)
		}
		fields := make([]InputField, 0, len(read.Columns))
		table, isTable := app.tables[read.Table]
		for _, name := range read.Columns {
			field := InputField{Name: name, Kind: InputScalar, Type: string(TypeJSON), Nullable: true}
			if isTable {
				for _, column := range table.Columns {
					if strings.EqualFold(column.Name, name) {
						field.Type, field.Nullable = string(column.Type), !column.NotNull
						break
					}
				}
			}
			fields = append(fields, field)
		}
		check := func(value any) error {
			if err := validateInputObject(value, fields, false); err != nil {
				return fmt.Errorf("input rows read %q: %w", read.Name, err)
			}
			return nil
		}
		switch read.Cardinality {
		case OptionalOne:
			if value == nil {
				continue
			}
			fallthrough
		case One:
			if err := check(value); err != nil {
				return err
			}
		case Many:
			array, ok := value.([]any)
			if !ok || array == nil || len(array) > read.Limit {
				return fmt.Errorf("input rows read %q must be an array of at most %d rows", read.Name, read.Limit)
			}
			for _, row := range array {
				if err := check(row); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("input rows read %q has unsupported cardinality", read.Name)
		}
	}
	for _, name := range sortedInputKeys(rows) {
		if !known[name] {
			return fmt.Errorf("input rows: unknown read %q", name)
		}
	}
	return nil
}
