package tailapps

import "testing"

func TestPlannerURLUserinfoStar(t *testing.T) {
 compiled := loadURLReputation(t)
 base := urlReputationFixtures(t)["observed"]
 for _, rawURL := range []string{"https://u*ser:pw@example.com/path", "https://u%2Aser:pw@example.com/path", "https://user:pw@example.com/path"} {
  t.Run(rawURL,func(t *testing.T){
   input := cloneURLInput(t, base)
   setURLAttributes(input, map[string]any{"tailapp.url.observed_full": rawURL,"tailapp.url.host":"example.com"})
   result := normalizeURLFixture(t, compiled, input)
   t.Logf("URL=%s decision=%s events=%d",rawURL,result.Decision,len(result.Events["otel_event"]))
   if result.Decision != "effective" || len(result.Events["otel_event"]) != 1 { t.Errorf("valid userinfo URL refused: %#v",result) }
  })
 }
}
