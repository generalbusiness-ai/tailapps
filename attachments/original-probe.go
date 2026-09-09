package main
import (
 "encoding/json"
 "fmt"
 "testing/fstest"
 core "github.com/generalbusiness-ai/tailapps/jsonataddl"
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
func main() {
 cases := []struct{Name, Prefix, Value string}{
  {"direct_keys", "", "$string($keys(event))"},
  {"allowed_direct", "", "$string(event.session_id)"},
  {"rebound_call", "$lowercase := $keys;", "$string($lowercase(event))"},
  {"rebound_apply", "$lowercase := $keys;", "$string(event ~> $lowercase)"},
 }
 for _, c := range cases {
  program := "("+c.Prefix+`{"decision":"effective","facts":[],"tables":{"sessions":{"upsert":[{"session_id":`+c.Value+`,"calls":1,"failures":0}]}}})`
  files := fstest.MapFS{"application.sql":{Data:[]byte(validDDL)},"folds/normalize.jsonata":{Data:[]byte(normalizeJSONata)},"folds/observe.jsonata":{Data:[]byte(program)}}
  app, err := core.LoadApplication(files,".","probe",core.Tailapp(),"probe-only")
  out := map[string]any{"case":c.Name,"program":program,"load_accepted":err==nil}
  if err != nil {out["load_error"]=err.Error()} else {
   result, evalErr := app.Evaluate("observe", core.EvaluationInput{
    Meta:map[string]any{"position":1,"event_id":"event-1","event_type":"otel_event"},
    Event:map[string]any{"kind":"tool-result","session_id":"s1","tool":"shell","success":false,"source_position":1},
    Rows:map[string]any{"current":nil},
   })
   out["evaluation_accepted"]=evalErr==nil
   if evalErr != nil {out["evaluation_error"]=evalErr.Error()} else {out["result"]=result}
  }
  b,_:=json.Marshal(out);fmt.Println(string(b))
 }
}
