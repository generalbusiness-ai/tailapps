package tailapps
import (
 "fmt"
 "testing"
 "github.com/generalbusiness-ai/tailapps/internal/profile"
)
func TestPlannerLoopRunTimestampProbe(t *testing.T) {
 guard, err := Load("agent-guard"); if err != nil { t.Fatal(err) }
 input := observedCodexInputs(t)["tool_result"]
 normalized, err := guard.Evaluate("normalize_harness_event", input); if err != nil { t.Fatal(err) }
 base := normalized.Events["otel_event"][0]
 var session any
 var loop map[string]any
 for pos:=1; pos<=4; pos++ {
  event:=cloneMap(base)
  event["source_position"]=pos
  event["event_time_unix_nano"]=fmt.Sprintf("1787900000000000%03d",pos*100)
  event["success"]=pos==1
  event["action_fingerprint"]=fmt.Sprint(pos)
  result, err := guard.Evaluate("update_guard_analytics", profile.EvaluationInput{
   Meta:map[string]any{"position":pos,"event_id":fmt.Sprintf("probe-%d#0",pos),"event_type":"otel_event"},
   Event:event,Rows:map[string]any{"prior":session,"tool_coverage_prior":nil,"target_coverage_prior":nil,"progress_coverage_prior":nil},
  }); if err!=nil {t.Fatal(err)}
  session=result.Tables["session_progress"].Upsert[0]
  if pos<4 && len(result.Tables["loop_findings"].Upsert)>0 { t.Fatalf("unexpected early loop at %d",pos) }
  if pos==4 { loop=result.Tables["loop_findings"].Upsert[0] }
 }
 t.Logf("success t1; failures t2,t3,t4: loop=%#v",loop)
 if loop["finding_kind"]!="repeated-failure" {t.Fatalf("wrong finding: %#v",loop)}
 if loop["last_observed_unix_nano"]!="1787900000000000400" {t.Fatalf("wrong last: %#v",loop)}
 if loop["first_observed_unix_nano"]!="1787900000000000200" { t.Fatalf("first timestamp should be first failure t2, got %v (session started at t1)",loop["first_observed_unix_nano"]) }
}
