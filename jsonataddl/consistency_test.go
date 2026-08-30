package jsonataddl_test

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/generalbusiness-ai/tailapps/internal/profile"
	"github.com/generalbusiness-ai/tailapps/jsonataddl"
)

// These tests bind the declared Tailapp dialect to internal/profile's actual
// behavior. Every source set is constructed purely from Dialect fields, and
// every load-bearing field has a case whose outcome flips if the field or
// the implementation drifts: layout paths, host event name and envelope
// names, private event name and cardinality, topology cardinalities, read
// and write authority, emission policy, and limits.

const passthroughProgram = `{"decision": "effective", "facts": [], "tables": {}}`

// application assembles a source set from the dialect plus DDL statements
// built by the caller; program files come from the dialect's layout.
func application(dialect jsonataddl.Dialect, definition string, programs map[string]string) fstest.MapFS {
	files := fstest.MapFS{
		dialect.Layout.DefinitionPath: &fstest.MapFile{Data: []byte(definition)},
	}
	for name, source := range programs {
		files[dialect.Layout.ProgramRoot+"/"+name+dialect.Layout.ProgramSuffix] = &fstest.MapFile{Data: []byte(source)}
	}
	return files
}

func program(dialect jsonataddl.Dialect, name string) string {
	return dialect.Layout.ProgramRoot + "/" + name + dialect.Layout.ProgramSuffix
}

// standardDefinition renders the smallest valid application from the
// dialect: one private event, one fold-owned table, one normalizer, one
// fold, one export.
func standardDefinition(dialect jsonataddl.Dialect) string {
	return fmt.Sprintf(`CREATE EVENT %s (
  key TEXT NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE totals (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE NORMALIZER normalize ON %s
USING '%s'
EMITS %s;

CREATE FOLD accumulate ON %s
READ prior OPTIONAL ONE AS
  SELECT key, total FROM totals WHERE key = :event.key
USING '%s'
WRITES totals;

CREATE EXPORT totals AS
  SELECT key, total FROM totals;
`,
		dialect.PrivateEvent.Name,
		dialect.HostEvent.Name, program(dialect, "normalize"), dialect.PrivateEvent.Name,
		dialect.PrivateEvent.Name, program(dialect, "accumulate"),
	)
}

func standardPrograms(dialect jsonataddl.Dialect) map[string]string {
	normalize := fmt.Sprintf(`{
  "decision": "effective",
  "facts": [],
  "events": {"%s": [{"key": "k", "source_position": meta.position}]},
  "tables": {}
}`, dialect.PrivateEvent.Name)
	accumulate := `{
  "decision": "effective",
  "facts": [],
  "tables": {"totals": {"upsert": [{"key": event.key, "total": 1}]}}
}`
	return map[string]string{"normalize": normalize, "accumulate": accumulate}
}

func mustFail(t *testing.T, files fstest.MapFS, wantFragment, why string) {
	t.Helper()
	_, err := profile.Load(files, ".", "dialect-probe")
	if err == nil {
		t.Fatalf("%s: compiled, want failure containing %q", why, wantFragment)
	}
	if wantFragment != "" && !strings.Contains(err.Error(), wantFragment) {
		t.Fatalf("%s: error %q, want containing %q", why, err, wantFragment)
	}
}

func TestTailappDialectLimitsMatchProfile(t *testing.T) {
	limits := jsonataddl.Tailapp().Limits
	observed := map[string][2]int{
		"MaxElementBytes": {limits.MaxElementBytes, profile.MaxElementBytes},
		"MaxSourceBytes":  {limits.MaxSourceBytes, profile.MaxSourceBytes},
		"MaxProgramBytes": {limits.MaxProgramBytes, profile.MaxProgramBytes},
		"MaxInputBytes":   {limits.MaxInputBytes, profile.MaxInputBytes},
		"MaxOutputBytes":  {limits.MaxOutputBytes, profile.MaxOutputBytes},
		"MaxDepth":        {limits.MaxDepth, profile.MaxDepth},
		"MaxRange":        {limits.MaxRange, profile.MaxRange},
		"MaxEvents":       {limits.MaxEvents, profile.MaxEvents},
		"MaxFacts":        {limits.MaxFacts, profile.MaxFacts},
		"MaxRowChanges":   {limits.MaxRowChanges, profile.MaxRowChanges},
		"MaxManyRows":     {limits.MaxManyRows, profile.MaxManyRows},
	}
	for name, pair := range observed {
		if pair[0] != pair[1] {
			t.Fatalf("dialect %s = %d, profile enforces %d", name, pair[0], pair[1])
		}
	}
}

func TestDialectLayoutAndEventNamesAreLoadBearing(t *testing.T) {
	dialect := jsonataddl.Tailapp()
	compiled, err := profile.Load(application(dialect, standardDefinition(dialect), standardPrograms(dialect)), ".", "dialect-probe")
	if err != nil {
		t.Fatalf("sources built purely from the dialect do not compile: %v", err)
	}
	if compiled.Event.Name != dialect.PrivateEvent.Name {
		t.Fatalf("compiled private event %q, dialect declares %q", compiled.Event.Name, dialect.PrivateEvent.Name)
	}

	// The definition path is load-bearing: without it nothing compiles.
	missing := application(dialect, standardDefinition(dialect), standardPrograms(dialect))
	delete(missing, dialect.Layout.DefinitionPath)
	mustFail(t, missing, "", "an application without the dialect definition path")

	// The host event name is load-bearing: a normalizer consuming anything
	// else is refused.
	foreign := strings.Replace(standardDefinition(dialect), "ON "+dialect.HostEvent.Name+"\n", "ON some_other_event\n", 1)
	mustFail(t, application(dialect, foreign, standardPrograms(dialect)), "", "a normalizer consuming a foreign host event")

	// The private event name is fixed by policy: another name is refused.
	if !dialect.PrivateEvent.ExactlyOne {
		t.Fatal("the Tailapp dialect must require exactly one private event")
	}
	renamed := strings.ReplaceAll(standardDefinition(dialect), dialect.PrivateEvent.Name, "renamed_event")
	programs := standardPrograms(dialect)
	programs["normalize"] = strings.ReplaceAll(programs["normalize"], dialect.PrivateEvent.Name, "renamed_event")
	mustFail(t, application(dialect, renamed, programs), "", "a private event with a non-dialect name")

	// Exactly one private event: declaring a second is refused.
	doubled := strings.Replace(standardDefinition(dialect), "CREATE TABLE", fmt.Sprintf("CREATE EVENT second_event (\n  key TEXT NOT NULL\n);\n\nCREATE TABLE"), 1)
	mustFail(t, application(dialect, doubled, standardPrograms(dialect)), "", "an application with two private events")
}

func TestDialectEnvelopeNamesAreLoadBearing(t *testing.T) {
	dialect := jsonataddl.Tailapp()
	if len(dialect.HostEvent.ScalarFields) == 0 {
		t.Fatal("the Tailapp dialect declares no envelope fields")
	}
	// A normalizer-owned table read whose parameters use every declared
	// envelope field must compile: the declared names are exactly the
	// parameter vocabulary profile admits for host-event programs.
	conditions := make([]string, 0, len(dialect.HostEvent.ScalarFields))
	for _, field := range dialect.HostEvent.ScalarFields {
		conditions = append(conditions, "key = :event."+field.Name)
	}
	build := func(where string) fstest.MapFS {
		definition := fmt.Sprintf(`CREATE EVENT %s (
  key TEXT NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE norm_state (
  key TEXT NOT NULL,
  PRIMARY KEY (key)
);

CREATE TABLE totals (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE NORMALIZER normalize ON %s
READ prior OPTIONAL ONE AS
  SELECT key FROM norm_state WHERE %s
USING '%s'
WRITES norm_state EMITS %s;

CREATE FOLD accumulate ON %s
READ prior OPTIONAL ONE AS
  SELECT key, total FROM totals WHERE key = :event.key
USING '%s'
WRITES totals;

CREATE EXPORT totals AS
  SELECT key, total FROM totals;
`,
			dialect.PrivateEvent.Name,
			dialect.HostEvent.Name, where, program(dialect, "normalize"), dialect.PrivateEvent.Name,
			dialect.PrivateEvent.Name, program(dialect, "accumulate"),
		)
		return application(dialect, definition, standardPrograms(dialect))
	}
	if _, err := profile.Load(build(strings.Join(conditions, " AND ")), ".", "dialect-probe"); err != nil {
		t.Fatalf("a read using every declared envelope field does not compile: %v", err)
	}
	mustFail(t, build("key = :event.not_in_the_envelope"), "undeclared scalar event field", "a read using an undeclared envelope field")
}

func TestDialectTopologyCardinalitiesAreLoadBearing(t *testing.T) {
	dialect := jsonataddl.Tailapp()
	if !dialect.Topology.ExactlyOneNormalizer || !dialect.Topology.AtLeastOneFold {
		t.Fatal("the Tailapp topology must require exactly one normalizer and at least one fold")
	}
	definition := standardDefinition(dialect)

	zeroNormalizers := strings.Replace(definition, "CREATE NORMALIZER", "-- CREATE NORMALIZER", 1)
	zeroNormalizers = strings.Replace(zeroNormalizers, "USING '"+program(dialect, "normalize")+"'\nEMITS "+dialect.PrivateEvent.Name+";", "", 1)
	zeroNormalizers = strings.Replace(zeroNormalizers, "-- CREATE NORMALIZER normalize ON "+dialect.HostEvent.Name, "", 1)
	mustFail(t, application(dialect, zeroNormalizers, standardPrograms(dialect)), "exactly one normalizer", "an application without a normalizer")

	second := fmt.Sprintf(`
CREATE NORMALIZER normalize_again ON %s
USING '%s'
EMITS %s;
`, dialect.HostEvent.Name, program(dialect, "normalize"), dialect.PrivateEvent.Name)
	twoNormalizers := strings.Replace(definition, "CREATE FOLD", second+"\nCREATE FOLD", 1)
	mustFail(t, application(dialect, twoNormalizers, standardPrograms(dialect)), "more than one normalizer", "an application with two normalizers")

	zeroFolds := definition
	foldStart := strings.Index(zeroFolds, "CREATE FOLD")
	foldEnd := strings.Index(zeroFolds, "CREATE EXPORT")
	zeroFolds = zeroFolds[:foldStart] + zeroFolds[foldEnd:]
	mustFail(t, application(dialect, zeroFolds, standardPrograms(dialect)), "at least one analytic fold", "an application without folds")
}

func TestDialectAuthorityIsLoadBearing(t *testing.T) {
	dialect := jsonataddl.Tailapp()
	if dialect.Authority.NormalizerReads != jsonataddl.ReadOwnTables ||
		dialect.Authority.FoldReads != jsonataddl.ReadOwnAndNormalizerTables ||
		!dialect.Authority.SingleWriterTables {
		t.Fatalf("the Tailapp authority policy = %#v does not match the enforced rules", dialect.Authority)
	}
	if dialect.Topology.FoldsMayEmitEvents {
		t.Fatal("the Tailapp topology declares folds may emit events; profile refuses that")
	}

	build := func(normalizerTail, foldRead string) fstest.MapFS {
		definition := fmt.Sprintf(`CREATE EVENT %s (
  key TEXT NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE norm_state (
  key TEXT NOT NULL,
  PRIMARY KEY (key)
);

CREATE TABLE totals (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE NORMALIZER normalize ON %s
%sUSING '%s'
WRITES norm_state EMITS %s;

CREATE FOLD accumulate ON %s
READ prior OPTIONAL ONE AS
  SELECT %s
USING '%s'
WRITES totals;

CREATE EXPORT totals AS
  SELECT key, total FROM totals;
`,
			dialect.PrivateEvent.Name,
			dialect.HostEvent.Name, normalizerTail, program(dialect, "normalize"), dialect.PrivateEvent.Name,
			dialect.PrivateEvent.Name, foldRead, program(dialect, "accumulate"),
		)
		return application(dialect, definition, standardPrograms(dialect))
	}

	// A fold reading the normalizer's table compiles: own-and-normalizer.
	if _, err := profile.Load(build("", "key FROM norm_state WHERE key = :event.key"), ".", "dialect-probe"); err != nil {
		t.Fatalf("a fold reading the normalizer's table does not compile: %v", err)
	}
	// A normalizer reading a fold's table is refused: own-tables only.
	normalizerReadsFold := "READ prior OPTIONAL ONE AS\n  SELECT key FROM totals WHERE key = :event.id\n"
	mustFail(t, build(normalizerReadsFold, "key, total FROM totals WHERE key = :event.key"), "normalizer", "a normalizer reading a fold-owned table")

	// A fold reading another fold's table is refused: never a sibling's.
	definition := standardDefinition(dialect)
	sibling := fmt.Sprintf(`
CREATE TABLE sibling_state (
  key TEXT NOT NULL,
  PRIMARY KEY (key)
);

CREATE FOLD sibling ON %s
READ prior OPTIONAL ONE AS
  SELECT key FROM sibling_state WHERE key = :event.key
USING '%s'
WRITES sibling_state;

CREATE FOLD snoop ON %s
READ prior OPTIONAL ONE AS
  SELECT key FROM sibling_state WHERE key = :event.key
USING '%s'
WRITES totals;
`, dialect.PrivateEvent.Name, program(dialect, "accumulate"), dialect.PrivateEvent.Name, program(dialect, "accumulate"))
	crossFold := strings.Replace(definition, "CREATE FOLD accumulate", sibling+"\nCREATE FOLD accumulate", 1)
	crossFold = strings.Replace(crossFold, `CREATE FOLD accumulate ON `+dialect.PrivateEvent.Name+`
READ prior OPTIONAL ONE AS
  SELECT key, total FROM totals WHERE key = :event.key
USING '`+program(dialect, "accumulate")+`'
WRITES totals;`, "", 1)
	mustFail(t, application(dialect, crossFold, standardPrograms(dialect)), "owned by analytic fold", "a fold reading a sibling fold's table")

	// Single-writer tables: a second writer for one table is refused.
	secondWriter := fmt.Sprintf(`
CREATE FOLD accumulate_again ON %s
READ prior OPTIONAL ONE AS
  SELECT key, total FROM totals WHERE key = :event.key
USING '%s'
WRITES totals;
`, dialect.PrivateEvent.Name, program(dialect, "accumulate"))
	doubled := strings.Replace(standardDefinition(dialect), "CREATE EXPORT", secondWriter+"\nCREATE EXPORT", 1)
	mustFail(t, application(dialect, doubled, standardPrograms(dialect)), "multiple writers", "two writers for one table")
}
