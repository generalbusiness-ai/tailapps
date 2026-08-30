package profile

import (
	"encoding/json"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The conformance corpus at jsonataddl/corpus freezes the current
// JSONata-with-DDL application semantics as executable cases: compile
// outcomes with pinned identity digests and exact diagnostics, and
// evaluation outcomes with golden logical results. Run with -update-corpus
// to regenerate goldens after a deliberate semantic change; the diff is the
// review surface.
var updateCorpus = flag.Bool("update-corpus", false, "rewrite corpus golden files from current behavior")

const corpusRoot = "../../jsonataddl/corpus/v1"

type corpusManifest struct {
	Interface   string `json:"interface"`
	Dialect     string `json:"dialect"`
	Application string `json:"application"`
	Compile     struct {
		Outcome    string `json:"outcome"`
		Identity   string `json:"identity,omitempty"`
		Diagnostic string `json:"diagnostic,omitempty"`
	} `json:"compile"`
	Evaluations []struct {
		Name          string `json:"name"`
		Program       string `json:"program"`
		Input         string `json:"input"`
		Expect        string `json:"expect,omitempty"`
		ErrorContains string `json:"error_contains,omitempty"`
		Repeat        int    `json:"repeat,omitempty"`
	} `json:"evaluations,omitempty"`
}

type corpusIdentity struct {
	Revision             string `json:"revision"`
	RuntimeProfile       string `json:"runtime_profile"`
	StorageSchemaDigest  string `json:"storage_schema_digest"`
	ExportContractDigest string `json:"export_contract_digest"`
}

// compileFunc is the seam a second implementation plugs into: the future
// extracted core's adapter runs the identical corpus by passing its own
// loader here, giving the migration its differential harness.
type compileFunc func(files fs.FS, root, name string) (*Profile, error)

func TestConformanceCorpus(t *testing.T) {
	runConformanceCorpus(t, Load)
}

func runConformanceCorpus(t *testing.T, compile compileFunc) {
	entries, err := os.ReadDir(corpusRoot)
	if err != nil {
		t.Fatalf("corpus root: %v", err)
	}
	ran := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ran++
		t.Run(entry.Name(), func(t *testing.T) {
			runCorpusCase(t, compile, filepath.Join(corpusRoot, entry.Name()))
		})
	}
	if ran == 0 {
		t.Fatal("corpus contains no cases")
	}
}

func runCorpusCase(t *testing.T, compile compileFunc, caseDir string) {
	manifest := readManifest(t, caseDir)
	appDir := filepath.Join(caseDir, manifest.Application)
	compiled, err := compile(os.DirFS(appDir), ".", "corpus-app")

	switch manifest.Compile.Outcome {
	case "error":
		if err == nil {
			t.Fatalf("compile succeeded; the corpus expects an error")
		}
		compareGoldenText(t, filepath.Join(caseDir, manifest.Compile.Diagnostic), err.Error())
		return
	case "ok":
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
	default:
		t.Fatalf("manifest compile outcome %q is not ok or error", manifest.Compile.Outcome)
	}

	identity := corpusIdentity{
		Revision:             compiled.Revision,
		RuntimeProfile:       compiled.RuntimeProfile,
		StorageSchemaDigest:  compiled.StorageSchemaDigest,
		ExportContractDigest: compiled.ExportContractDigest,
	}
	compareGoldenJSON(t, filepath.Join(caseDir, manifest.Compile.Identity), identity)

	recompiled, err := compile(os.DirFS(appDir), ".", "corpus-app")
	if err != nil {
		t.Fatalf("recompile: %v", err)
	}
	if recompiled.Revision != compiled.Revision || recompiled.StorageSchemaDigest != compiled.StorageSchemaDigest || recompiled.ExportContractDigest != compiled.ExportContractDigest {
		t.Fatalf("recompilation changed identity: %#v vs %#v", recompiled, compiled)
	}

	for _, evaluation := range manifest.Evaluations {
		t.Run(evaluation.Name, func(t *testing.T) {
			if (evaluation.Expect == "") == (evaluation.ErrorContains == "") {
				t.Fatalf("evaluation must declare exactly one of expect or error_contains: %#v", evaluation)
			}
			var input EvaluationInput
			decodeFile(t, filepath.Join(caseDir, evaluation.Input), &input)
			repeats := evaluation.Repeat
			if repeats < 1 {
				repeats = 1
			}
			var first []byte
			for attempt := 0; attempt < repeats; attempt++ {
				result, err := compiled.Evaluate(evaluation.Program, input)
				if evaluation.ErrorContains != "" {
					if err == nil || !strings.Contains(err.Error(), evaluation.ErrorContains) {
						t.Fatalf("error = %v, want containing %q", err, evaluation.ErrorContains)
					}
					return
				}
				if err != nil {
					t.Fatalf("evaluate: %v", err)
				}
				encoded, err := json.MarshalIndent(result, "", " ")
				if err != nil {
					t.Fatal(err)
				}
				if first == nil {
					first = encoded
				} else if string(first) != string(encoded) {
					t.Fatalf("repeated evaluation diverged:\n%s\nvs\n%s", first, encoded)
				}
			}
			compareGoldenText(t, filepath.Join(caseDir, evaluation.Expect), string(first)+"\n")
		})
	}
}

func readManifest(t *testing.T, caseDir string) corpusManifest {
	t.Helper()
	var manifest corpusManifest
	decodeFile(t, filepath.Join(caseDir, "manifest.json"), &manifest)
	if manifest.Application == "" || manifest.Interface == "" || manifest.Dialect == "" {
		t.Fatalf("manifest must declare interface, dialect and application: %#v", manifest)
	}
	return manifest
}

func decodeFile(t *testing.T, path string, target any) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, target); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func compareGoldenJSON(t *testing.T, path string, value any) {
	t.Helper()
	encoded, err := json.MarshalIndent(value, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	compareGoldenText(t, path, string(encoded)+"\n")
}

func compareGoldenText(t *testing.T, path, actual string) {
	t.Helper()
	if *updateCorpus {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(actual), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s missing (run go test -run TestConformanceCorpus -update-corpus and review the diff): %v", path, err)
	}
	if string(expected) != actual {
		t.Fatalf("golden %s differs from current behavior:\n--- golden ---\n%s\n--- current ---\n%s", path, expected, actual)
	}
}
