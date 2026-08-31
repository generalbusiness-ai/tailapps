package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/generalbusiness-ai/tailapps/internal/control"
	"github.com/generalbusiness-ai/tailapps/internal/engine"
)

func TestHelpLeadsWithProductAndInstallPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"simple, local micro-apps", "apps install", "tailapp serve", "tailapp metrics", "tailapp ineffective"} {
		if !strings.Contains(stdout.String(), wanted) {
			t.Fatalf("help omitted %q:\n%s", wanted, stdout.String())
		}
	}
}

func TestMetricsRejectsDisablingItsOnlyOutputFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"metrics", "--json=false"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "JSON output only") {
		t.Fatalf("metrics --json=false error = %v", err)
	}
}

func TestVersionIsJSONAndDoesNotNeedAResidentHome(t *testing.T) {
	t.Setenv("TAILAPP_HOME", "relative-home-is-irrelevant-for-version")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"version"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &value); err != nil {
		t.Fatalf("version output is not JSON: %v; %s", err, stdout.String())
	}
	for _, key := range []string{"version", "revision", "dirty", "source_url", "go_version", "os", "arch"} {
		if _, found := value[key]; !found {
			t.Fatalf("version JSON omitted %q: %#v", key, value)
		}
	}
}

func TestDefaultHomeMatchesDocumentedPerUserLocation(t *testing.T) {
	if got, want := defaultHome("/Users/example"), "/Users/example/.local/share/tailapp"; got != want {
		t.Fatalf("defaultHome = %q, want %q", got, want)
	}
}

func TestSetupInstallsOnlyMissingRequestedBuiltInsThroughControl(t *testing.T) {
	home, err := os.MkdirTemp("/tmp", "tailapp-cli-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(home); err != nil {
			t.Error(err)
		}
	})
	resident, err := engine.Open(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	defer resident.Close()
	listener, err := control.Listen(filepath.Join(home, "engine.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() { _ = http.Serve(listener, &control.Server{Engine: resident}) }()

	var stdout, stderr bytes.Buffer
	if err := setup(&stdout, &stderr, home, []string{"--bundles", "daily-review,session-cost"}); err != nil {
		t.Fatal(err)
	}
	var first SetupResult
	if err := json.Unmarshal(stdout.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if strings.Join(first.Installed, ",") != "daily-review,session-cost" || len(first.AlreadyPresent) != 0 {
		t.Fatalf("first setup = %#v", first)
	}
	stdout.Reset()
	if err := setup(&stdout, &stderr, home, []string{"--bundles", "session-cost,daily-review"}); err != nil {
		t.Fatal(err)
	}
	var second SetupResult
	if err := json.Unmarshal(stdout.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Installed) != 0 || strings.Join(second.AlreadyPresent, ",") != "session-cost,daily-review" {
		t.Fatalf("second setup overwrote or missed an existing Tailapp: %#v", second)
	}
}

func TestSetupBundleSelectionRejectsUnknownAndDuplicateNames(t *testing.T) {
	for _, value := range []string{"", "unknown", "daily-review,daily-review"} {
		if _, err := setupBundles(value); err == nil {
			t.Fatalf("setupBundles(%q) unexpectedly succeeded", value)
		}
	}
	if selected, err := setupBundles("none"); err != nil || len(selected) != 0 {
		t.Fatalf("setupBundles(none) = %#v, %v", selected, err)
	}
}

func TestReadSourceDirectorySelectsCompleteSourceSet(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "folds"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"application.sql":         "CREATE EVENT otel_event (id TEXT NOT NULL);",
		"folds/normalize.jsonata": "{}",
		"README.md":               "author notes",
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sources, err := readSourceDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || len(sources["application.sql"]) == 0 || len(sources["folds/normalize.jsonata"]) == 0 {
		t.Fatalf("sources = %#v", sources)
	}
	if _, ok := sources["README.md"]; ok {
		t.Fatal("author documentation became executable source")
	}
}
