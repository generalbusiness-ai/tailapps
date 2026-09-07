package main
import("encoding/json";"fmt";j "github.com/jsonata-go/jsonata/v206")
func main(){
 cases:=[]struct{Name,Input,Source string}{
 {"round-nan-precision","null",`$round(1.25, 0/0)`},
 {"round-positive-huge-precision","null",`$round(1.25, 1e300)`},
 {"round-negative-huge-precision","null",`$round(1.25, -1e300)`},
 {"round-zero-precision","null",`$round(1.25, 0)`},
 {"round-min-int-precision","null",`$round(1.25, -9223372036854775808)`},
 }
 for _,c:=range cases{calls:=[]string{};e,err:=j.Compile(c.Source,false);if err!=nil{panic(err)};err=e.RegisterFunction("probe",func(args []interface{})(interface{},error){calls=append(calls,args[0].(string));return args[0],nil},"<s:s>");if err!=nil{panic(err)};out,err:=e.Evaluate([]byte(c.Input),nil);failure:="";if err!=nil{failure=err.Error()};b,_:=json.Marshal(map[string]interface{}{"name":c.Name,"source":c.Source,"input":json.RawMessage(c.Input),"calls":calls,"output":string(out),"error":failure});fmt.Println(string(b))}
}
