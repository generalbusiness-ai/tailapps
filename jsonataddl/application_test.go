package jsonataddl

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ncruces/go-sqlite3"
)

// renamedDialect is a complete host policy sharing no name with the Tailapp
// dialect: different layout, different event names, different envelope. An
// application translated into these names must compile and evaluate
// identically, which is the mechanical proof that core code special-cases
// no host name.
func renamedDialect() Dialect {
	dialect := Tailapp()
	dialect.Identity = DialectIdentity{Name: "renamed-host", Version: "9"}
	dialect.Layout = SourceLayout{DefinitionPath: "app.ddl", ProgramRoot: "programs", ProgramSuffix: ".jn"}
	dialect.HostEvent = NewEventContract("host_record",
		EnvelopeField{Name: "id", Type: "TEXT", Nullable: false},
		EnvelopeField{Name: "topic", Type: "TEXT", Nullable: true},
	)
	dialect.PrivateEvent = PrivateEventPolicy{Name: "inner_event", ExactlyOne: true}
	return dialect
}

func renamedSources() fstest.MapFS {
	return fstest.MapFS{
		"app.ddl": &fstest.MapFile{Data: []byte(`
CREATE EVENT inner_event (
  key TEXT NOT NULL,
  amount INTEGER NOT NULL
);

CREATE TABLE tallies (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE TABLE seen_topics (
  topic TEXT NOT NULL,
  PRIMARY KEY (topic)
);

CREATE NORMALIZER shape ON host_record
READ mine OPTIONAL ONE AS
  SELECT topic FROM seen_topics WHERE topic = :event.topic
USING 'programs/shape.jn'
WRITES seen_topics
EMITS inner_event;

CREATE FOLD tally ON inner_event
READ prior OPTIONAL ONE AS
  SELECT key, total FROM tallies WHERE key = :event.key
USING 'programs/tally.jn'
WRITES tallies;
`)},
		"programs/shape.jn": &fstest.MapFile{Data: []byte(`{
  "decision": "effective",
  "facts": [],
  "events": {"inner_event": [{"key": event.body.key, "amount": event.body.amount}]},
  "tables": {"seen_topics": {"upsert": [{"topic": event.topic}]}}
}`)},
		"programs/tally.jn": &fstest.MapFile{Data: []byte(`(
  $old := rows.prior;
  {
    "decision": "effective",
    "facts": [],
    "tables": {"tallies": {"upsert": [{
      "key": event.key,
      "total": $old ? $old.total + event.amount : event.amount
    }]}}
  }
)`)},
	}
}

func TestRenamedDialectCompilesAndEvaluates(t *testing.T) {
	sources := renamedSources()
	sources["app.ddl"] = &fstest.MapFile{Data: append(sources["app.ddl"].Data, []byte("\nCREATE EXPORT tallies AS SELECT key, total FROM tallies;\n")...)}
	application, err := LoadApplication(sources, ".", "renamed-app", renamedDialect(), "renamed-runtime@1")
	if err != nil {
		t.Fatalf("compile under renamed dialect: %v", err)
	}
	if application.RuntimeProfile() != "renamed-runtime@1" {
		t.Fatalf("runtime profile = %q", application.RuntimeProfile())
	}
	// A normalizer under the renamed dialect consumes host_record, may read
	// its own tables with envelope parameters, and emits only inner_event.
	result, err := application.Evaluate("shape", EvaluationInput{
		Meta:  map[string]any{"position": 1},
		Event: map[string]any{"id": "r-1", "topic": "alpha", "body": map[string]any{"key": "alpha", "amount": 5}},
		Rows:  map[string]any{"mine": nil},
	})
	if err != nil {
		t.Fatalf("normalizer under renamed dialect: %v", err)
	}
	emitted := result.Events["inner_event"]
	if len(emitted) != 1 || emitted[0]["key"] != "alpha" {
		t.Fatalf("emitted = %#v", result.Events)
	}
	folded, err := application.Evaluate("tally", EvaluationInput{
		Meta:  map[string]any{"position": 1},
		Event: emitted[0],
		Rows:  map[string]any{"prior": nil},
	})
	if err != nil {
		t.Fatalf("fold under renamed dialect: %v", err)
	}
	if len(folded.Tables["tallies"].Upsert) != 1 {
		t.Fatalf("fold mutation plan = %#v", folded.Tables)
	}
	// The dialect names surface in diagnostics exactly as configured.
	if _, err := application.Evaluate("shape", EvaluationInput{Event: map[string]any{"body": map[string]any{}}}); err == nil ||
		!strings.Contains(err.Error(), "inner_event") {
		t.Fatalf("diagnostic must carry the configured private event name: %v", err)
	}
}

func TestRenamedDialectRejectsWrongEventNames(t *testing.T) {
	sources := renamedSources()
	definition := strings.Replace(string(sources["app.ddl"].Data), "EMITS inner_event", "EMITS otel_event", 1)
	sources["app.ddl"] = &fstest.MapFile{Data: []byte(definition)}
	_, err := LoadApplication(sources, ".", "renamed-app", renamedDialect(), "renamed-runtime@1")
	if err == nil || !strings.Contains(err.Error(), `normalizer "shape" may emit only inner_event`) {
		t.Fatalf("emitting a foreign event name must fail with the configured name: %v", err)
	}
}

func TestLoadApplicationRequiresRuntimeIdentity(t *testing.T) {
	_, err := LoadApplication(renamedSources(), ".", "renamed-app", renamedDialect(), "")
	if err == nil || !strings.Contains(err.Error(), "runtime profile identity is required") {
		t.Fatalf("missing runtime identity must fail: %v", err)
	}
}

func TestUnsupportedDialectPoliciesAreRefused(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Dialect)
		want   string
	}{
		{"folds-may-emit", func(d *Dialect) { d.Topology.FoldsMayEmitEvents = true }, "outside the implemented topology"},
		{"multiple-normalizers", func(d *Dialect) { d.Topology.ExactlyOneNormalizer = false }, "outside the implemented topology"},
		{"optional-private-event", func(d *Dialect) { d.PrivateEvent.ExactlyOne = false }, "outside the implemented topology"},
		{"shared-writers", func(d *Dialect) { d.Authority.SingleWriterTables = false }, "single-writer tables"},
		{"unknown-visibility", func(d *Dialect) { d.Authority.FoldReads = "everything" }, "not implemented"},
		{"colliding-events", func(d *Dialect) { d.PrivateEvent.Name = d.HostEvent.Name }, "must be distinct"},
		{"zero-limit", func(d *Dialect) { d.Limits.MaxFacts = 0 }, "must all be positive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dialect := renamedDialect()
			tc.mutate(&dialect)
			_, err := LoadApplication(renamedSources(), ".", "renamed-app", dialect, "renamed-runtime@1")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("policy %s must be refused with %q: %v", tc.name, tc.want, err)
			}
		})
	}
}

func loadCorpusApplication(t *testing.T, dir string) *Application {
	t.Helper()
	application, err := LoadApplication(os.DirFS(dir), ".", "corpus-app", Tailapp(), "test-runtime@0")
	if err != nil {
		t.Fatal(err)
	}
	return application
}

func TestCompiledHandleIsImmutable(t *testing.T) {
	application := loadCorpusApplication(t, "corpus/v1/projection-state/app")
	before := application.StorageSchemaDigest()

	tables := application.Tables()
	for name, table := range tables {
		table.Columns[0].Name = "mutated"
		table.PrimaryKey[0] = "mutated"
		table.Writer = "mutated"
		tables[name] = table
	}
	plan, _ := application.ReadPlan("settle")
	for index := range plan {
		plan[index].SQL = "SELECT mutated"
		if len(plan[index].Parameters) > 0 {
			plan[index].Parameters[0] = "mutated"
		}
	}
	folds := application.Folds()
	for index := range folds {
		folds[index].Writes = append(folds[index].Writes[:0], "mutated")
	}
	event := application.Event()
	event.Columns[0].Name = "mutated"
	sources := application.Sources()
	for name := range sources {
		sources[name][0] = '!'
	}
	schema := application.SchemaSQL()
	if len(schema) > 0 {
		schema[0] = "DROP TABLE everything"
	}

	freshTables := application.Tables()
	for _, table := range freshTables {
		if table.Columns[0].Name == "mutated" || table.PrimaryKey[0] == "mutated" || table.Writer == "mutated" {
			t.Fatal("table mutation leaked into the compiled handle")
		}
	}
	freshPlan, _ := application.ReadPlan("settle")
	for _, read := range freshPlan {
		if read.SQL == "SELECT mutated" {
			t.Fatal("read plan mutation leaked into the compiled handle")
		}
		for _, parameter := range read.Parameters {
			if parameter == "mutated" {
				t.Fatal("read parameter mutation leaked into the compiled handle")
			}
		}
	}
	for _, fold := range application.Folds() {
		for _, write := range fold.Writes {
			if write == "mutated" {
				t.Fatal("fold writes mutation leaked into the compiled handle")
			}
		}
	}
	if application.Event().Columns[0].Name == "mutated" {
		t.Fatal("event mutation leaked into the compiled handle")
	}
	for _, content := range application.Sources() {
		if content[0] == '!' {
			t.Fatal("source mutation leaked into the compiled handle")
		}
	}
	if application.SchemaSQL()[0] == "DROP TABLE everything" {
		t.Fatal("schema SQL mutation leaked into the compiled handle")
	}
	if application.StorageSchemaDigest() != before {
		t.Fatal("identity changed under caller mutation")
	}
}

func TestReadAuthorizerDerivesFromThePlan(t *testing.T) {
	application := loadCorpusApplication(t, "corpus/v1/projection-state/app")
	authorizer, found := application.ReadAuthorizer("settle")
	if !found {
		t.Fatal("settle has a read plan and must have an authorizer")
	}
	decisions := []struct {
		name   string
		action sqlite3.AuthorizerActionCode
		args   [4]string
		want   sqlite3.AuthorizerReturnCode
	}{
		{"select-itself", sqlite3.AUTH_SELECT, [4]string{"", "", "", ""}, sqlite3.AUTH_OK},
		{"own-table-column", sqlite3.AUTH_READ, [4]string{"ledger", "balance", "main", ""}, sqlite3.AUTH_OK},
		{"plan-view", sqlite3.AUTH_READ, [4]string{"ledger_positive", "balance", "main", ""}, sqlite3.AUTH_OK},
		{"sibling-fold-table", sqlite3.AUTH_READ, [4]string{"shadow_notes", "note", "main", ""}, sqlite3.AUTH_DENY},
		{"undeclared-table", sqlite3.AUTH_READ, [4]string{"tailapp_frontier", "revision", "main", ""}, sqlite3.AUTH_DENY},
		{"undeclared-column", sqlite3.AUTH_READ, [4]string{"ledger", "secret", "main", ""}, sqlite3.AUTH_DENY},
		{"foreign-schema", sqlite3.AUTH_READ, [4]string{"ledger", "balance", "aux", ""}, sqlite3.AUTH_DENY},
		{"function", sqlite3.AUTH_FUNCTION, [4]string{"", "count", "", ""}, sqlite3.AUTH_DENY},
		{"pragma", sqlite3.AUTH_PRAGMA, [4]string{"schema_version", "", "main", ""}, sqlite3.AUTH_DENY},
		{"write", sqlite3.AUTH_DELETE, [4]string{"ledger", "", "main", ""}, sqlite3.AUTH_DENY},
		{"transaction", sqlite3.AUTH_TRANSACTION, [4]string{"BEGIN", "", "", ""}, sqlite3.AUTH_DENY},
	}
	for _, tc := range decisions {
		t.Run(tc.name, func(t *testing.T) {
			got := authorizer(tc.action, tc.args[0], tc.args[1], tc.args[2], tc.args[3])
			if got != tc.want {
				t.Fatalf("decision = %v, want %v", got, tc.want)
			}
		})
	}
	if _, found := application.ReadAuthorizer("nope"); found {
		t.Fatal("unknown program must have no authorizer")
	}
}

func TestContinueCompatibleOverCoreHandles(t *testing.T) {
	existing := loadCorpusApplication(t, "corpus/v1/projection-state/app")
	next := loadCorpusApplication(t, "corpus/v1/projection-state/app")
	if err := ContinueCompatible(existing, next); err != nil {
		t.Fatalf("identical applications must be continue-compatible: %v", err)
	}
	basic := loadCorpusApplication(t, "corpus/v1/basic/app")
	if err := ContinueCompatible(existing, basic); err == nil {
		t.Fatal("removing writable tables must not be continue-compatible")
	}
}

func TestValidateSourceHonorsTheLayout(t *testing.T) {
	dialect := renamedDialect()
	if err := ValidateSource(dialect, "programs/x.jn", []byte("1")); err != nil {
		t.Fatalf("layout program path must be admitted: %v", err)
	}
	if err := ValidateSource(dialect, "app.ddl", []byte("1")); err != nil {
		t.Fatalf("layout definition path must be admitted: %v", err)
	}
	if err := ValidateSource(dialect, "folds/x.jsonata", []byte("1")); err == nil {
		t.Fatal("a foreign layout's path must be rejected")
	}
	if err := ValidateSource(dialect, "programs/x.jn", nil); err == nil {
		t.Fatal("empty content must be rejected")
	}
}
