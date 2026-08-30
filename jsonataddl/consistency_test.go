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
// behavior: sources are constructed purely from Dialect fields, so any drift
// between the description and the implementation fails here while behavior
// migrates into the core.

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

// dialectSources builds a minimal valid application purely from the dialect:
// the definition path, program root and suffix, host and private event
// names, and topology all come from the declared value, never from
// literals. If profile compiles it, the dialect describes reality.
func dialectSources(dialect jsonataddl.Dialect) fstest.MapFS {
	definition := fmt.Sprintf(`CREATE EVENT %s (
  key TEXT NOT NULL,
  source_position INTEGER NOT NULL
);

CREATE TABLE totals (
  key TEXT NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (key)
);

CREATE NORMALIZER normalize ON %s
USING '%s/normalize%s'
EMITS %s;

CREATE FOLD accumulate ON %s
READ prior OPTIONAL ONE AS
  SELECT key, total FROM totals WHERE key = :event.key
USING '%s/accumulate%s'
WRITES totals;

CREATE EXPORT totals AS
  SELECT key, total FROM totals;
`,
		dialect.PrivateEvent.Name,
		dialect.Topology.NormalizerConsumes,
		dialect.Layout.ProgramRoot, dialect.Layout.ProgramSuffix,
		dialect.Topology.NormalizerEmits,
		dialect.Topology.FoldsConsume,
		dialect.Layout.ProgramRoot, dialect.Layout.ProgramSuffix,
	)
	normalize := fmt.Sprintf(`{
  "decision": "effective",
  "facts": [],
  "events": {"%s": [{"key": "k", "source_position": meta.position}]},
  "tables": {}
}`, dialect.Topology.NormalizerEmits)
	accumulate := `{
  "decision": "effective",
  "facts": [],
  "tables": {"totals": {"upsert": [{"key": event.key, "total": 1}]}}
}`
	return fstest.MapFS{
		dialect.Layout.DefinitionPath:                                             &fstest.MapFile{Data: []byte(definition)},
		dialect.Layout.ProgramRoot + "/normalize" + dialect.Layout.ProgramSuffix:  &fstest.MapFile{Data: []byte(normalize)},
		dialect.Layout.ProgramRoot + "/accumulate" + dialect.Layout.ProgramSuffix: &fstest.MapFile{Data: []byte(accumulate)},
	}
}

func TestTailappDialectDescribesTheCompiler(t *testing.T) {
	dialect := jsonataddl.Tailapp()
	compiled, err := profile.Load(dialectSources(dialect), ".", "dialect-probe")
	if err != nil {
		t.Fatalf("sources built purely from the dialect do not compile: %v", err)
	}
	if compiled.Event.Name != dialect.PrivateEvent.Name {
		t.Fatalf("compiled private event %q, dialect declares %q", compiled.Event.Name, dialect.PrivateEvent.Name)
	}

	// The host event name is load-bearing: a normalizer consuming anything
	// else must be refused.
	wrong := dialectSources(dialect)
	definition := string(wrong[dialect.Layout.DefinitionPath].Data)
	definition = strings.Replace(definition, "ON "+dialect.Topology.NormalizerConsumes+"\n", "ON some_other_event\n", 1)
	wrong[dialect.Layout.DefinitionPath] = &fstest.MapFile{Data: []byte(definition)}
	if _, err := profile.Load(wrong, ".", "dialect-probe"); err == nil {
		t.Fatal("a normalizer consuming a foreign host event compiled; the dialect host event is not load-bearing")
	}

	// Fold event emission is closed by topology policy; the corpus's
	// fold-cannot-emit golden freezes the runtime half of this fact.
	if dialect.Topology.FoldsMayEmitEvents {
		t.Fatal("the Tailapp topology declares folds may emit events; profile refuses that")
	}

	// Single-writer policy is load-bearing: two folds writing one table must
	// be refused.
	doubled := dialectSources(dialect)
	definition = string(doubled[dialect.Layout.DefinitionPath].Data)
	second := fmt.Sprintf(`
CREATE FOLD accumulate_again ON %s
READ prior OPTIONAL ONE AS
  SELECT key, total FROM totals WHERE key = :event.key
USING '%s/accumulate%s'
WRITES totals;
`, dialect.Topology.FoldsConsume, dialect.Layout.ProgramRoot, dialect.Layout.ProgramSuffix)
	definition = strings.Replace(definition, "CREATE EXPORT", second+"\nCREATE EXPORT", 1)
	doubled[dialect.Layout.DefinitionPath] = &fstest.MapFile{Data: []byte(definition)}
	if !dialect.Topology.SingleWriterTables {
		t.Fatal("the Tailapp topology must declare single-writer tables")
	}
	if _, err := profile.Load(doubled, ".", "dialect-probe"); err == nil || !strings.Contains(err.Error(), "multiple writers") {
		t.Fatalf("two writers for one table were not refused as the dialect declares: %v", err)
	}
}
