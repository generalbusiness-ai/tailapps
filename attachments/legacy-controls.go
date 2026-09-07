// Source-only decision evidence: unchanged pinned reducer, not the proposed runtime.
package main
import("encoding/json";"fmt")
func reduceTupleStream(tupleStream interface{}) interface{} {
	arr, ok := tupleStream.([]interface{})
	if !ok {
		return tupleStream
	}
	if len(arr) == 0 {
		return tupleStream
	}

	result := make(map[string]interface{})
	// Copy first tuple
	if firstTuple, ok := arr[0].(map[string]interface{}); ok {
		for k, v := range firstTuple {
			result[k] = v
		}
	}

	// Merge remaining tuples
	for ii := 1; ii < len(arr); ii++ {
		if tuple, ok := arr[ii].(map[string]interface{}); ok {
			for prop, value := range tuple {
				if existing, exists := result[prop]; exists {
					if existingArr, ok := existing.([]interface{}); ok {
						result[prop] = append(existingArr, value)
					} else {
						result[prop] = []interface{}{existing, value}
					}
				} else {
					result[prop] = value
				}
			}
		}
	}
	return result
}

func raw(v interface{}) string { b,e:=json.Marshal(v);if e!=nil{panic(e)};return string(b) }
func same(got interface{},want string){if raw(got)!=want{panic(fmt.Sprintf("got %s want %s",raw(got),want))}}
func main(){
 for _,capacity:=range []int{3,4}{for _,independent:=range []bool{false,true}{for _,reverse:=range []bool{false,true}{
  a:=make([]interface{},3,capacity);copy(a,[]interface{}{0,1,2});b:=a
  if independent{b=make([]interface{},3,capacity);copy(b,a)}
  first:=map[string]interface{}{"a":a,"b":b}; later:=[]interface{}{map[string]interface{}{"a":"A"},map[string]interface{}{"b":"B"}}
  if reverse{later[0],later[1]=later[1],later[0]}
  // Single-field continuation tuples force append order through the unchanged function.
  out:=reduceTupleStream(append([]interface{}{first},later...));want:=`{"a":[0,1,2,"A"],"b":[0,1,2,"B"]}`
  if capacity==4&&!independent {if reverse{want=`{"a":[0,1,2,"A"],"b":[0,1,2,"A"]}`}else{want=`{"a":[0,1,2,"B"],"b":[0,1,2,"B"]}`}}
  same(out,want);fmt.Printf("capacity=%d independent=%t reverse=%t result=%s\n",capacity,independent,reverse,raw(out))
 }}}
 backing:=[]interface{}{0,1,2,"sentinel"};alias:=backing[:3]
 out:=reduceTupleStream([]interface{}{map[string]interface{}{"a":alias,"b":alias},map[string]interface{}{"a":"A"},map[string]interface{}{"b":"B"}})
 same(backing,`[0,1,2,"B"]`);fmt.Println("legacy borrowed append overwrites caller sentinel",raw(backing),"result",raw(out))
 tuple:=map[string]interface{}{"@":"context","a":1};single:=reduceTupleStream(tuple).(map[string]interface{});context:=single["@"];delete(single,"@")
 if _,exists:=tuple["@"];exists{panic("legacy context deletion did not reproduce")};fmt.Println("legacy singleton context",context,"input after value-phase delete",raw(tuple))
 for _,c:=range []struct{v []interface{};want string}{
  {[]interface{}{1,[]interface{}{2,3}},`{"a":[1,[2,3]]}`},
  {[]interface{}{[]interface{}{1,2},[]interface{}{3,4}},`{"a":[1,2,[3,4]]}`},
  {[]interface{}{[]interface{}{},[]interface{}{}},`{"a":[[]]}`},
  {[]interface{}{1,2,3},`{"a":[1,2,3]}`},
 } {stream:=[]interface{}{};for _,v:=range c.v{stream=append(stream,map[string]interface{}{"a":v})};out:=reduceTupleStream(stream);same(out,c.want);fmt.Println("unchanged nesting",raw(out))}
 out=reduceTupleStream([]interface{}{map[string]interface{}{"a":1},map[string]interface{}{"b":2},map[string]interface{}{"a":3}})
 same(out,`{"a":[1,3],"b":2}`);fmt.Println("unchanged missing fields",raw(out))
}
