package jsonataddl

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The codec reproduces the value rules of the four existing boundaries.
// Wrapper Go types differ between today's sites (map[string]any at ingest,
// map[string]string at query); their JSON renderings are identical, and the
// codec standardizes on map[string]any.

func TestWrapIntegerMatchesIngestRule(t *testing.T) {
	cases := []struct {
		value int64
		want  any
	}{
		{0, int64(0)},
		{MaxExactInteger, int64(MaxExactInteger)},
		{MinExactInteger, int64(MinExactInteger)},
		{MaxExactInteger + 1, map[string]any{IntegerWrapperKey: "9007199254740992"}},
		{MinExactInteger - 1, map[string]any{IntegerWrapperKey: "-9007199254740992"}},
	}
	for _, tc := range cases {
		if got := WrapInteger(tc.value); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("WrapInteger(%d) = %#v, want %#v", tc.value, got, tc.want)
		}
	}
}

func TestWrapBytesSpelling(t *testing.T) {
	got := WrapBytes([]byte("hello"))
	want := map[string]any{BytesWrapperKey: "aGVsbG8="}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WrapBytes = %#v, want %#v", got, want)
	}
}

func TestValidateValueUsesContractDiagnostics(t *testing.T) {
	number := func(text string) json.Number { return json.Number(text) }
	cases := []struct {
		name    string
		value   any
		logical LogicalType
		notNull bool
		want    string // empty means accepted
	}{
		{"null nullable", nil, TypeText, false, ""},
		{"null not null", nil, TypeText, true, "null violates NOT NULL"},
		{"text ok", "x", TypeText, true, ""},
		{"text wrong", true, TypeText, true, "must be text"},
		{"integer ok", number("9007199254740991"), TypeInteger, true, ""},
		{"integer not a number", "1", TypeInteger, true, "must be an integer"},
		{"integer beyond exact range", number("9007199254740992"), TypeInteger, true, "must be an exactly representable JSON integer"},
		{"integer fractional", number("1.5"), TypeInteger, true, "must be an exactly representable JSON integer"},
		{"real ok", number("0.5"), TypeReal, true, ""},
		{"real wrong", "0.5", TypeReal, true, "must be a finite number"},
		{"boolean ok", true, TypeBoolean, true, ""},
		{"boolean wrong", "yes", TypeBoolean, true, "must be boolean"},
		{"blob ok", "aGVsbG8=", TypeBlob, true, ""},
		{"blob invalid base64", "!!!", TypeBlob, true, "must be base64 text"},
		{"blob wrong type", 5, TypeBlob, true, "must be base64 text"},
		{"json admits anything", map[string]any{"nested": []any{nil}}, TypeJSON, true, ""},
		{"unsupported type", "x", LogicalType("FANCY"), true, `unsupported logical type "FANCY"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateValue(tc.value, tc.logical, tc.notNull)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("rejected: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDecodePreservesNumbersAndRefusesNonObjects(t *testing.T) {
	value, err := DecodeCanonical([]byte(`{"n": 9007199254740991}`))
	if err != nil {
		t.Fatal(err)
	}
	object := value.(map[string]any)
	if number, ok := object["n"].(json.Number); !ok || number.String() != "9007199254740991" {
		t.Fatalf("number not preserved: %#v", object["n"])
	}
	if _, err := DecodeObject([]byte(`[1]`)); err == nil || err.Error() != "must be a JSON object" {
		t.Fatalf("non-object error = %v", err)
	}
	if _, err := DecodeObject([]byte(`null`)); err == nil || err.Error() != "must be a JSON object" {
		t.Fatalf("null object error = %v", err)
	}
}

func TestSQLiteBindValueMatchesProjectionRules(t *testing.T) {
	// JSON columns cross as JSON text even for bare numbers: affinity rule.
	if got := SQLiteBindValue(json.Number("1"), TypeJSON); got != "1" {
		t.Fatalf("JSON number bind = %#v, want JSON text", got)
	}
	if got := SQLiteBindValue(map[string]any{"k": "v"}, TypeJSON); got != `{"k":"v"}` {
		t.Fatalf("JSON object bind = %#v", got)
	}
	if got := SQLiteBindValue(json.Number("5"), TypeInteger); got != int64(5) {
		t.Fatalf("integer bind = %#v", got)
	}
	if got := SQLiteBindValue(json.Number("0.5"), TypeReal); got != 0.5 {
		t.Fatalf("real bind = %#v", got)
	}
	if got := SQLiteBindValue(true, TypeBoolean); got != 1 {
		t.Fatalf("boolean true bind = %#v", got)
	}
	if got := SQLiteBindValue(false, TypeBoolean); got != 0 {
		t.Fatalf("boolean false bind = %#v", got)
	}
	if got := SQLiteBindValue("aGVsbG8=", TypeBlob); string(got.([]byte)) != "hello" {
		t.Fatalf("blob bind = %#v", got)
	}
	if got := SQLiteBindValue(nil, TypeText); got != nil {
		t.Fatalf("null bind = %#v", got)
	}
	if got := SQLiteBindValue("plain", TypeText); got != "plain" {
		t.Fatalf("text bind = %#v", got)
	}
}

func TestLogicalColumnValueMatchesQueryRules(t *testing.T) {
	if value, err := LogicalColumnValue(SQLiteColumn{Kind: ColumnNull}, TypeText); err != nil || value != nil {
		t.Fatalf("null column = %#v, %v", value, err)
	}
	if value, _ := LogicalColumnValue(SQLiteColumn{Kind: ColumnInteger, Int: 1}, TypeBoolean); value != true {
		t.Fatalf("boolean column = %#v", value)
	}
	if value, _ := LogicalColumnValue(SQLiteColumn{Kind: ColumnInteger, Int: 42}, TypeInteger); value != int64(42) {
		t.Fatalf("integer column = %#v", value)
	}
	big, _ := LogicalColumnValue(SQLiteColumn{Kind: ColumnInteger, Int: MaxExactInteger + 1}, TypeInteger)
	if !reflect.DeepEqual(big, map[string]any{IntegerWrapperKey: "9007199254740992"}) {
		t.Fatalf("big integer column = %#v", big)
	}
	if _, err := LogicalColumnValue(SQLiteColumn{Kind: ColumnFloat, Float: inf()}, TypeReal); err == nil || err.Error() != "query returned non-finite number" {
		t.Fatalf("non-finite column error = %v", err)
	}
	decoded, err := LogicalColumnValue(SQLiteColumn{Kind: ColumnText, Text: `{"k":1}`}, TypeJSON)
	if err != nil || !reflect.DeepEqual(decoded, map[string]any{"k": float64(1)}) {
		t.Fatalf("JSON column = %#v, %v", decoded, err)
	}
	if value, _ := LogicalColumnValue(SQLiteColumn{Kind: ColumnText, Text: "x"}, TypeText); value != "x" {
		t.Fatalf("text column = %#v", value)
	}
	blob, _ := LogicalColumnValue(SQLiteColumn{Kind: ColumnBlob, Blob: []byte("hello")}, TypeBlob)
	if !reflect.DeepEqual(blob, map[string]any{BytesWrapperKey: "aGVsbG8="}) {
		t.Fatalf("blob column = %#v", blob)
	}
	if _, err := LogicalColumnValue(SQLiteColumn{Kind: SQLiteColumnKind(99)}, TypeText); err == nil || err.Error() != "query returned unsupported SQLite type" {
		t.Fatalf("unsupported kind error = %v", err)
	}
}

func TestValueRoundTripsAcrossBoundaries(t *testing.T) {
	// A validated blob crosses to SQLite as bytes and reads back as the
	// wrapper; a validated integer crosses as int64 and reads back exactly.
	if err := ValidateValue("aGVsbG8=", TypeBlob, true); err != nil {
		t.Fatal(err)
	}
	bound := SQLiteBindValue("aGVsbG8=", TypeBlob).([]byte)
	back, err := LogicalColumnValue(SQLiteColumn{Kind: ColumnBlob, Blob: bound}, TypeBlob)
	if err != nil || !reflect.DeepEqual(back, WrapBytes([]byte("hello"))) {
		t.Fatalf("blob round trip = %#v, %v", back, err)
	}

	number := json.Number("9007199254740991")
	if err := ValidateValue(number, TypeInteger, true); err != nil {
		t.Fatal(err)
	}
	boundInt := SQLiteBindValue(number, TypeInteger).(int64)
	backInt, err := LogicalColumnValue(SQLiteColumn{Kind: ColumnInteger, Int: boundInt}, TypeInteger)
	if err != nil || backInt != int64(MaxExactInteger) {
		t.Fatalf("integer round trip = %#v, %v", backInt, err)
	}
}

func inf() float64 {
	one := 1.0
	zero := 0.0
	return one / zero
}
