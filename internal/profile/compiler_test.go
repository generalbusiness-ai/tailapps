package profile

import (
	"strings"
	"testing"
	"testing/fstest"
)

const validDDL = `
CREATE EVENT otel_event (
  kind TEXT NOT NULL,
  session_id TEXT NOT NULL,
  tool TEXT NOT NULL,
  success BOOLEAN NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE coverage (
  harness TEXT NOT NULL,
  capability TEXT NOT NULL,
  state TEXT NOT NULL,
  last_position INTEGER NOT NULL,
  PRIMARY KEY (harness, capability)
);

CREATE TABLE sessions (
  session_id TEXT PRIMARY KEY,
  calls INTEGER NOT NULL,
  failures INTEGER NOT NULL
);

CREATE INDEX sessions_failures ON sessions(failures);
CREATE VIEW failing_sessions AS SELECT session_id, failures FROM sessions WHERE failures > 0;

CREATE NORMALIZER normalize ON otlp_record
USING 'folds/normalize.jsonata'
WRITES coverage
EMITS otel_event;

CREATE FOLD observe ON otel_event
READ current OPTIONAL ONE AS
  SELECT session_id, calls, failures FROM sessions WHERE session_id = :event.session_id
USING 'folds/observe.jsonata'
WRITES sessions;

CREATE EXPORT sessions AS SELECT session_id, calls, failures FROM sessions;
`

const normalizeJSONata = `(
  $known := event.name = "tool.result";
  {
    "decision": "effective",
    "facts": [],
    "events": {
      "otel_event": $known ? [{
        "kind": "tool-result",
        "session_id": "s1",
        "tool": "shell",
        "success": false,
        "source_position": meta.position
      }] : []
    },
    "tables": {
      "coverage": {"upsert": [{
        "harness": event.source,
        "capability": "tool-result",
        "state": $known ? "observed" : "unknown",
        "last_position": meta.position
      }]}
    }
  }
)`

const observeJSONata = `(
  $prior := rows.current;
  {
    "decision": "effective",
    "facts": [],
    "tables": {
      "sessions": {"upsert": [{
        "session_id": event.session_id,
        "calls": $prior ? $prior.calls + 1 : 1,
        "failures": $prior ? $prior.failures + (event.success ? 0 : 1) : (event.success ? 0 : 1)
      }]}
    }
  }
)`

func validFS(ddl string) fstest.MapFS {
	return fstest.MapFS{
		"application.sql":         {Data: []byte(ddl)},
		"folds/normalize.jsonata": {Data: []byte(normalizeJSONata)},
		"folds/observe.jsonata":   {Data: []byte(observeJSONata)},
	}
}

func TestLoadCompilesFixedTopology(t *testing.T) {
	profile, err := Load(validFS(validDDL), ".", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	if profile.RuntimeProfile != RuntimeID || !strings.HasPrefix(profile.Revision, "sha256:") {
		t.Fatalf("identity = %#v", profile)
	}
	if profile.Event.Name != "otel_event" || profile.Normalizer.Event != "otlp_record" || len(profile.Folds) != 1 {
		t.Fatalf("topology = %#v", profile)
	}
	if profile.Tables["coverage"].Writer != "normalize" || profile.Tables["sessions"].Writer != "observe" {
		t.Fatalf("writers = %#v", profile.Tables)
	}
	if len(profile.Exports["sessions"].Columns) != 3 || profile.ExportContractDigest == "" || profile.StorageSchemaDigest == "" {
		t.Fatalf("contracts = %#v", profile.Exports)
	}

	again, err := Load(validFS(validDDL), ".", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	if again.Revision != profile.Revision || again.StorageSchemaDigest != profile.StorageSchemaDigest || again.ExportContractDigest != profile.ExportContractDigest {
		t.Fatal("compilation identity is not stable")
	}
}

func TestRevisionAndStorageCompatibilityAreSeparate(t *testing.T) {
	prior, err := Load(validFS(validDDL), ".", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	changedPrograms := validFS(validDDL)
	changedPrograms["folds/observe.jsonata"] = &fstest.MapFile{Data: []byte(strings.Replace(observeJSONata, `"calls": $prior ? $prior.calls + 1 : 1`, `"calls": $prior ? $prior.calls + 2 : 2`, 1))}
	next, err := Load(changedPrograms, ".", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	if next.Revision == prior.Revision {
		t.Fatal("program change did not change revision")
	}
	if next.StorageSchemaDigest != prior.StorageSchemaDigest {
		t.Fatal("program change changed storage schema")
	}
	if err := ContinueCompatible(prior, next); err != nil {
		t.Fatalf("program-only continuation: %v", err)
	}
	formattedDDL := strings.Replace(validDDL, "calls INTEGER NOT NULL", "calls    INTEGER    NOT NULL", 1)
	formatted, err := Load(validFS(formattedDDL), ".", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	if err := ContinueCompatible(prior, formatted); err != nil {
		t.Fatalf("format-only table change: %v", err)
	}

	additiveDDL := strings.Replace(validDDL, "CREATE INDEX", "CREATE TABLE additions (id TEXT PRIMARY KEY);\nCREATE FOLD fill_additions ON otel_event USING 'folds/additions.jsonata' WRITES additions;\nCREATE INDEX", 1)
	additiveFiles := validFS(additiveDDL)
	additiveFiles["folds/additions.jsonata"] = &fstest.MapFile{Data: []byte(`{"decision":"effective","facts":[],"tables":{"additions":{"upsert":[{"id":event.session_id}]}}}`)}
	additive, err := Load(additiveFiles, ".", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	if err := ContinueCompatible(prior, additive); err != nil {
		t.Fatalf("additive continuation: %v", err)
	}

	changedDDL := strings.Replace(validDDL, "calls INTEGER NOT NULL", "calls TEXT NOT NULL", 1)
	changed, err := Load(validFS(changedDDL), ".", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	if err := ContinueCompatible(prior, changed); err == nil || !strings.Contains(err.Error(), "changed stored shape") {
		t.Fatalf("changed table accepted: %v", err)
	}
	constraintDDL := strings.Replace(validDDL, "failures INTEGER NOT NULL", "failures INTEGER NOT NULL CHECK (failures >= 0)", 1)
	constraintChanged, err := Load(validFS(constraintDDL), ".", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	if err := ContinueCompatible(prior, constraintChanged); err == nil || !strings.Contains(err.Error(), "changed stored shape") {
		t.Fatalf("changed constraint accepted: %v", err)
	}
}

func TestCompilerRejectsAuthorityViolations(t *testing.T) {
	tests := map[string]string{
		"second normalizer":       strings.Replace(validDDL, "CREATE FOLD observe", "CREATE NORMALIZER again ON otlp_record USING 'folds/normalize.jsonata' EMITS otel_event;\nCREATE FOLD observe", 1),
		"wrong event":             strings.Replace(validDDL, "CREATE EVENT otel_event", "CREATE EVENT public_event", 1),
		"duplicate writer":        strings.Replace(validDDL, "WRITES sessions;", "WRITES sessions, coverage;", 1),
		"normalizer foreign read": strings.Replace(validDDL, "USING 'folds/normalize.jsonata'", "READ foreign OPTIONAL ONE AS SELECT session_id FROM sessions WHERE session_id = :event.id\nUSING 'folds/normalize.jsonata'", 1),
		"analytic foreign read":   strings.Replace(validDDL, "SELECT session_id, calls, failures FROM sessions", "SELECT harness, capability, state, last_position FROM coverage", 1),
		"ambient JSONata":         validDDL,
		"drop":                    strings.Replace(validDDL, "CREATE INDEX", "DROP TABLE sessions;\nCREATE INDEX", 1),
		"missing primary key":     strings.Replace(validDDL, "session_id TEXT PRIMARY KEY", "session_id TEXT NOT NULL", 1),
		"cross namespace export":  strings.Replace(validDDL, "FROM sessions;\n", "FROM other.sessions;\n", 1),
	}
	for name, ddl := range tests {
		t.Run(name, func(t *testing.T) {
			files := validFS(ddl)
			if name == "ambient JSONata" {
				files["folds/observe.jsonata"] = &fstest.MapFile{Data: []byte(`{"decision":"effective","facts":[],"tables":{},"time":$now()}`)}
			}
			if _, err := Load(files, ".", "agent-guard"); err == nil {
				t.Fatal("invalid application compiled")
			}
		})
	}
}

func TestManyReadRequiresUniqueKeySuffix(t *testing.T) {
	ddl := strings.Replace(validDDL,
		"READ current OPTIONAL ONE AS\n  SELECT session_id, calls, failures FROM sessions WHERE session_id = :event.session_id",
		"READ current MANY LIMIT 10 AS\n  SELECT session_id, calls, failures FROM sessions WHERE session_id = :event.session_id ORDER BY session_id", 1)
	if _, err := Load(validFS(ddl), ".", "agent-guard"); err != nil {
		t.Fatalf("unique-key order rejected: %v", err)
	}
	bad := strings.Replace(ddl, "ORDER BY session_id", "ORDER BY calls", 1)
	if _, err := Load(validFS(bad), ".", "agent-guard"); err == nil || !strings.Contains(err.Error(), "unique key") {
		t.Fatalf("non-total MANY accepted: %v", err)
	}
}

func TestReadAuthorityTraversesViews(t *testing.T) {
	ownedViewDDL := strings.Replace(validDDL,
		"CREATE VIEW failing_sessions AS SELECT session_id, failures FROM sessions WHERE failures > 0;",
		"CREATE VIEW failing_sessions AS SELECT session_id, calls, failures FROM sessions WHERE failures > 0;", 1)
	ownedViewDDL = strings.Replace(ownedViewDDL, "FROM sessions WHERE session_id = :event.session_id\nUSING", "FROM failing_sessions WHERE session_id = :event.session_id\nUSING", 1)
	if _, err := Load(validFS(ownedViewDDL), ".", "agent-guard"); err != nil {
		t.Fatalf("own-table view rejected: %v", err)
	}

	crossDDL := strings.Replace(validDDL, "CREATE INDEX sessions_failures", `
CREATE TABLE other_state (id TEXT PRIMARY KEY);
CREATE VIEW other_view AS SELECT id FROM other_state;
CREATE FOLD other_writer ON otel_event USING 'folds/other.jsonata' WRITES other_state;
CREATE INDEX sessions_failures`, 1)
	crossDDL = strings.Replace(crossDDL,
		"READ current OPTIONAL ONE AS\n  SELECT session_id, calls, failures FROM sessions WHERE session_id = :event.session_id",
		"READ current OPTIONAL ONE AS\n  SELECT id FROM other_view WHERE id = :event.session_id", 1)
	files := validFS(crossDDL)
	files["folds/other.jsonata"] = &fstest.MapFile{Data: []byte(`{"decision":"effective","facts":[],"tables":{"other_state":{"upsert":[{"id":event.session_id}]}}}`)}
	if _, err := Load(files, ".", "agent-guard"); err == nil || !strings.Contains(err.Error(), "owned by analytic fold") {
		t.Fatalf("analytic-to-analytic view read accepted: %v", err)
	}
}

func TestCommaJoinCannotEvadeReadAuthority(t *testing.T) {
	ddl := strings.Replace(validDDL, "CREATE INDEX sessions_failures", `
CREATE TABLE other_state (id TEXT PRIMARY KEY);
CREATE FOLD other_writer ON otel_event USING 'folds/other.jsonata' WRITES other_state;
CREATE VIEW leaked_state AS SELECT session_id, calls, failures FROM sessions, other_state;
CREATE INDEX sessions_failures`, 1)
	ddl = strings.Replace(ddl,
		"SELECT session_id, calls, failures FROM sessions WHERE session_id = :event.session_id",
		"SELECT session_id, calls, failures FROM leaked_state WHERE session_id = :event.session_id", 1)
	files := validFS(ddl)
	files["folds/other.jsonata"] = &fstest.MapFile{Data: []byte(`{"decision":"effective","facts":[],"tables":{"other_state":{"upsert":[{"id":event.session_id}]}}}`)}
	if _, err := Load(files, ".", "agent-guard"); err == nil || !strings.Contains(err.Error(), "comma-separated joins") {
		t.Fatalf("comma join accepted: %v", err)
	}
}

func TestExportThroughApplicationViewIsRefused(t *testing.T) {
	ddl := strings.Replace(validDDL,
		"CREATE EXPORT sessions AS SELECT session_id, calls, failures FROM sessions;",
		"CREATE EXPORT sessions AS SELECT session_id, failures FROM failing_sessions;", 1)
	if _, err := Load(validFS(ddl), ".", "agent-guard"); err == nil || !strings.Contains(err.Error(), "export base tables directly") {
		t.Fatalf("view-based export accepted: %v", err)
	}
}

func TestStatementSplitterHandlesQuotesAndComments(t *testing.T) {
	source := `-- lead ;
CREATE TABLE example (id TEXT PRIMARY KEY, note TEXT CHECK(note <> ';'));
/* between ; */ CREATE VIEW example_view AS SELECT id, note FROM example;
`
	statements, err := splitStatements(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 2 || !strings.Contains(statements[0], "';'") {
		t.Fatalf("statements = %#v", statements)
	}
	for _, invalid := range []string{"CREATE TABLE x (id TEXT);", "CREATE TABLE x (id TEXT PRIMARY KEY)", "CREATE TABLE x (id TEXT CHECK(id <> 'x));"} {
		if _, err := splitStatements(invalid); invalid != "CREATE TABLE x (id TEXT);" && err == nil {
			t.Fatalf("invalid SQL %q accepted", invalid)
		}
	}
}

func TestEvaluateValidatesProgramAuthorityAndTypes(t *testing.T) {
	compiled, err := Load(validFS(validDDL), ".", "agent-guard")
	if err != nil {
		t.Fatal(err)
	}
	input := EvaluationInput{
		Meta:  map[string]any{"position": 7, "event_id": "local:7", "event_type": "otlp_record"},
		Event: map[string]any{"id": "local:7", "signal": "log", "name": "tool.result", "source": "codex", "time_unix_nano": "7", "observed_unix_nano": nil, "trace_id": nil, "span_id": nil, "content_digest": "abc", "record": map[string]any{}},
		Rows:  map[string]any{},
	}
	result, err := compiled.Evaluate("normalize", input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events["otel_event"]) != 1 || len(result.Tables["coverage"].Upsert) != 1 {
		t.Fatalf("result = %#v", result)
	}

	program := compiled.Normalizer
	for name, output := range map[string]string{
		"extra event field": `{"decision":"effective","facts":[],"events":{"otel_event":[{"kind":"x","session_id":"s","tool":"t","success":true,"source_position":1,"extra":1}]},"tables":{}}`,
		"wrong type":        `{"decision":"effective","facts":[],"events":{"otel_event":[{"kind":"x","session_id":"s","tool":"t","success":"yes","source_position":1}]},"tables":{}}`,
		"undeclared table":  `{"decision":"effective","facts":[],"events":{},"tables":{"sessions":{"upsert":[]}}}`,
		"ineffective write": `{"decision":"ineffective","facts":[],"events":{},"tables":{"coverage":{"upsert":[{"harness":"x","capability":"x","state":"x","last_position":1}]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := compiled.validateOutput(program, []byte(output)); err == nil {
				t.Fatal("invalid output accepted")
			}
		})
	}
}

func TestSourcePathsAndBounds(t *testing.T) {
	for _, name := range []string{"../fold.jsonata", "/fold.jsonata", "folds//x.jsonata", `folds\\x.jsonata`} {
		if err := validateSourcePath(name); err == nil {
			t.Fatalf("invalid path %q accepted", name)
		}
	}
	files := validFS(validDDL)
	files["folds/unused.jsonata"] = &fstest.MapFile{Data: []byte(`{}`)}
	if _, err := Load(files, ".", "agent-guard"); err == nil || !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("unused source accepted: %v", err)
	}
}

func TestJSONataSubsetRejectsUnboundedExtensionPoints(t *testing.T) {
	for name, program := range map[string]string{
		"user function":          `(function($x){$x})(1)`,
		"unicode lambda":         `(λ($x){$x})(1)`,
		"higher order":           `$map([1,2], function($x){$x})`,
		"unknown builtin":        `$mystery(event)`,
		"apply keys":             `event.record ~> $keys`,
		"apply spread":           `event.record ~> $spread`,
		"apply sort":             `event.record ~> $sort`,
		"apply base64encode":     `event.record ~> $base64encode`,
		"apply dynamic function": `event.record ~> event.callable`,
		"object wildcard":        `event.record.*`,
		"descendant wildcard":    `event.record.**`,
		"generative multiply":    `[1..4096].[1..4096].($ * 2)`,
		"generated range":        `[1..4096]`,
	} {
		t.Run(name, func(t *testing.T) {
			files := validFS(validDDL)
			files["folds/observe.jsonata"] = &fstest.MapFile{Data: []byte(program)}
			if _, err := Load(files, ".", "agent-guard"); err == nil || !strings.Contains(err.Error(), "bounded profile") {
				t.Fatalf("program accepted: %v", err)
			}
		})
	}
}

func TestJSONataSubsetAllowsApplyOperatorForAllowlistedFunction(t *testing.T) {
	files := validFS(validDDL)
	files["folds/observe.jsonata"] = &fstest.MapFile{Data: []byte(`event.session_id ~> $string`)}
	if _, err := Load(files, ".", "agent-guard"); err != nil {
		t.Fatalf("allowlisted apply rejected: %v", err)
	}
}

func TestJSONataAsteriskInsideStringIsAllowed(t *testing.T) {
	if err := validateJSONataSource([]byte(`{"pattern":"*"}`)); err != nil {
		t.Fatal(err)
	}
}
