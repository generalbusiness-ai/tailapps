package main
import ("fmt"; "errors"; j "github.com/jsonata-go/jsonata/v206")
func main(){
 for _,src:=range []string{"$exists($reverse)","$exists($reverse[false])", "($true := false; $true ? $reverse := 3 : 0; $exists($reverse[false]))"}{
  e,err:=j.Compile(src,false);if err!=nil{panic(err)};seen:=0
  e.Assign(j.ExitCallbackSymbol,func(n,input any,f *j.Frame,v any)error{if node,yes:=n.(*j.ASTNode);yes && node.Type=="variable" && node.Value=="reverse" {if _,ok:=v.(*j.Function);ok{seen++;return errors.New("forbidden callable value")}};return nil})
  out,err:=e.Evaluate([]byte(`{}`),nil);fmt.Printf("%s => output=%s error=%v guard_function_observations=%d\n",src,out,err,seen)
 }
}
