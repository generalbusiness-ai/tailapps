package jsonataddl_test

import (
	"fmt"
	"testing/fstest"

	"github.com/generalbusiness-ai/tailapps/jsonataddl"
)

func exampleSources() fstest.MapFS {
	return fstest.MapFS{
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

}

func ExampleLoadApplication() {
	application, err := jsonataddl.LoadApplication(
		exampleSources(),
		".",
		"example",
		jsonataddl.Tailapp(),
		"example-host-runtime-v1",
	)
	if err != nil {
		panic(err)
	}
	result, err := application.Evaluate("normalize", jsonataddl.EvaluationInput{
		Meta: map[string]any{"position": 1, "event_id": "event-1", "event_type": "otlp_record"},
		Event: map[string]any{
			"id": "event-1", "signal": "log", "name": "example", "source": "demo",
			"time_unix_nano": nil, "observed_unix_nano": nil, "trace_id": nil, "span_id": nil,
			"content_digest": "example-content-digest", "record": map[string]any{},
		},
		Rows: map[string]any{},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Decision)

	// Output: effective
}

func ExampleApplication_Evaluate_abbreviated() {
	application, err := jsonataddl.LoadApplication(exampleSources(), ".", "example", jsonataddl.Tailapp(), "example-host-runtime-v1")
	if err != nil {
		panic(err)
	}
	// Missing required fields are refused even when the program ignores them.
	_, err = application.Evaluate("normalize", jsonataddl.EvaluationInput{
		Meta: map[string]any{"position": 1}, Event: map[string]any{}, Rows: map[string]any{},
	})
	fmt.Println(err)
	// Output: input meta: field "event_id" is required
}
