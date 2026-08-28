package cli

import (
	"bytes"
	"context"
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
