package scripts_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const jsonataddlModule = "github.com/generalbusiness-ai/tailapps/jsonataddl"

func TestVerifyJSONataDDLModuleRequiresPassingExampleAndCleansEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the published verifier is a POSIX shell script")
	}

	root := repoRoot(t)
	fakeBin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFakeGo(t, fakeBin)

	for _, test := range []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{name: "passing example", mode: "example-pass"},
		{name: "missing or renamed example", mode: "example-missing", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "go.log")
			env := fakeGoEnvironment(fakeBin, logPath, test.mode)
			output, err := runFailure(t, root, env, "sh", "scripts/verify-jsonataddl-module.sh", "v0.1.0", "file:///module-proxy")
			if test.wantErr {
				if err == nil || !strings.Contains(string(output), "did not run and pass ExampleLoadApplication") {
					t.Fatalf("missing example outcome = %v\n%s", err, output)
				}
				return
			}
			if err != nil || !strings.Contains(string(output), "verified "+jsonataddlModule+" v0.1.0") {
				t.Fatalf("passing example outcome = %v\n%s", err, output)
			}
			calls, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
				if !strings.HasSuffix(line, "||||||sum.golang.org|off|file:///module-proxy") {
					t.Fatalf("Go subprocess inherited a forbidden setting: %q", line)
				}
			}
		})
	}
}

func TestCheckJSONataDDLBoundaryFailsClosedAndAdmitsOnlyTheModule(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the boundary checker is a POSIX shell script")
	}

	root := repoRoot(t)
	fakeBin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFakeGo(t, fakeBin)

	for _, test := range []struct {
		name       string
		mode       string
		wantErr    bool
		wantOutput string
	}{
		{name: "exact module and descendants", mode: "boundary-allowed"},
		{name: "go list failure", mode: "boundary-list-failure", wantErr: true, wantOutput: "could not enumerate"},
		{name: "outside Tailapps dependency", mode: "boundary-disallowed", wantErr: true, wantOutput: "internal/profile"},
		{name: "root Tailapps dependency", mode: "boundary-root", wantErr: true, wantOutput: jsonataddlModule[:strings.LastIndex(jsonataddlModule, "/")]},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := fakeGoEnvironment(fakeBin, filepath.Join(t.TempDir(), "go.log"), test.mode)
			output, err := runFailure(t, root, env, "sh", "scripts/check-jsonataddl-boundary.sh")
			if test.wantErr {
				if err == nil || !strings.Contains(string(output), test.wantOutput) {
					t.Fatalf("boundary outcome = %v\n%s", err, output)
				}
				return
			}
			if err != nil {
				t.Fatalf("allowed boundary outcome = %v\n%s", err, output)
			}
		})
	}
}

func fakeGoEnvironment(fakeBin, logPath, mode string) []string {
	return append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GO_LOG="+logPath,
		"FAKE_GO_MODE="+mode,
		"GOFLAGS=-mod=vendor",
		"GOPRIVATE=example.invalid",
		"GONOPROXY=example.invalid",
		"GONOSUMDB=example.invalid",
		"GOINSECURE=example.invalid",
		"GOSUMDB=off",
		"GOWORK=/untrusted/go.work",
		"GOPROXY=file:///untrusted-proxy",
	)
}

func writeFakeGo(t *testing.T, directory string) {
	t.Helper()
	script := `#!/bin/sh
set -eu
printf '%s|%s|%s|%s|%s|%s|%s|%s|%s\n' "$*" "${GOFLAGS-}" "${GOPRIVATE-}" "${GONOPROXY-}" "${GONOSUMDB-}" "${GOINSECURE-}" "${GOSUMDB-}" "${GOWORK-}" "${GOPROXY-}" >>"$FAKE_GO_LOG"
case "$FAKE_GO_MODE:$*" in
  boundary-list-failure:list\ -deps\ -test\ ./...) exit 23 ;;
  boundary-allowed:list\ -deps\ -test\ ./...)
    printf '%s\n' \
      'github.com/generalbusiness-ai/tailapps/jsonataddl' \
      'github.com/generalbusiness-ai/tailapps/jsonataddl [github.com/generalbusiness-ai/tailapps/jsonataddl.test]' \
      'github.com/generalbusiness-ai/tailapps/jsonataddl_test [github.com/generalbusiness-ai/tailapps/jsonataddl.test]' \
      'github.com/generalbusiness-ai/tailapps/jsonataddl.test' \
      'github.com/generalbusiness-ai/tailapps/jsonataddl/corpus'
    ;;
  boundary-disallowed:list\ -deps\ -test\ ./...)
    printf '%s\n' \
      'github.com/generalbusiness-ai/tailapps/jsonataddl' \
      'github.com/generalbusiness-ai/tailapps/internal/profile'
    ;;
  boundary-root:list\ -deps\ -test\ ./...)
    printf '%s\n' \
      'github.com/generalbusiness-ai/tailapps/jsonataddl' \
      'github.com/generalbusiness-ai/tailapps'
    ;;
  example-pass:list\ -m\ -f*) printf '%s\n' 'github.com/generalbusiness-ai/tailapps/jsonataddl v0.1.0' ;;
  example-missing:list\ -m\ -f*) printf '%s\n' 'github.com/generalbusiness-ai/tailapps/jsonataddl v0.1.0' ;;
  example-pass:test*)
    printf '%s\n' '=== RUN   ExampleLoadApplication' '--- PASS: ExampleLoadApplication (0.00s)' 'PASS' 'ok  github.com/generalbusiness-ai/tailapps/jsonataddl 0.001s'
    ;;
  example-missing:test*)
    printf '%s\n' 'testing: warning: no tests to run' 'PASS' 'ok  github.com/generalbusiness-ai/tailapps/jsonataddl 0.001s [no tests to run]'
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(directory, "go"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}
