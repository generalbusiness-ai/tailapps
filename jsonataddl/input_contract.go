package jsonataddl

import "fmt"

// InputKind is one of the four supported input member forms. Objects cannot
// recursively declare another contract; JSON object contents remain host-owned.
type InputKind string

const (
	InputScalar       InputKind = "scalar"
	InputStringArray  InputKind = "string-array"
	InputScalarObject InputKind = "scalar-object"
	InputJSONObject   InputKind = "json-object"
)

type InputField struct {
	Name     string
	Kind     InputKind
	Type     string // Only InputScalar uses Type.
	Optional bool
	Nullable bool
	Members  []EnvelopeField // Only InputScalarObject uses Members.
}

// ObjectContract is an immutable closed object declaration. Use the constructor
// even for an empty object: the zero value means no contract was supplied.
type ObjectContract struct {
	specified bool
	nullable  bool
	fields    []InputField
}

func NewObjectContract(nullable bool, fields ...InputField) ObjectContract {
	return ObjectContract{specified: true, nullable: nullable, fields: copyInputFields(fields)}
}

func (contract ObjectContract) Nullable() bool       { return contract.nullable }
func (contract ObjectContract) Fields() []InputField { return copyInputFields(contract.fields) }

func copyInputFields(fields []InputField) []InputField {
	owned := make([]InputField, len(fields))
	for i, field := range fields {
		owned[i] = field
		owned[i].Members = append([]EnvelopeField(nil), field.Members...)
	}
	return owned
}

// InputContract declares both stages' metadata and the normalizer event's
// structured members. Event's scalar members come from HostEvent, which remains
// the sole declaration of scalar SQL read parameters. Event also states the
// complete normalizer event's root null policy.
type InputContract struct {
	Meta  ObjectContract
	Event ObjectContract
}

func validateScalarFields(fields []EnvelopeField) error {
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if !identifierRE.MatchString(field.Name) || seen[field.Name] {
			return fmt.Errorf("invalid or duplicate scalar field %q", field.Name)
		}
		seen[field.Name] = true
		switch LogicalType(field.Type) {
		case TypeText, TypeInteger, TypeReal, TypeBoolean, TypeBlob:
		default:
			return fmt.Errorf("scalar field %q has unsupported type %q", field.Name, field.Type)
		}
	}
	return nil
}

func validateInputContract(dialect Dialect) error {
	if err := validateScalarFields(dialect.HostEvent.Fields()); err != nil {
		return err
	}
	for _, part := range []struct {
		name     string
		contract ObjectContract
	}{
		{"meta", dialect.Input.Meta}, {"event", dialect.Input.Event},
	} {
		if !part.contract.specified {
			return fmt.Errorf("input %s contract must be declared", part.name)
		}
		seen := make(map[string]bool)
		if part.name == "event" {
			for _, field := range dialect.HostEvent.Fields() {
				seen[field.Name] = true
			}
		}
		for _, field := range part.contract.fields {
			if !identifierRE.MatchString(field.Name) || seen[field.Name] {
				return fmt.Errorf("input %s has invalid, duplicate or colliding field %q", part.name, field.Name)
			}
			seen[field.Name] = true
			if part.name == "meta" && field.Kind != InputScalar || part.name == "event" && field.Kind == InputScalar {
				return fmt.Errorf("input %s field %q uses an inadmissible form", part.name, field.Name)
			}
			if field.Kind != InputScalar && field.Type != "" || field.Kind != InputScalarObject && len(field.Members) != 0 {
				return fmt.Errorf("input field %q has fields outside its form", field.Name)
			}
			switch field.Kind {
			case InputScalar:
				if err := validateScalarFields([]EnvelopeField{{Name: field.Name, Type: field.Type}}); err != nil {
					return err
				}
			case InputScalarObject:
				if err := validateScalarFields(field.Members); err != nil {
					return err
				}
			case InputStringArray, InputJSONObject:
			default:
				return fmt.Errorf("input field %q has unsupported form %q", field.Name, field.Kind)
			}
		}
	}
	return nil
}
