package mergeplan
import("context";"encoding/json";"os";"testing";"github.com/generalbusiness-ai/gitseq/internal/workroom")
func TestTail3925ActualSuccession(t *testing.T){
 raw,e:=os.ReadFile("/tmp/tail3925-succession-projection.json");if e!=nil{t.Fatal(e)}
 var s struct{Durable struct{Head string;Depth int;Projection workroom.Projection}};if e=json.Unmarshal(raw,&s);e!=nil{t.Fatal(e)}
 raw,e=os.ReadFile("/tmp/tail3925-changed-paths.json");if e!=nil{t.Fatal(e)};var changes []Change;if e=json.Unmarshal(raw,&changes);e!=nil{t.Fatal(e)}
 raw,e=os.ReadFile("/tmp/tail3925-reviewed-path-artifacts.json");if e!=nil{t.Fatal(e)};var ids []string;if e=json.Unmarshal(raw,&ids);e!=nil{t.Fatal(e)};reviewed:=map[string]bool{};for _,id:=range ids{reviewed[id]=true}
 const candidate="f36cb0eb2645cce40ebd30466406d0f293e6b455";const target="5d84ac71d625b29d9e7e9dbdf4c7c66c60429434";const other="git:sha1:da732b0bdaad4426ed4ad666b892d8a7c68f625f#git:sha1:7bfc28b144d68fefcfe29e24f991d15bdc18025b"
 classified,e:=Classify(context.Background(),"/Users/hughpyle/play/tailapp",s.Durable.Projection,changes,target,candidate,reviewed);if e!=nil{t.Fatal(e)};plan:=PlanSuccession(s.Durable.Projection,changes,classified)
 if _,retired:=plan.Retire[other];retired{t.Fatal("would retire current main publication")};if classified[other].Class!=ClassAbandoned{t.Fatalf("classifier behavior changed: %+v",classified[other])}
 out:=map[string]any{"frontier_head":s.Durable.Head,"frontier_depth":s.Durable.Depth,"candidate":candidate,"target_pre_head":target,"target_ref":"refs/heads/recovery/resident-5d84-pending","changes":changes,"classifications":classified,"succession":plan,"preserve_other_destination_artifact":other,"scope":"Read-only actual Classify/PlanSuccession result before independent approval; not merge admissibility or authorization. Current main publication3906 remains live despite the known abandoned classifier label; requester explicitly forbids cleaning it up."};b,e:=json.MarshalIndent(out,"","  ");if e!=nil{t.Fatal(e)};if e=os.WriteFile("/tmp/tail3925-succession-result.json",b,0600);e!=nil{t.Fatal(e)};t.Logf("publish=%d retire=%d left_live=%d; current main pointer preserved",len(plan.Publish),len(plan.Retire),len(plan.LeftLive))
}
