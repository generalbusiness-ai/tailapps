package tailapps
import (
 "encoding/json"
 "fmt"
 "testing"
 "time"
 "github.com/generalbusiness-ai/tailapps/internal/profile"
)
func TestPlannerSessionDayBucketProbe(t *testing.T) {
 stats, err := Load("activity-stats"); if err != nil { t.Fatal(err) }
 for _, cross := range []bool{false,true} {
  var prior any
  for i:=0;i<2;i++ {
   input:=observedCodexInputs(t)["sse_usage"]
   stamp:=time.Date(2026,9,6,23,58+i,0,0,time.UTC)
   if cross && i==1 {stamp=time.Date(2026,9,7,0,1,0,0,time.UTC)}
   input.Event["time_unix_nano"]=fmt.Sprint(stamp.UnixNano())
   input.Event["observed_unix_nano"]=fmt.Sprint(stamp.UnixNano())
   normalized,err:=stats.Evaluate("normalize_activity",input);if err!=nil {t.Fatal(err)}
   if len(normalized.Events["otel_event"])!=1 {t.Fatal("expected normalized event")}
   event:=normalized.Events["otel_event"][0]
   result,err:=stats.Evaluate("update_session_activity",profile.EvaluationInput{Meta:map[string]any{"position":i+1,"event_id":fmt.Sprint(i),"event_type":"otel_event"},Event:event,Rows:map[string]any{"prior":prior}});if err!=nil {t.Fatal(err)}
   prior=result.Tables["session_activity"].Upsert[0]
  }
  b,err:=json.Marshal(map[string]any{"cross_midnight":cross,"row":prior});if err!=nil{t.Fatal(err)}
  t.Log("DAY_ROW="+string(b))
 }
}
