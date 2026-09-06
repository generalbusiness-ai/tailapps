package engine
import("context";"os";"path/filepath";"strings";"testing")
func TestPlannerInstallActivationFailureRetainsDraft(t *testing.T) {
 ctx:=context.Background();home:=filepath.Join(t.TempDir(),"home");e,err:=Open(ctx,home);if err!=nil{t.Fatal(err)};defer e.Close()
 if err=os.MkdirAll(filepath.Join(home,"projections"),0700);err!=nil{t.Fatal(err)}
 obstacle:=filepath.Join(home,"projections","session-cost");if err=os.WriteFile(obstacle,[]byte("isolated activation obstacle"),0600);err!=nil{t.Fatal(err)}
 _,err=e.Install(ctx,"session-cost","session-cost",nil);if err==nil||!strings.Contains(err.Error(),"validated draft remains"){t.Fatalf("install error=%v",err)};t.Logf("install error: %v",err)
 app,sources,err:=e.App(ctx,"session-cost");if err!=nil{t.Fatal(err)};if app.ActiveRevision!=nil||app.DraftRevision==""||len(sources)==0{t.Fatalf("retained state: app=%+v sources=%d",app,len(sources))};t.Logf("retained draft=%s active=nil sources=%d",app.DraftRevision,len(sources))
 if err=os.Remove(obstacle);err!=nil{t.Fatal(err)}
 _,err=e.Install(ctx,"session-cost","session-cost",nil);if err==nil||!strings.Contains(err.Error(),"already exists"){t.Fatalf("repeat install=%v",err)};t.Logf("repeat install after obstacle removal: %v",err)
 frontier,err:=e.Activate(ctx,"session-cost",app.DraftRevision,"reset",true);if err!=nil{t.Fatal(err)};t.Logf("existing draft explicit activation recovered: %+v",frontier)
}
