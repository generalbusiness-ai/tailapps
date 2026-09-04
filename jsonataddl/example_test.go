package jsonataddl_test

import (
	"fmt"
	"testing/fstest"

	"github.com/generalbusiness-ai/tailapps/jsonataddl"
)

func ExampleLoadApplication() {
	files := fstest.MapFS{
		"application.sql": {Data: []byte(`CREATE EVENT otel_event (
  key TEXT NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE totals (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE NORMALIZER normalize ON otlp_record
USING 'folds/normalize.jsonata'
EMITS otel_event;

CREATE FOLD accumulate ON otel_event
USING 'folds/accumulate.jsonata'
WRITES totals;

CREATE EXPORT totals AS
  SELECT key, total FROM totals;
`)},
		"folds/normalize.jsonata": {Data: []byte(`{
  "decision": "effective",
  "facts": [],
  "events": {},
  "tables": {}
}`)},
		"folds/accumulate.jsonata": {Data: []byte(`{
  "decision": "effective",
  "facts": [],
  "tables": {}
}`)},
	}

	application, err := jsonataddl.LoadApplication(
		files,
		".",
		"example",
		jsonataddl.Tailapp(),
		"example-host-runtime-v1",
	)
	if err != nil {
		panic(err)
	}
	result, err := application.Evaluate("normalize", jsonataddl.EvaluationInput{
		Meta:  map[string]any{"position": 1},
		Event: map[string]any{"id": "event-1"},
		Rows:  map[string]any{},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Decision)

	// Output: effective
}
