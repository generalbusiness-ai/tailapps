package main
import("encoding/json";"fmt";j "github.com/jsonata-go/jsonata/v206")
func main(){source:=`{($count($)>1?$substring("abc",1,1e300):"k"):$}`;e,err:=j.Compile(source,false);if err!=nil{panic(err)};out,err:=e.Evaluate([]byte(`[1,2]`),nil);failure:="";if err!=nil{failure=err.Error()};b,_:=json.Marshal(map[string]interface{}{"source":source,"input":"[1,2]","output":string(out),"error":failure});fmt.Println(string(b))}
