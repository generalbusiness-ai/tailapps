package main
import("encoding/json";"errors";"fmt";"strings";j "github.com/jsonata-go/jsonata/v206")
func main(){
 e,err:=j.Compile(`$probe("INVITATION-CANARY-I7")`,false);if err!=nil{panic(err)};e.SetMaxDepth(64);e.SetMaxTime(2000)
 calls:=0
 out,first:=e.Evaluate([]byte(`{}`),map[string]interface{}{"probe":j.JSONataFunc(func(args []interface{})(interface{},error){calls++;return "fixed-result",nil})})
 _,absent:=e.Evaluate([]byte(`{}`),nil)
 _,leak:=e.Evaluate([]byte(`{}`),map[string]interface{}{"probe":j.JSONataFunc(func(args []interface{})(interface{},error){return nil,errors.New(fmt.Sprint(args[0]))})})
 b,_:=json.MarshalIndent(map[string]any{"pinned_jsonata":"v0.0.0-20250709164031-599f35f32e5f","scoped_call_succeeded":first==nil,"canonical_output":string(out),"calls":calls,"next_unbound_call_refused":absent!=nil,"raw_callback_error_contains_canary":leak!=nil&&strings.Contains(leak.Error(),"INVITATION-CANARY-I7")},"","  ");fmt.Println(string(b))
 if first!=nil||string(out)!=`"fixed-result"`||calls!=1||absent==nil||leak==nil||!strings.Contains(leak.Error(),"INVITATION-CANARY-I7"){panic("control expectations failed")}
}
