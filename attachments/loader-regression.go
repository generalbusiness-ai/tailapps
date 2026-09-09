package jsonataddl_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/generalbusiness-ai/tailapps/jsonataddl"
)

// Exercise the exported loader and evaluator, including the result contract:
// accepting a spelling is insufficient if it can resolve to another function.
func TestLoadApplicationConfinesCallableBindings(t *testing.T) {
	cases := []struct {
		name, expression string
		refuse           bool
	}{
		{"direct forbidden", `$string($keys(event))`, true},
		{"rebound call", `($lowercase := $keys; $string($lowercase(event)))`, true},
		{"rebound apply", `($lowercase := $keys; $string(event ~> $lowercase))`, true},
		{"nested binding", `($x := ($lowercase := $keys; $string($lowercase(event))); $x)`, true},
		{"binding chain", `($x := $keys; $lowercase := $x; $string($lowercase(event)))`, true},
		{"rebound partial", `($lowercase := $keys; $string(event ~> $lowercase(?)))`, true},
		{"partial result call", `($lowercase := $keys; $string($lowercase(?)(event)))`, true},
		{"returned callable", `$string($lookup({"f":$keys},"f")(event))`, true},
		{"returned callable apply", `$string(event ~> $lookup({"f":$keys},"f")())`, true},
		{"outside reference", `$string($exists($reverse))`, true},
		{"outside container", `$string($exists([$reverse]))`, true},
		{"allowed alias", `($sum := $length; $string($sum("abc")))`, false},
		{"allowed partial alias", `($sum := $substring(?,1); $sum("abc"))`, false},
		{"nested allowed alias", `($sum := $length; ($sum := $abs; $sum(-2)); $string($sum("abc")))`, false},
		{"lexical data", `($reverse := 3; $string($exists($reverse)))`, false},
		{"ordinary object key", `$string({"reverse":3}.reverse)`, false},
		{"allowed container", `$string($exists([$length]))`, false},
		{"returned allowed callable", `$string($lookup({"f":$length},"f")("abc"))`, false},
		{"builtin name index", `[event.key]#$count.$string($)`, false},
		{"builtin name stage index", `[event.key][true]#$count.$string($)`, false},
		{"direct allowed", `$string(event.key)`, false},
		{"scalar binding", `($x := event.key; $string($x))`, false},
		{"nested scalar binding", `($x := event.key; ($y := $x; $string($y)))`, false},
		{"allowed apply", `event.key ~> $string`, false},
		{"allowed call apply", `event.key ~> $string()`, false},
		{"allowed partial call", `$string(?)(event.key)`, false},
		{"allowed partial apply", `event.key ~> $string(?)`, false},
		{"ordinary index", `[event.key]#$i.$string($)`, false},
		{"ordinary focus", `event@$item.$string($item.key)`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := exampleSources()
			files["folds/accumulate.jsonata"] = &fstest.MapFile{Data: []byte(
				`{"decision":"effective","facts":[],"tables":{"totals":{"upsert":[{"key":` + tc.expression + `,"total":1}]}}}`)}
			app, err := jsonataddl.LoadApplication(files, ".", "callable-test", jsonataddl.Tailapp(), "callable-test-runtime")
			if tc.refuse {
				if err == nil || !strings.Contains(err.Error(), "bounded profile") {
					t.Fatalf("callable evasion reached compilation: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("allowed expression refused: %v", err)
			}
			result, err := app.Evaluate("accumulate", jsonataddl.EvaluationInput{
				Meta:  map[string]any{"position": 1, "event_id": "test-1", "event_type": "otel_event"},
				Event: map[string]any{"key": "s1", "source_position": 1}, Rows: map[string]any{},
			})
			if err != nil {
				t.Fatalf("allowed evaluation failed: %v", err)
			}
			rows := result.Tables["totals"].Upsert
			want := map[string]string{"allowed alias": "3", "allowed partial alias": "bc", "nested allowed alias": "3", "lexical data": "true", "ordinary object key": "3", "allowed container": "true", "returned allowed callable": "3"}[tc.name]
			if want == "" {
				want = "s1"
			}
			if len(rows) != 1 || rows[0]["key"] != want {
				t.Fatalf("allowed result changed: %#v", rows)
			}
		})
	}
}
