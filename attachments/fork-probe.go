package main
import("encoding/json";"fmt";j "github.com/generalbusiness-ai/jsonata/v206")
func main(){
 for _,s:=range []string{`[$probe("same"),$probe("same")]`,`false ? $probe("unused") : "ok"`,`$.($probe("item"))`,`{$probe("key"): $}`,`{$count($)>1 ? $probe("probe-only") : "k":$}`,`{"k":$probe("value")}`} {
  e,err:=j.Compile(s,false);if err!=nil{panic(err)};calls:=[]string{}
  err=e.RegisterFunction("probe",func(args []interface{})(interface{},error){calls=append(calls,args[0].(string));return args[0],nil},"<s:s>");if err!=nil{panic(err)}
  out,err:=e.Evaluate([]byte(`[1,2]`),nil);failure:="";if err!=nil{failure=err.Error()};b,_:=json.Marshal(map[string]interface{}{"source":s,"input":json.RawMessage(`[1,2]`),"calls":calls,"output":string(out),"error":failure});fmt.Println(string(b))
 }
}
