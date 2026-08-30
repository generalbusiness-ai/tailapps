package jsonataddl

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	jsonata "github.com/jsonata-go/jsonata/v206"
)

// The JSONata confinement rules admit only the deterministic bounded subset:
// no ambient clock, randomness, or dynamic evaluation; no user-defined
// functions; no wildcards or generated ranges; and a closed set of pure
// builtin functions. The rules are dialect-independent - they define what
// the core evaluator itself is, not host policy.

var ambientJSONataRE = regexp.MustCompile(`(?i)\$(?:now|millis|random|shuffle|eval)\b`)

var allowedJSONataFunctions = nameSet(
	"abs", "boolean", "ceil", "contains", "count", "exists", "floor",
	"length", "lookup", "lowercase", "max", "min", "not", "number",
	"round", "string", "substring", "sum", "uppercase",
)

func validateJSONataLexicalSource(source []byte) error {
	if hasUnquotedAsterisk(source) {
		return errors.New("JSONata wildcard and multiplication syntax is outside the deterministic bounded profile")
	}
	if hasUnquotedRange(source) {
		return errors.New("JSONata generated ranges are outside the deterministic bounded profile")
	}
	return nil
}

func validateJSONataAST(value any) error {
	root, ok := value.(*jsonata.ASTNode)
	if !ok || root == nil {
		return errors.New("compiled JSONata expression has no inspectable AST")
	}
	seen := make(map[*jsonata.ASTNode]bool)
	var walk func(*jsonata.ASTNode) error
	walk = func(node *jsonata.ASTNode) error {
		if node == nil || seen[node] {
			return nil
		}
		seen[node] = true
		switch node.Type {
		case "lambda":
			return errors.New("user-defined JSONata functions are outside the bounded profile")
		case "function", "partial":
			if err := validateJSONataCallable(node.Procedure); err != nil {
				return err
			}
		case "apply":
			if err := validateJSONataCallable(node.RHS); err != nil {
				return err
			}
		}
		children := []*jsonata.ASTNode{
			node.LHS, node.RHS, node.Expression, node.Procedure, node.Pattern,
			node.Update, node.Delete, node.Condition, node.Then, node.Else, node.Body,
		}
		children = append(children, node.Expressions...)
		children = append(children, node.Arguments...)
		children = append(children, node.Steps...)
		for _, pair := range node.RHSPairs {
			children = append(children, pair[0], pair[1])
		}
		for _, pair := range node.LHSPairs {
			children = append(children, pair[0], pair[1])
		}
		for _, term := range node.RHSTerms {
			if term != nil {
				children = append(children, term.Expression)
			}
		}
		for _, term := range node.Terms {
			if term != nil {
				children = append(children, term.Expression)
			}
		}
		for _, stage := range node.Predicate {
			if stage != nil {
				children = append(children, stage.Expr)
			}
		}
		for _, stage := range node.Stages {
			if stage != nil {
				children = append(children, stage.Expr)
			}
		}
		if node.Group != nil {
			for _, pair := range node.Group.LHS {
				children = append(children, pair[0], pair[1])
			}
		}
		for _, child := range children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}

func validateJSONataCallable(node *jsonata.ASTNode) error {
	if node == nil {
		return errors.New("dynamic JSONata function application is outside the bounded profile")
	}
	if node.Type == "function" || node.Type == "partial" {
		return validateJSONataCallable(node.Procedure)
	}
	if node.Type != "variable" {
		return errors.New("dynamic JSONata function application is outside the bounded profile")
	}
	name, ok := node.Value.(string)
	if !ok || !allowedJSONataFunctions[strings.ToLower(name)] {
		return fmt.Errorf("JSONata function $%v is outside the bounded profile", node.Value)
	}
	return nil
}

func hasUnquotedRange(source []byte) bool {
	var quote byte
	escaped := false
	for index, current := range source {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' && quote != '`' {
				escaped = true
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		switch current {
		case '\'', '"', '`':
			quote = current
		case '.':
			if index+1 < len(source) && source[index+1] == '.' {
				return true
			}
		}
	}
	return false
}

// hasUnquotedAsterisk rejects the evaluator's observable object wildcards.
// Multiplication is rejected with them until the core owns deterministic step
// and allocation counters; reducing the language is safer than pretending a
// machine-dependent timeout is a semantic bound.
func hasUnquotedAsterisk(source []byte) bool {
	var quote byte
	escaped := false
	for _, current := range source {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' && quote != '`' {
				escaped = true
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		switch current {
		case '\'', '"', '`':
			quote = current
		case '*':
			return true
		}
	}
	return false
}

func nameSet(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
