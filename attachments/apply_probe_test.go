package profile

import (
	"testing"

	jsonata "github.com/jsonata-go/jsonata/v206"
)

// Probe: the bounded-JSONata subset gate (validateJSONataSource) does not reject
// the apply operator ~> nor the Unicode lambda synonym λ, and the pinned
// evaluator actually compiles and evaluates both. Demonstrates H-NEW-1 / H-NEW-2
// at the current main head.
func TestApplyOperatorAndLambdaSynonymBypassSubsetGate(t *testing.T) {
	// H-NEW-1: apply a non-allowlisted unary builtin without "$name(".
	// $keys / $spread defeat the object-wildcard exclusion (telemetry exfil);
	// $sort with no ( likewise escapes the allowlist.
	applyPrograms := []string{
		`event ~> $keys`,
		`event ~> $spread`,
		`event ~> $reverse`,
		`["b","a"] ~> $sort`,
		`"x" ~> $base64encode`,
	}
	for _, program := range applyPrograms {
		if err := validateJSONataSource([]byte(program)); err != nil {
			t.Errorf("gate REJECTED %q (would be safe): %v", program, err)
			continue
		}
		expr, err := jsonata.Compile(program, false)
		if err != nil {
			t.Errorf("gate admitted %q but evaluator refused to compile: %v", program, err)
			continue
		}
		if _, err := expr.Evaluate([]byte(`{"event":{"a":1,"b":2}}`), nil); err != nil {
			t.Errorf("gate admitted and compiled %q but eval failed: %v", program, err)
			continue
		}
		t.Logf("ADMITTED + EXECUTED (bypass): %q", program)
	}

	// H-NEW-2: λ is a synonym for function; the lambda regex is ASCII-only.
	// Arbitrary user lambdas + recursion via ~> become expressible.
	lambda := `("x" ~> λ($s){$s & $s} ~> λ($s){$s & $s})`
	if err := validateJSONataSource([]byte(lambda)); err != nil {
		t.Errorf("gate REJECTED lambda-synonym program (would be safe): %v", err)
	} else {
		expr, err := jsonata.Compile(lambda, false)
		if err != nil {
			t.Errorf("gate admitted lambda-synonym but evaluator refused: %v", err)
		} else if out, err := expr.Evaluate([]byte(`{}`), nil); err != nil {
			t.Errorf("gate admitted+compiled lambda-synonym but eval failed: %v", err)
		} else {
			t.Logf("ADMITTED + EXECUTED (bypass): %q -> %v", lambda, out)
		}
	}
}
