package jsonataddl
import("testing"; jsonata "github.com/jsonata-go/jsonata/v206")
func TestI7GroupOrderProbe(t *testing.T) {
 source := `($x := 0; {"a": $x := 1, "b": $x})`
 if ambientJSONataRE.MatchString(source) { t.Fatal("ambient refusal") }; if err:=validateJSONataLexicalSource([]byte(source));err!=nil {t.Fatal(err)}
 e,err:=jsonata.Compile(source,false);if err!=nil {t.Fatal(err)};if err:=validateJSONataAST(e.AST());err!=nil {t.Fatal(err)}
 seen:=map[string]int{};for i:=0;i<1000;i++ {out,err:=e.Evaluate([]byte(`{}`),nil);if err!=nil {t.Fatal(err)};seen[string(out)]++};t.Logf("source=%s accepted=true outputs=%v",source,seen)
 if len(seen)<2 {t.Fatal("probe did not reproduce divergent outputs")}
}
