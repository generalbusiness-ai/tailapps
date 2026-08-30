package examples

import (
	"os"
	"testing"
	"testing/fstest"

	"github.com/generalbusiness-ai/tailapps/internal/profile"
)

func TestSignalCountsCompiles(t *testing.T) {
	files := fstest.MapFS{}
	for _, name := range []string{"application.sql", "folds/normalize.jsonata", "folds/count.jsonata"} {
		content, err := os.ReadFile("signal-counts/" + name)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = &fstest.MapFile{Data: content}
	}
	compiled, err := profile.Load(files, ".", "signal-counts")
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Exports["signal_counts"].Name != "signal_counts" {
		t.Fatalf("compiled profile = %#v", compiled)
	}
}
