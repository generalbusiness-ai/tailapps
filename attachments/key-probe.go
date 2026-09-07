package main
import("encoding/json";"fmt";j "github.com/jsonata-go/jsonata/v206")
func main(){
 cases:=[]struct{Name,Input,Source string}{
 {"two-sites","[1,2]",`[$probe("same"),$probe("same")]`},
 {"false-branch","[1,2]",`false ? $probe("unused") : "ok"`},
 {"item-path","[1,2]",`$.($probe("item"))`},
 {"constructor-key","[1,2]",`{$probe("key"):$}`},
 {"whole-only-provider","[1,2]",`{$count($)>1 ? $probe("probe-only") : "k":$}`},
 {"group-value","[1,2]",`{"k":$probe("value")}`},
 {"mixed-key-precheck-refusal","[1,2]",`{$count($)>1 ? 7 : $probe("k"):$}`},
 {"mixed-key-legacy-control","[1,2]",`{$count($)>1 ? 7 : "k":$}`},
 {"unselected-extension-syntax","[1,2]",`{false ? $probe("unused") : ($count($)>1 ? 7 : "k"):$}`},
 {"empty-key","[]",`{$probe("k"):$}`},
 {"null-key","null",`{$probe("k"):$}`},
 {"invalid-item-key","[1,2]",`{$count($)>1 ? $probe("whole") : 7:$}`},
 }
 for _,c:=range cases{calls:=[]string{};e,err:=j.Compile(c.Source,false);if err!=nil{panic(err)};err=e.RegisterFunction("probe",func(args []interface{})(interface{},error){calls=append(calls,args[0].(string));return args[0],nil},"<s:s>");if err!=nil{panic(err)};out,err:=e.Evaluate([]byte(c.Input),nil);failure:="";if err!=nil{failure=err.Error()};b,_:=json.Marshal(map[string]interface{}{"name":c.Name,"source":c.Source,"input":json.RawMessage(c.Input),"calls":calls,"output":string(out),"error":failure});fmt.Println(string(b))}
}
