package projection

import (
	"context"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/generalbusiness-ai/tailapps/internal/profile"
	sqlitedriver "github.com/ncruces/go-sqlite3/driver"
)

func jsonUpgradeProfile(t *testing.T) *profile.Profile {
	t.Helper()
	current, err := profile.LoadCurrent(fstest.MapFS{
		"application.sql": {Data: []byte(`
CREATE EVENT otel_event (id TEXT NOT NULL);
CREATE TABLE documents (id TEXT PRIMARY KEY, document JSON);
CREATE NORMALIZER normalize ON otlp_record USING 'folds/normalize.jsonata' EMITS otel_event;
CREATE FOLD project ON otel_event USING 'folds/project.jsonata' WRITES documents;
CREATE EXPORT documents AS SELECT id,document FROM documents;`)},
		"folds/normalize.jsonata": {Data: []byte(`{"decision":"effective","facts":[],"tables":{}}`)},
		"folds/project.jsonata":   {Data: []byte(`{"decision":"effective","facts":[],"tables":{}}`)},
	}, ".", "json-upgrade")
	if err != nil {
		t.Fatal(err)
	}
	return current
}

func TestRecoveredLegacyJSONCannotContinueIntoTextStorage(t *testing.T) {
	ctx := context.Background()
	current := jsonUpgradeProfile(t)
	old := *current
	old.RuntimeProfile = "jsonata-ddl-runtime:sha256:5032bcfe6634db4462fcfe39775e55d1c7e11fcb7663c961f6ebe8860192b8b1"
	old.Revision = "old-json-revision"
	old.SchemaSQL = append([]string(nil), current.SchemaSQL...)
	for i := range old.SchemaSQL {
		old.SchemaSQL[i] = strings.ReplaceAll(old.SchemaSQL[i], "JSON_TEXT", "JSON")
	}
	path := filepath.Join(t.TempDir(), "state.sqlite")
	p, err := Create(ctx, path, &old, 0, "reset")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Database().Exec("INSERT INTO documents VALUES ('a',?)", `{"n":42}`); err != nil {
		t.Fatal(err)
	}
	if err = p.Close(); err != nil {
		t.Fatal(err)
	}
	if reused, err := Open(ctx, path, current); err == nil {
		reused.Close()
		t.Fatal("current runtime reused an old projection")
	}
	// Engine recovery compiles prior source with the current resolver before
	// this query-only opening. Its in-memory shape is not the stored shape.
	p, err = OpenForUpgrade(ctx, path, current, old.Revision)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	before, err := p.Frontier(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var schemaBefore string
	if err = p.db.QueryRow("SELECT sql FROM sqlite_master WHERE name='documents'").Scan(&schemaBefore); err != nil {
		t.Fatal(err)
	}
	if err = p.Continue(ctx, current, 9); err == nil {
		t.Fatal("recovered legacy JSON storage accepted unsafe continuation")
	}
	var revision, runtime, document string
	if err = p.Database().QueryRow("SELECT revision,runtime_profile FROM tailapp_projection_identity").Scan(&revision, &runtime); err != nil || revision != old.Revision || runtime != old.RuntimeProfile {
		t.Fatalf("failed continue changed identity: %s %s %v", revision, runtime, err)
	}
	if err = p.Database().QueryRow("SELECT document FROM documents WHERE id='a'").Scan(&document); err != nil || document != `{"n":42}` {
		t.Fatalf("failed continue changed row: %s %v", document, err)
	}
	after, err := p.Frontier(ctx)
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("failed continue changed frontier: %#v %v", after, err)
	}
	var schemaAfter string
	if err = p.db.QueryRow("SELECT sql FROM sqlite_master WHERE name='documents'").Scan(&schemaAfter); err != nil || schemaAfter != schemaBefore {
		t.Fatalf("failed continue changed schema: %s %v", schemaAfter, err)
	}
	uri := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	reader, err := sqlitedriver.Open(uri.String())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err = reader.QueryRow("SELECT document FROM documents WHERE id='a'").Scan(&document); err != nil || document != `{"n":42}` {
		t.Fatalf("query-only access lost: %s %v", document, err)
	}
	if _, err = reader.Exec("DELETE FROM documents"); err == nil {
		t.Fatal("query-only connection accepted writes")
	}
}

func TestJSONStorageContinuationControls(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"compatible", "incompatible", "missing-column", "missing-table"} {
		t.Run(kind, func(t *testing.T) {
			current := jsonUpgradeProfile(t)
			p, err := Create(ctx, filepath.Join(t.TempDir(), "state.sqlite"), current, 0, "reset")
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			if kind != "compatible" {
				if _, err = p.db.Exec("DROP TABLE documents"); err != nil {
					t.Fatal(err)
				}
				var replacement string
				switch kind {
				case "incompatible":
					replacement = "CREATE TABLE documents (id TEXT PRIMARY KEY, document BLOB)"
				case "missing-column":
					replacement = "CREATE TABLE documents (id TEXT PRIMARY KEY)"
				}
				if replacement != "" {
					if _, err = p.db.Exec(replacement); err != nil {
						t.Fatal(err)
					}
				}
				if err = p.Continue(ctx, current, 9); err == nil {
					t.Fatal("invalid actual JSON storage admitted")
				}
				return
			}
			if _, err = p.db.Exec("INSERT INTO documents VALUES ('a','42')"); err != nil {
				t.Fatal(err)
			}
			files := fstest.MapFS{}
			for name, source := range current.Sources {
				files[name] = &fstest.MapFile{Data: source}
			}
			files["application.sql"].Data = []byte(strings.Replace(string(files["application.sql"].Data), "WRITES documents;", "WRITES documents, extra;", 1) + "\nCREATE TABLE extra (id TEXT PRIMARY KEY, document JSON);")
			next, err := profile.LoadCurrent(files, ".", "json-upgrade")
			if err != nil {
				t.Fatal(err)
			}
			if err = p.Continue(ctx, next, 9); err != nil {
				t.Fatalf("compatible additive continuation refused: %v", err)
			}
			var document, declared string
			if err = p.db.QueryRow("SELECT document FROM documents").Scan(&document); err != nil || document != "42" {
				t.Fatalf("compatible continuation lost rows: %s %v", document, err)
			}
			if err = p.db.QueryRow("SELECT type FROM pragma_table_info('extra') WHERE name='document'").Scan(&declared); err != nil || declared != "JSON_TEXT" {
				t.Fatalf("additive table has wrong physical storage: %s %v", declared, err)
			}
			frontier, err := p.Frontier(ctx)
			if err != nil || frontier.InterpretedPosition != 9 {
				t.Fatalf("continuation boundary not advanced: %#v %v", frontier, err)
			}
		})
	}
}
