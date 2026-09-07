package jsonataddl
import("testing"; jsonata "github.com/jsonata-go/jsonata/v206")
func TestCodexTupleAliasPinnedConfinement(t *testing.T){
 source:=`($w := [[0,1,2]]; $rows := [{"a":$w,"b":$w},{"a":["A"],"b":["B"]}]; $rows.a@$a.b@$b{"one":{"a":$a,"b":$b}})`
 if ambientJSONataRE.MatchString(source) {t.Fatal("ambient refusal")}
 if err:=validateJSONataLexicalSource([]byte(source));err!=nil {t.Fatal(err)}
 expression,err:=jsonata.Compile(source,false);if err!=nil {t.Fatal(err)}
 if err:=validateJSONataAST(expression.AST());err!=nil {t.Fatal(err)}
 expression.SetMaxDepth(128);expression.SetMaxRange(1000);expression.SetMaxTime(evaluationWallTimeMilliseconds)
 for run:=0;run<2;run++ {
  outcomes:=map[string]int{}
  for i:=0;i<1000;i++ {value,err:=expression.Evaluate(nil,nil);if err!=nil {t.Fatal(err)};outcomes[string(value)]++}
  t.Log("run",run+1,"confinement accepted")
  for value,count:=range outcomes {t.Log(count,value)}
 }
}
