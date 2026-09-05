package jsonataddl

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

func inputTestSources() fstest.MapFS {
	sources := renamedSources()
	sources["app.ddl"].Data = append(sources["app.ddl"].Data, []byte("\nCREATE EXPORT tallies AS SELECT key,total FROM tallies;\n")...)
	return sources
}

func readInputTestSources(t *testing.T) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{}
	for _, name := range []string{"application.sql", "folds/normalize.jsonata", "folds/settle.jsonata", "folds/shadow.jsonata"} {
		data, err := os.ReadFile("corpus/v1/projection-state/app/" + name)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = &fstest.MapFile{Data: data}
	}
	// A constant output isolates input validation from the program's use of rows.
	files["folds/settle.jsonata"].Data = []byte(`{"decision":"effective","facts":[],"tables":{}}`)
	return files
}

func TestReadInputShapesAndPrivateEventAdmission(t *testing.T) {
	files := readInputTestSources(t)
	load := func() *Application {
		t.Helper()
		app, err := LoadApplication(files, ".", "reads", Tailapp(), "test/1")
		if err != nil {
			t.Fatal(err)
		}
		return app
	}
	app := load()
	valid := func() EvaluationInput {
		return EvaluationInput{
			Meta:  map[string]any{"position": 1, "event_id": "r#0", "event_type": "otel_event"},
			Event: map[string]any{"key": "k", "amount": 3, "retire": nil},
			Rows:  map[string]any{"prior": nil, "marks": []any{}, "positive": nil},
		}
	}
	if _, err := app.Evaluate("settle", valid()); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*EvaluationInput){
		"null rows":               func(i *EvaluationInput) { i.Rows = nil },
		"missing read":            func(i *EvaluationInput) { delete(i.Rows, "prior") },
		"unknown read":            func(i *EvaluationInput) { i.Rows["extra"] = nil },
		"optional one array":      func(i *EvaluationInput) { i.Rows["prior"] = []any{} },
		"missing column":          func(i *EvaluationInput) { i.Rows["prior"] = map[string]any{"key": "k"} },
		"extra column":            func(i *EvaluationInput) { i.Rows["prior"] = map[string]any{"key": "k", "balance": 1, "extra": 2} },
		"wrong column type":       func(i *EvaluationInput) { i.Rows["prior"] = map[string]any{"key": "k", "balance": "1"} },
		"null nonnullable column": func(i *EvaluationInput) { i.Rows["prior"] = map[string]any{"key": "k", "balance": nil} },
		"null many":               func(i *EvaluationInput) { i.Rows["marks"] = nil },
		"many object":             func(i *EvaluationInput) { i.Rows["marks"] = map[string]any{} },
		"many null row":           func(i *EvaluationInput) { i.Rows["marks"] = []any{nil} },
		"many over limit": func(i *EvaluationInput) {
			rows := make([]any, 11)
			for n := range rows {
				rows[n] = map[string]any{"key": "k", "mark": n}
			}
			i.Rows["marks"] = rows
		},
		"view missing column":      func(i *EvaluationInput) { i.Rows["positive"] = map[string]any{"key": "k"} },
		"view extra column":        func(i *EvaluationInput) { i.Rows["positive"] = map[string]any{"key": "k", "balance": 1, "extra": 2} },
		"private missing nullable": func(i *EvaluationInput) { delete(i.Event, "retire") },
		"private extra":            func(i *EvaluationInput) { i.Event["extra"] = 1 },
		"private wrong type":       func(i *EvaluationInput) { i.Event["amount"] = "3" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			input := valid()
			mutate(&input)
			if _, err := app.Evaluate("settle", input); err == nil || !strings.Contains(err.Error(), "input ") {
				t.Fatalf("invalid input admitted: %v", err)
			}
		})
	}
	input := valid()
	input.Rows["prior"] = map[string]any{"key": "k", "balance": 1}
	input.Rows["positive"] = map[string]any{"key": nil, "balance": map[string]any{"opaque": []any{1, "x"}}}
	rows := make([]any, 10)
	for n := range rows {
		rows[n] = map[string]any{"key": "k", "mark": n}
	}
	input.Rows["marks"] = rows
	if _, err := app.Evaluate("settle", input); err != nil {
		t.Fatalf("exact MANY limit and untyped view JSON: %v", err)
	}
	files["application.sql"].Data = []byte(strings.Replace(string(files["application.sql"].Data), "READ prior OPTIONAL ONE", "READ prior ONE", 1))
	app = load()
	if _, err := app.Evaluate("settle", valid()); err == nil {
		t.Fatal("required ONE accepted null")
	}
	if _, err := app.Evaluate("settle", input); err != nil {
		t.Fatalf("required ONE object: %v", err)
	}
}

func inputTestApplication(t *testing.T, dialect Dialect) *Application {
	t.Helper()
	sources := inputTestSources()
	app, err := LoadApplication(sources, ".", "input-test", dialect, "input-test/1")
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func declaredInputDialect() Dialect {
	d := renamedDialect()
	d.Input.Meta = NewObjectContract(false,
		InputField{Name: "position", Kind: InputScalar, Type: "INTEGER"},
		InputField{Name: "optional", Kind: InputScalar, Type: "TEXT", Optional: true},
		InputField{Name: "nullable", Kind: InputScalar, Type: "TEXT", Nullable: true},
		InputField{Name: "real", Kind: InputScalar, Type: "REAL"},
		InputField{Name: "flag", Kind: InputScalar, Type: "BOOLEAN"},
		InputField{Name: "blob", Kind: InputScalar, Type: "BLOB"})
	d.Input.Event = NewObjectContract(false,
		InputField{Name: "body", Kind: InputJSONObject},
		InputField{Name: "tags", Kind: InputStringArray},
		InputField{Name: "payload", Kind: InputScalarObject, Members: []EnvelopeField{
			{Name: "id", Type: "TEXT"}, {Name: "quantity", Type: "INTEGER", Nullable: true},
			{Name: "optional", Type: "TEXT", Optional: true},
		}})
	return d
}

func declaredTestInput() EvaluationInput {
	return EvaluationInput{
		Meta: map[string]any{"position": 1, "nullable": nil, "real": 1.25, "flag": true, "blob": "AA=="},
		Event: map[string]any{"id": "r-1", "topic": "alpha", "body": map[string]any{"key": "alpha", "amount": 5},
			"tags": []string{}, "payload": map[string]any{"id": "p-1", "quantity": nil}},
		Rows: map[string]any{"mine": nil},
	}
}

func TestDeclaredInputAdmissionPrecedesEvaluation(t *testing.T) {
	app := inputTestApplication(t, declaredInputDialect())
	input := declaredTestInput()
	if err := app.ValidateProgramInput("shape", input.Meta, input.Event); err != nil {
		t.Fatal(err)
	}
	result, err := app.Evaluate("shape", input)
	if err != nil || len(result.Events["inner_event"]) != 1 {
		t.Fatalf("valid input: %#v, %v", result, err)
	}
	cases := map[string]struct {
		mutate func(*EvaluationInput)
		want   string
	}{
		"missing metadata":          {func(i *EvaluationInput) { delete(i.Meta, "position") }, `field "position" is required`},
		"unknown metadata":          {func(i *EvaluationInput) { i.Meta["undeclared"] = 1 }, `unknown field "undeclared"`},
		"null metadata":             {func(i *EvaluationInput) { i.Meta = nil }, "input meta"},
		"optional is not nullable":  {func(i *EvaluationInput) { i.Meta["optional"] = nil }, `field "optional"`},
		"nullable is not optional":  {func(i *EvaluationInput) { delete(i.Meta, "nullable") }, `field "nullable" is required`},
		"integer string":            {func(i *EvaluationInput) { i.Meta["position"] = "1" }, `field "position"`},
		"integer fraction":          {func(i *EvaluationInput) { i.Meta["position"] = 1.5 }, `field "position"`},
		"inexact integer":           {func(i *EvaluationInput) { i.Meta["position"] = json.Number("9007199254740992") }, `field "position"`},
		"nonfinite real":            {func(i *EvaluationInput) { i.Meta["real"] = json.Number("1e999") }, `field "real"`},
		"numeric boolean":           {func(i *EvaluationInput) { i.Meta["flag"] = 1 }, `field "flag"`},
		"invalid blob":              {func(i *EvaluationInput) { i.Meta["blob"] = "%%%" }, `field "blob"`},
		"null event":                {func(i *EvaluationInput) { i.Event = nil }, "input event"},
		"missing envelope":          {func(i *EvaluationInput) { delete(i.Event, "id") }, `field "id" is required`},
		"missing nullable envelope": {func(i *EvaluationInput) { delete(i.Event, "topic") }, `field "topic" is required`},
		"unknown event":             {func(i *EvaluationInput) { i.Event["extra"] = true }, `unknown field "extra"`},
		"missing object":            {func(i *EvaluationInput) { delete(i.Event, "body") }, `field "body" is required`},
		"array instead of object":   {func(i *EvaluationInput) { i.Event["body"] = []any{} }, `field "body"`},
		"null array":                {func(i *EvaluationInput) { i.Event["tags"] = nil }, `field "tags"`},
		"nonstring array member":    {func(i *EvaluationInput) { i.Event["tags"] = []any{"x", nil} }, `field "tags"`},
		"missing nested scalar":     {func(i *EvaluationInput) { delete(i.Event["payload"].(map[string]any), "quantity") }, `field "quantity" is required`},
		"unknown nested scalar":     {func(i *EvaluationInput) { i.Event["payload"].(map[string]any)["extra"] = 1 }, `unknown field "extra"`},
		"nested scalar type":        {func(i *EvaluationInput) { i.Event["payload"].(map[string]any)["quantity"] = "2" }, `field "quantity"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			input := declaredTestInput()
			tc.mutate(&input)
			// The program does not use most of these fields. Output validation cannot
			// substitute for admission at either public entry point.
			for stage, err := range map[string]error{"before reads": app.ValidateProgramInput("shape", input.Meta, input.Event), "evaluate": func() error { _, err := app.Evaluate("shape", input); return err }()} {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("%s: want %q, got %v", stage, tc.want, err)
				}
			}
		})
	}
	if err := app.ValidateProgramInput("missing", input.Meta, input.Event); err == nil {
		t.Fatal("unknown program admitted")
	}
}

func TestOpaqueInputJSONAndDepthBounds(t *testing.T) {
	app := inputTestApplication(t, declaredInputDialect())
	for _, depth := range []int{70, 1021, 1022} {
		t.Run(strconv.Itoa(depth), func(t *testing.T) {
			input := declaredTestInput()
			input.Event["body"].(map[string]any)["opaque"] = json.RawMessage(strings.Repeat("[", depth) + `1` + strings.Repeat("]", depth))
			err := app.ValidateProgramInput("shape", input.Meta, input.Event)
			_, evaluationErr := app.Evaluate("shape", input)
			// Root, event, body add three containers.
			if depth <= 1021 {
				if err != nil || evaluationErr != nil {
					t.Fatalf("bounded opaque JSON: %v / %v", err, evaluationErr)
				}
			} else if err == nil || evaluationErr == nil || !strings.Contains(err.Error(), "depth 1024") {
				t.Fatalf("over-depth input: %v / %v", err, evaluationErr)
			}
		})
	}
	input := declaredTestInput()
	input.Event["body"].(map[string]any)["opaque_number"] = json.Number("1e999")
	if err := app.ValidateProgramInput("shape", input.Meta, input.Event); err != nil {
		t.Fatalf("opaque JSON was subjected to REAL admission: %v", err)
	}
	// JSONata retains its existing numeric interpretation and may reject this
	// admitted JSON value. Input admission does not change the evaluator.
	delete(input.Event["body"].(map[string]any), "opaque_number")
	input.Event["body"].(map[string]any)["escaped"] = strings.Repeat(`\"[{`, 1100)
	if _, err := app.Evaluate("shape", input); err != nil {
		t.Fatalf("string brackets counted as containers: %v", err)
	}
	input.Event["body"].(map[string]any)["opaque"] = json.RawMessage(`{"broken":`)
	if _, err := app.Evaluate("shape", input); err == nil {
		t.Fatal("invalid raw JSON admitted")
	}

	input = declaredTestInput()
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	d := declaredInputDialect()
	d.Limits.MaxInputBytes = len(encoded)
	if _, err := inputTestApplication(t, d).Evaluate("shape", input); err != nil {
		t.Fatalf("exact byte limit: %v", err)
	}
	d.Limits.MaxInputBytes--
	if _, err := inputTestApplication(t, d).Evaluate("shape", input); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("over byte limit: %v", err)
	}
}

func TestInputContractRefusesIncompleteOrAmbiguousDialects(t *testing.T) {
	cases := map[string]func(*Dialect){
		"missing meta":   func(d *Dialect) { d.Input.Meta = ObjectContract{} },
		"missing event":  func(d *Dialect) { d.Input.Event = ObjectContract{} },
		"zero depth":     func(d *Dialect) { d.Limits.MaxInputDepth = 0 },
		"negative depth": func(d *Dialect) { d.Limits.MaxInputDepth = -1 },
		"metadata object": func(d *Dialect) {
			d.Input.Meta = NewObjectContract(false, InputField{Name: "x", Kind: InputJSONObject})
		},
		"event scalar duplication": func(d *Dialect) {
			d.Input.Event = NewObjectContract(false, InputField{Name: "x", Kind: InputScalar, Type: "TEXT"})
		},
		"event collision": func(d *Dialect) {
			d.Input.Event = NewObjectContract(false, InputField{Name: "id", Kind: InputJSONObject})
		},
		"duplicate structured": func(d *Dialect) {
			f := d.Input.Event.Fields()
			d.Input.Event = NewObjectContract(false, append(f, f[0])...)
		},
		"unknown kind": func(d *Dialect) {
			d.Input.Event = NewObjectContract(false, InputField{Name: "x", Kind: "recursive-object"})
		},
		"unused type": func(d *Dialect) {
			d.Input.Event = NewObjectContract(false, InputField{Name: "x", Kind: InputJSONObject, Type: "TEXT"})
		},
		"unused members": func(d *Dialect) {
			d.Input.Event = NewObjectContract(false, InputField{Name: "x", Kind: InputJSONObject, Members: []EnvelopeField{{Name: "x", Type: "TEXT"}}})
		},
	}
	for _, typ := range []string{"JSON", "text", "TEXT\nINJECT", "TEXT;other"} {
		cases["scalar type "+typ] = func(d *Dialect) {
			d.HostEvent = NewEventContract("host_record", EnvelopeField{Name: "topic", Type: typ})
		}
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := declaredInputDialect()
			mutate(&d)
			if _, err := LoadApplication(inputTestSources(), ".", "input-test", d, "test/1"); err == nil {
				t.Fatal("invalid contract compiled")
			}
		})
	}
	// Explicit empty and nullable objects are declarations, unlike zero values.
	d := renamedDialect()
	d.HostEvent = NewEventContract("host_record")
	d.Input = InputContract{Meta: NewObjectContract(true), Event: NewObjectContract(true)}
	sources := inputTestSources()
	sources["app.ddl"].Data = []byte(strings.Replace(string(sources["app.ddl"].Data), "READ mine OPTIONAL ONE AS\n  SELECT topic FROM seen_topics WHERE topic = :event.topic\n", "", 1))
	sources["programs/shape.jn"] = &fstest.MapFile{Data: []byte(`{"decision":"effective","facts":[],"events":{},"tables":{}}`)}
	app, err := LoadApplication(sources, ".", "empty", d, "test/1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Evaluate("shape", EvaluationInput{Rows: map[string]any{}}); err != nil {
		t.Fatalf("explicit nullable roots: %v", err)
	}
}

func TestInputStringArrayPreservesOrderAndDuplicates(t *testing.T) {
	sources := inputTestSources()
	sources["programs/shape.jn"].Data = []byte(strings.Replace(string(sources["programs/shape.jn"].Data), `"facts": []`, `"facts": [{"tags": event.tags}]`, 1))
	app, err := LoadApplication(sources, ".", "array-input", declaredInputDialect(), "test/1")
	if err != nil {
		t.Fatal(err)
	}
	input := declaredTestInput()
	input.Event["tags"] = []string{"b", "a", "b"}
	result, err := app.Evaluate("shape", input)
	if err != nil || len(result.Facts) != 1 {
		t.Fatalf("array input: %#v, %v", result, err)
	}
	encoded, err := json.Marshal(result.Facts[0]["tags"])
	if err != nil || string(encoded) != `["b","a","b"]` {
		t.Fatalf("semantic array reordered or deduplicated: %s, %v", encoded, err)
	}
}

func TestReadColumnCasePreservesTypeAdmission(t *testing.T) {
	for _, selected := range []string{"balance", "BALANCE", "BaLaNcE"} {
		t.Run(selected, func(t *testing.T) {
			files := readInputTestSources(t)
			files["application.sql"].Data = []byte(strings.Replace(string(files["application.sql"].Data), "SELECT key, balance\n  FROM ledger\n", "SELECT key, "+selected+"\n  FROM ledger\n", 1))
			app, err := LoadApplication(files, ".", "reads", Tailapp(), "test/1")
			if err != nil {
				t.Fatal(err)
			}
			for _, test := range []struct {
				name  string
				value any
				valid bool
			}{
				{"integer", 1, true}, {"string", "1", false}, {"null", nil, false}, {"fraction", 1.5, false}, {"object", map[string]any{}, false},
			} {
				t.Run(test.name, func(t *testing.T) {
					input := EvaluationInput{
						Meta:  map[string]any{"position": 1, "event_id": "r#0", "event_type": "otel_event"},
						Event: map[string]any{"key": "k", "amount": 3, "retire": nil},
						Rows:  map[string]any{"prior": map[string]any{"key": "k", selected: test.value}, "marks": []any{}, "positive": nil},
					}
					_, err := app.Evaluate("settle", input)
					if test.valid && err != nil {
						t.Fatalf("valid INTEGER refused: %v", err)
					}
					if !test.valid && (err == nil || !strings.Contains(err.Error(), "input rows read")) {
						t.Fatalf("invalid INTEGER input: %v", err)
					}
				})
			}
		})
	}
}
