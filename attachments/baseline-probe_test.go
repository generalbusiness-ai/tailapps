package jsonataddl
import("testing"; jsonata "github.com/jsonata-go/jsonata/v206")
func TestCallableDecisionBaseline(t *testing.T){
 for _,source:=range []string{`($sum := $length; $sum("abc"))`,`($sum := $substring(?,1); $sum("abc"))`,`($reverse := 3; $exists($reverse))`,`{"reverse":3}.reverse`,`($sum := $length; ($sum := $abs; $sum(-2)); $sum("abc"))`,`$exists([$length])`,`$exists([$reverse])`,`($sum := $map; $sum(["aa","bbb"], $length))`,`($sum := $reverse; $sum([1,2]))`,`$exists($reverse)`,`$reverse([1,2])`}{
  ambient:=ambientJSONataRE.MatchString(source);lex:=validateJSONataLexicalSource([]byte(source));e,err:=jsonata.Compile(source,false);if err!=nil {t.Fatal(err)};ast:=validateJSONataAST(e.AST());out,eval:=e.Evaluate([]byte(`{}`),nil);t.Logf("source=%s ambient=%v lexical=%v AST=%v output=%s eval=%v",source,ambient,lex,ast,out,eval)
 }
}
