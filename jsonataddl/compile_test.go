package jsonataddl

import (
	"strings"
	"testing"
)

// The statement splitter and source-path validation were previously tested
// through internal/profile's private helpers; they live here now.

func TestStatementSplitterHandlesQuotesAndComments(t *testing.T) {
	source := `-- lead ;
CREATE TABLE example (id TEXT PRIMARY KEY, note TEXT CHECK(note <> ';'));
/* between ; */ CREATE VIEW example_view AS SELECT id, note FROM example;
`
	statements, err := splitDDLStatements(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 2 || !strings.Contains(statements[0], "';'") {
		t.Fatalf("statements = %#v", statements)
	}
	for _, invalid := range []string{"CREATE TABLE x (id TEXT PRIMARY KEY)", "CREATE TABLE x (id TEXT CHECK(id <> 'x));"} {
		if _, err := splitDDLStatements(invalid); err == nil {
			t.Fatalf("invalid SQL %q accepted", invalid)
		}
	}
}

func TestSourcePathValidationRejectsEscapes(t *testing.T) {
	for _, name := range []string{"../fold.jsonata", "/fold.jsonata", "folds//x.jsonata", `folds\x.jsonata`, "", "folds/./x.jsonata"} {
		if err := validateSourcePath(name); err == nil {
			t.Fatalf("invalid path %q accepted", name)
		}
	}
}

func TestJSONataAsteriskInsideStringLiteralIsAllowed(t *testing.T) {
	if err := validateJSONataLexicalSource([]byte(`{"pattern":"*"}`)); err != nil {
		t.Fatal(err)
	}
	if err := validateJSONataLexicalSource([]byte(`a * b`)); err == nil {
		t.Fatal("unquoted asterisk accepted")
	}
}
