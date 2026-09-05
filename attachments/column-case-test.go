package jsonataddl
import ("os"; "strings"; "testing"; "testing/fstest")
func TestPlannerReadColumnCasePreservesTypeAdmission(t *testing.T) {
 for _, selected := range []string{"balance", "BALANCE"} {
  t.Run(selected,func(t *testing.T) {
   files:=fstest.MapFS{}
   for _,name:=range []string{"application.sql","folds/normalize.jsonata","folds/settle.jsonata","folds/shadow.jsonata"} {
    data,err:=os.ReadFile("corpus/v1/projection-state/app/"+name);if err!=nil {t.Fatal(err)}
    files[name]=&fstest.MapFile{Data:data}
   }
   files["application.sql"].Data=[]byte(strings.Replace(string(files["application.sql"].Data),"SELECT key, balance\n  FROM ledger\n", "SELECT key, "+selected+"\n  FROM ledger\n",1))
   files["folds/settle.jsonata"].Data=[]byte(`{"decision":"effective","facts":[],"tables":{}}`)
   app,err:=LoadApplication(files,".","reads",Tailapp(),"test/1");if err!=nil {t.Fatal(err)}
   for _,value:=range []any{1,"wrong-type",nil} {
    input:=EvaluationInput{Meta:map[string]any{"position":1,"event_id":"r#0","event_type":"otel_event"},Event:map[string]any{"key":"k","amount":3,"retire":nil},Rows:map[string]any{"prior":map[string]any{"key":"k",selected:value},"marks":[]any{},"positive":nil}}
    _,err:=app.Evaluate("settle",input)
    if value==1 && err!=nil {t.Fatalf("valid integer refused: %v",err)}
    if value!=1 && err==nil {t.Errorf("selected %q admitted invalid nonnullable INTEGER value %#v",selected,value)}
   }
  })
 }
}
