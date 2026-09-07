package jsonata
import "testing"
var numericProofCalls []string
func TestNumericDecisionOmissions(t *testing.T) {
 cases:=[]struct{source,want string;work,allocation uint64}{
 {`$round(1.25,0/0)`,`1`,83953394,410528},
 {`$substring("abc",0,1e300)`,`"abc"`,67196,82080},
 {`[1,2,3][0/0]`,`[1,2,3]`,50399006,299641},
 }
 for _,c:=range cases{for _,which:=range []string{"exact","work-short","allocation-short"}{
  p,e:=compileResourceProgram(c.source);if e!=nil{t.Fatal(e)};limit:=ResourceLimits{c.work,c.allocation,128};want:=error(nil)
  if which=="work-short"{limit.Work--;want=ResourceWork};if which=="allocation-short"{limit.Allocation--;want=ResourceAllocation}
  b:=NewResourceBudget(limit);s:=b.Scope(limit);out,e:=evaluateResourceProgram(&s,p,[]byte(`null`))
  if e!=want || (want==nil && string(out)!=c.want) || (want!=nil && out!=nil){t.Fatalf("%s %s got %s/%v used%+v; want%s/%v",c.source,which,out,e,b.Used(),c.want,want)}
 }}
}

func TestNumericDecisionProtectedSlice(t *testing.T) {
 source:=`{($count($)>1?$substring("abc",1,1e300):"k"):$}`
 p,e:=compileResourceProgram(source);if e!=nil{t.Fatal(e)};limit:=ResourceLimits{1<<44,1<<40,128};b:=NewResourceBudget(limit);scope:=b.Scope(limit)
 out,e:=evaluateResourceProgram(&scope,p,[]byte(`[1,2]`));if e!=ResourceContract||out!=nil||b.Failure()!=ResourceContract{t.Fatalf("protected group failure %s/%v",out,e)}
 // A declared invalid slice must be refused explicitly before Go's bounds panic.
 b=NewResourceBudget(limit);scope=b.Scope(limit);m:=createResourceFunctions(&scope)
 var problem interface{};func(){defer func(){problem=recover()}();m.substringFunc([]interface{}{"abc",float64(1),float64(1e300)})}()
 if problem!=ResourceContract||b.Failure()!=ResourceContract{t.Fatalf("unprotected native bounds panic: %T %v",problem,problem)}
 t.Log("private whole-input failure retained; invalid slice refused before native bounds panic")
}
