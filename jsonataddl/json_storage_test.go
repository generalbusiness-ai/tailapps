package jsonataddl

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	sqlitedriver "github.com/ncruces/go-sqlite3/driver"
)

func jsonStorageApplication(t *testing.T) *Application {
	t.Helper()
	app, err := compileApplication("json-storage", map[string][]byte{
		"application.sql": []byte(`
CREATE EVENT otel_event (id TEXT NOT NULL);
CREATE TABLE documents (id TEXT NOT NULL, document JSON, note TEXT CHECK(note <> 'JSON'), PRIMARY KEY(id));
CREATE NORMALIZER normalize ON otlp_record USING 'folds/normalize.jsonata' EMITS otel_event;
CREATE FOLD project ON otel_event
READ prior OPTIONAL ONE AS SELECT document FROM documents WHERE id=:event.id
USING 'folds/project.jsonata' WRITES documents;
CREATE EXPORT documents AS SELECT id,document FROM documents;
CREATE VIEW document_view AS SELECT id,document FROM documents;`),
		"folds/normalize.jsonata": []byte(`{"decision":"effective","facts":[],"tables":{}}`),
		"folds/project.jsonata":   []byte(`{"decision":"effective","facts":[],"tables":{}}`),
	}, Tailapp(), "json-storage-test")
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestJSONCompiledStorageRoundTrip(t *testing.T) {
	app := jsonStorageApplication(t)
	if app.Tables()["documents"].Columns[1].Type != TypeJSON || app.Exports()["documents"].Columns[1].Type != TypeJSON {
		t.Fatal("physical spelling escaped into logical table/export metadata")
	}
	if !strings.Contains(app.SchemaSQL()[0], "note <> 'JSON'") {
		t.Fatal("physical type rewrite changed a CHECK literal")
	}
	plan, ok := app.ReadPlan("project")
	if !ok || len(plan) != 1 {
		t.Fatal("missing compiled read plan")
	}
	for _, encoded := range []string{`42`, `0.125`, `1e3`, `-0`, `9007199254740993`, `1.00000000000000001`, `1e400`, `"42"`, `true`, `false`, `{"n":9007199254740993}`, `[1,"x",null]`, `null`} {
		t.Run(encoded, func(t *testing.T) {
			value, err := DecodeCanonical([]byte(encoded))
			if err != nil {
				t.Fatal(err)
			}
			if err = ValidateValue(value, TypeJSON, false); err != nil {
				t.Fatal(err)
			}
			db, err := sqlitedriver.Open(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for _, statement := range append(app.SchemaSQL(), app.ReplaceableSQL()...) {
				if _, err = db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = db.Exec("INSERT INTO documents (id,document) VALUES (?,?)", "a", SQLiteBindValue(value, TypeJSON)); err != nil {
				t.Fatal(err)
			}
			var storage string
			if err = db.QueryRow("SELECT typeof(document) FROM documents").Scan(&storage); err != nil {
				t.Fatal(err)
			}
			wantStorage := "text"
			if value == nil {
				wantStorage = "null"
			}
			if storage != wantStorage {
				t.Fatalf("JSON coerced to %s; want %s", storage, wantStorage)
			}
			for _, query := range []string{plan[0].SQL, "SELECT document AS payload FROM document_view WHERE id=?"} {
				rows, err := db.Query(query, "a")
				if err != nil {
					t.Fatal(err)
				}
				types, err := rows.ColumnTypes()
				if err != nil || len(types) != 1 || !rows.Next() {
					rows.Close()
					t.Fatalf("missing typed row: %v", err)
				}
				var raw any
				if err = rows.Scan(&raw); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				declared := LogicalType(types[0].DatabaseTypeName())
				if err = rows.Close(); err != nil {
					t.Fatal(err)
				}
				foldValue, err := ReadRowValue(raw, declared)
				if err != nil || !reflect.DeepEqual(foldValue, value) {
					t.Fatalf("fold value=%#v %v; want %#v", foldValue, err, value)
				}
				column := SQLiteColumn{Kind: ColumnNull}
				if raw != nil {
					text, ok := raw.(string)
					if !ok {
						t.Fatalf("JSON physical representation is %T", raw)
					}
					column = SQLiteColumn{Kind: ColumnText, Text: text}
				}
				queryValue, err := LogicalColumnValue(column, declared)
				if err != nil || !reflect.DeepEqual(queryValue, value) {
					t.Fatalf("query value=%#v %v; want %#v", queryValue, err, value)
				}
				reencoded, err := json.Marshal(queryValue)
				if err != nil || string(reencoded) != encoded {
					t.Fatalf("query changed JSON bytes: %s %v; want %s", reencoded, err, encoded)
				}
			}
		})
	}
}

func TestJSONPhysicalTypeIsNotSourceDDL(t *testing.T) {
	app := jsonStorageApplication(t)
	sources := app.Sources()
	sources["application.sql"] = []byte(strings.Replace(string(sources["application.sql"]), "document JSON", "document JSON_TEXT", 1))
	if _, err := compileApplication("json-storage", sources, Tailapp(), "json-storage-test"); err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("physical storage marker admitted in source DDL: %v", err)
	}
}

func TestJSONPhysicalShapeRefusesLegacyContinue(t *testing.T) {
	next := jsonStorageApplication(t)
	prior := *next
	prior.tables = next.Tables()
	table := prior.tables["documents"]
	table.StorageShape = strings.ReplaceAll(table.StorageShape, "json_text", "json")
	prior.tables["documents"] = table
	compiler := coreCompiler{app: &prior}
	if err := compiler.computeDigests(); err != nil {
		t.Fatal(err)
	}
	if prior.StorageSchemaDigest() == next.StorageSchemaDigest() {
		t.Fatal("physical storage change did not change the schema digest")
	}
	if err := ContinueCompatible(&prior, next); err == nil {
		t.Fatal("legacy JSON affinity allowed continue into TEXT storage")
	}
	if err := ContinueCompatible(next, &prior); err == nil {
		t.Fatal("TEXT storage allowed continue back into legacy JSON affinity")
	}
	if err := ContinueCompatible(next, jsonStorageApplication(t)); err != nil {
		t.Fatalf("identical physical storage refused: %v", err)
	}
}
