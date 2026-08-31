package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
