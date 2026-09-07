package main
import("encoding/json";"fmt";j "github.com/jsonata-go/jsonata/v206")
func run(source string)(result map[string]interface{}){
 result=map[string]interface{}{"source":source}
 defer func(){if value:=recover();value!=nil {result["panic"]=fmt.Sprint(value)}}()
 e,err:=j.Compile(source,false);if err!=nil {result["compile_error"]=err.Error();return}
 out,err:=e.Evaluate([]byte(`null`),nil);result["output"]=string(out);if err!=nil {result["error"]=err.Error()};return
}
func main(){
 for _,n:=range []string{`0/0`,`1/0`,`-1/0`,`1e300`,`-1e300`,`9223372036854775808`,`-9223372036854775808`,`1.5`,`-1.5`,`0`} {
  for _,source:=range []string{`$round(1.25,`+n+`)`,`$substring("abc",`+n+`)`,`$substring("abc",1,`+n+`)`,`$substring("abc",0,`+n+`)`} {
   b,_:=json.Marshal(run(source));fmt.Println(string(b))
  }
 }
}
