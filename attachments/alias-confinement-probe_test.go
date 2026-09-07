package jsonataddl
import("testing"; jsonata "github.com/jsonata-go/jsonata/v206")
func TestI7AliasConfinementProbe(t *testing.T){
 for _,source:=range []string{`$reverse([1,2])`,`($sum := $reverse; $sum([1,2]))`,`$exists($reverse)`,`($sum := $map; $sum(["aa","bbb"], $length))`}{
  ambient:=ambientJSONataRE.MatchString(source);lex:=validateJSONataLexicalSource([]byte(source));e,err:=jsonata.Compile(source,false);if err!=nil {t.Fatal(err)};ast:=validateJSONataAST(e.AST());out,eval:=e.Evaluate([]byte(`{}`),nil);t.Logf("source=%s ambient=%v lexical=%v AST=%v output=%s eval=%v",source,ambient,lex,ast,out,eval)
 }
}
