package jsonata
import("encoding/json";"testing")
func TestCodexTupleAliasMechanism(t *testing.T){
 for _,capacity:=range []int{3,4} {
  outcomes:=map[string]int{}
  for trial:=0;trial<1000;trial++ {
   shared:=make([]interface{},3,capacity);for i:=range shared {shared[i]=float64(i)}
   input:=[]interface{}{map[string]interface{}{"a":shared,"b":shared},map[string]interface{}{"a":"A","b":"B"}}
   value:=reduceTupleStream(input);raw,err:=json.Marshal(value);if err!=nil {t.Fatal(err)};outcomes[string(raw)]++
  }
  t.Log("initial shared slice len3 capacity",capacity)
  for output,count:=range outcomes {t.Log(count,output)}
 }
}
