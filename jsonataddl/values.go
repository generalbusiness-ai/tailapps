package jsonataddl

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
)

// The logical value codec: the one implementation of the value rules that
// today appear independently at four product boundaries (OTLP
// canonicalization, fold output validation, projection writes, query
// reads). Hosts still decide what their native input means; they construct
// and convert values through this codec instead of duplicating its bounds,
// wrapper spellings, or diagnostics. Validation diagnostics use the exact
// contract-facing spellings the conformance corpus freezes, so replacing
// the evaluator's copy is diagnostic-identical.

// LogicalType is a declared column type in the shared value model.
type LogicalType string

const (
	TypeText    LogicalType = "TEXT"
	TypeInteger LogicalType = "INTEGER"
	TypeReal    LogicalType = "REAL"
	TypeBoolean LogicalType = "BOOLEAN"
	TypeBlob    LogicalType = "BLOB"
	TypeJSON    LogicalType = "JSON"
)

// The exactly representable JSON integer range.
const (
	MinExactInteger = -(1<<53 - 1)
	MaxExactInteger = 1<<53 - 1
)

// The lossless wrapper spellings for values outside JSON's direct scalar
// model. These exact keys are the wire contract.
const (
	IntegerWrapperKey = "integer_decimal"
	BytesWrapperKey   = "bytes_base64"
)

// WrapInteger renders an int64 as a JSON-safe value: the number itself
// inside the exact range, the decimal-string wrapper outside it.
func WrapInteger(value int64) any {
	if value < MinExactInteger || value > MaxExactInteger {
		return map[string]any{IntegerWrapperKey: strconv.FormatInt(value, 10)}
	}
	return value
}

// WrapBytes renders raw bytes as the base64 wrapper object.
func WrapBytes(value []byte) any {
	return map[string]any{BytesWrapperKey: base64.StdEncoding.EncodeToString(value)}
}

// FiniteNumber admits a float64 only when it is finite.
func FiniteNumber(value float64) (float64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("non-finite number")
	}
	return value, nil
}

// ValidateValue checks one decoded JSON value against a declared logical
// type and null policy, with the contract-facing diagnostics the corpus
// freezes. Numbers must arrive as json.Number (number-preserving decoding);
// blob values are base64 text; JSON columns admit every decoded value.
func ValidateValue(value any, logical LogicalType, notNull bool) error {
	if value == nil {
		if notNull {
			return errors.New("null violates NOT NULL")
		}
		return nil
	}
	switch logical {
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
		if err != nil || integer < MinExactInteger || integer > MaxExactInteger {
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
		// Every decoded JSON value is admitted; canonicalization occurs at
		// the decoding boundary and SQLite stores the encoded form.
	default:
		return fmt.Errorf("unsupported logical type %q", logical)
	}
	return nil
}

// DecodeCanonical decodes one JSON value with number preservation: every
// number arrives as json.Number, exactly as the evaluator and validator
// expect.
func DecodeCanonical(encoded []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// DecodeObject decodes one JSON object with number preservation, with the
// contract-facing diagnostic for non-objects.
func DecodeObject(encoded []byte) (map[string]any, error) {
	value, err := DecodeCanonical(encoded)
	if err != nil {
		return nil, errors.New("must be a JSON object")
	}
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return nil, errors.New("must be a JSON object")
	}
	return object, nil
}

// SQLiteBindValue converts a validated logical value to the value bound
// into SQLite, by declared type. JSON columns always cross as JSON text -
// handling json.Number first would give a top-level JSON number
// INTEGER/REAL affinity and make the next declared read reject otherwise
// valid state. Booleans store as 0/1; blob base64 text stores as raw
// bytes.
func SQLiteBindValue(value any, logical LogicalType) any {
	if value == nil {
		return nil
	}
	if logical == TypeJSON {
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}
	if number, ok := value.(json.Number); ok {
		if integer, err := number.Int64(); err == nil {
			return integer
		}
		if real, err := number.Float64(); err == nil {
			return real
		}
		return number.String()
	}
	switch logical {
	case TypeBoolean:
		if typed, ok := value.(bool); ok {
			if typed {
				return 1
			}
			return 0
		}
	case TypeBlob:
		if typed, ok := value.(string); ok {
			decoded, _ := base64.StdEncoding.DecodeString(typed)
			return decoded
		}
	}
	return value
}

// SQLiteColumnKind is the storage class a read value arrived with.
type SQLiteColumnKind int

const (
	ColumnNull SQLiteColumnKind = iota
	ColumnInteger
	ColumnFloat
	ColumnText
	ColumnBlob
)

// SQLiteColumn is one read column value, decoupled from any driver.
type SQLiteColumn struct {
	Kind  SQLiteColumnKind
	Int   int64
	Float float64
	Text  string
	Blob  []byte
}

// LogicalColumnValue converts one SQLite read value to its logical JSON
// form by declared type: BOOLEAN integers become booleans, integers beyond
// the exact range take the integer wrapper, JSON text decodes, blobs take
// the bytes wrapper, and non-finite floats are refused.
func LogicalColumnValue(column SQLiteColumn, declared LogicalType) (any, error) {
	switch column.Kind {
	case ColumnNull:
		return nil, nil
	case ColumnInteger:
		if declared == TypeBoolean {
			return column.Int != 0, nil
		}
		return WrapInteger(column.Int), nil
	case ColumnFloat:
		if _, err := FiniteNumber(column.Float); err != nil {
			return nil, errors.New("query returned non-finite number")
		}
		return column.Float, nil
	case ColumnText:
		if declared == TypeJSON {
			var decoded any
			if err := json.Unmarshal([]byte(column.Text), &decoded); err != nil {
				return nil, err
			}
			return decoded, nil
		}
		return column.Text, nil
	case ColumnBlob:
		return WrapBytes(column.Blob), nil
	default:
		return nil, errors.New("query returned unsupported SQLite type")
	}
}
