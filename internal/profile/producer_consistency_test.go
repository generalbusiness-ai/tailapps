package profile_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/generalbusiness-ai/tailapps/internal/engine"
	"github.com/generalbusiness-ai/tailapps/internal/query"
	"github.com/generalbusiness-ai/tailapps/jsonataddl"
)

// TestDialectNullabilityMatchesTheCanonicalProducer binds every envelope
// field's nullability declaration to the actual canonical producer, both
// ways: crafted OTLP records drive the real ingest path so that every
// field the dialect declares nullable is observed null in at least one
// produced record (a wrongly non-nullable declaration fails the
// never-null assertion) and observed as a string in at least one (binding
// the TEXT type at the producer), while every non-nullable field must be
// present and non-null in every produced record (a wrongly nullable
// declaration fails the observed-null requirement).
func TestDialectNullabilityMatchesTheCanonicalProducer(t *testing.T) {
	dialect := jsonataddl.Tailapp()
	ctx := context.Background()
	resident, err := engine.Open(ctx, filepath.Join(t.TempDir(), "engine"))
	if err != nil {
		t.Fatal(err)
	}
	defer resident.Close()

	// A probe application built purely from the dialect: one nullable TEXT
	// event column per envelope field, copied verbatim into an exported
	// table keyed by the record id.
	fields := dialect.HostEvent.Fields()
	eventColumns := make([]string, 0, len(fields))
	tableColumns := make([]string, 0, len(fields))
	selectList := make([]string, 0, len(fields))
	copies := make([]string, 0, len(fields))
	for _, field := range fields {
		eventColumns = append(eventColumns, fmt.Sprintf("  %s TEXT%s", field.Name, map[bool]string{true: "", false: " NOT NULL"}[field.Nullable]))
		constraint := ""
		if field.Name == "id" {
			constraint = " NOT NULL"
		}
		tableColumns = append(tableColumns, fmt.Sprintf("  %s TEXT%s", field.Name, constraint))
		selectList = append(selectList, field.Name)
		copies = append(copies, fmt.Sprintf("    %q: event.%s", field.Name, field.Name))
	}
	definition := fmt.Sprintf(`CREATE EVENT %s (
%s,
  source_position INTEGER NOT NULL
);

CREATE TABLE probe_rows (
%s,
  PRIMARY KEY (id)
);

CREATE NORMALIZER normalize ON %s
USING '%s/normalize%s'
EMITS %s;

CREATE FOLD record ON %s
READ prior OPTIONAL ONE AS
  SELECT %s FROM probe_rows WHERE id = :event.id
USING '%s/record%s'
WRITES probe_rows;

CREATE EXPORT probe_rows AS
  SELECT %s FROM probe_rows;
`,
		dialect.PrivateEvent.Name, strings.Join(eventColumns, ",\n"),
		strings.Join(tableColumns, ",\n"),
		dialect.HostEvent.Name, dialect.Layout.ProgramRoot, dialect.Layout.ProgramSuffix, dialect.PrivateEvent.Name,
		dialect.PrivateEvent.Name, strings.Join(selectList, ", "), dialect.Layout.ProgramRoot, dialect.Layout.ProgramSuffix,
		strings.Join(selectList, ", "),
	)
	normalize := fmt.Sprintf(`{
  "decision": "effective",
  "facts": [],
  "events": {"%s": [{
%s,
    "source_position": meta.position
  }]},
  "tables": {}
}`, dialect.PrivateEvent.Name, strings.Join(copies, ",\n"))
	record := fmt.Sprintf(`{
  "decision": "effective",
  "facts": [],
  "tables": {"probe_rows": {"upsert": [{
%s
  }]}}
}`, strings.Join(copies, ",\n"))
	sources := map[string][]byte{
		dialect.Layout.DefinitionPath:                                            []byte(definition),
		dialect.Layout.ProgramRoot + "/normalize" + dialect.Layout.ProgramSuffix: []byte(normalize),
		dialect.Layout.ProgramRoot + "/record" + dialect.Layout.ProgramSuffix:    []byte(record),
	}
	if _, err := resident.Install(ctx, "envelope-probe", "", sources); err != nil {
		t.Fatalf("probe application from the dialect does not install: %v", err)
	}

	// Two crafted records through the real producer: the first with a zero
	// event time and absent trace identity, the second with a zero observed
	// time and full trace identity.
	payload := `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"probe-src"}}]},"scopeLogs":[{"logRecords":[
	  {"timeUnixNano":"0","observedTimeUnixNano":"111","eventName":"case.null-time","attributes":[{"key":"k","value":{"stringValue":"v"}}]},
	  {"timeUnixNano":"222","observedTimeUnixNano":"0","eventName":"case.null-observed","traceId":"AQIDBAUGBwgJCgsMDQ4PEA==","spanId":"AQIDBAUGBwg="}
	]}]}]}`
	request := httptest.NewRequest("POST", "/v1/logs", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	resident.Receiver().ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("ingest = %d %s", response.Code, response.Body.String())
	}
	if err := resident.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	result, err := resident.Query(ctx, "envelope-probe", query.Request{SQL: "SELECT " + strings.Join(selectList, ", ") + " FROM probe_rows ORDER BY id"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("produced rows = %d, want the two crafted records", len(result.Rows))
	}

	nullSeen := make(map[string]bool)
	stringSeen := make(map[string]bool)
	for _, row := range result.Rows {
		for index, field := range fields {
			value := row[index]
			switch value.(type) {
			case nil:
				nullSeen[field.Name] = true
			case string:
				stringSeen[field.Name] = true
			default:
				t.Fatalf("envelope field %q produced %T, dialect declares TEXT", field.Name, value)
			}
		}
	}
	for _, field := range fields {
		if field.Nullable && !nullSeen[field.Name] {
			t.Fatalf("field %q is declared nullable but no crafted producer case observed it null; the declaration is unproven", field.Name)
		}
		if !field.Nullable && nullSeen[field.Name] {
			t.Fatalf("field %q is declared non-nullable but the producer delivered null", field.Name)
		}
		if !stringSeen[field.Name] {
			t.Fatalf("field %q was never observed as a string; the TEXT declaration is unproven", field.Name)
		}
	}
}
