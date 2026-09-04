package scripts_test

import (
	"os"
	"os/exec"
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
			output, err := runFailure(t, root, env, "sh", "scripts/verify-jsonataddl-module.sh", "v0.1.1", "file:///module-proxy")
			if test.wantErr {
				if err == nil || !strings.Contains(string(output), "did not run and pass ExampleLoadApplication") {
					t.Fatalf("missing example outcome = %v\n%s", err, output)
				}
				return
			}
			if err != nil || !strings.Contains(string(output), "verified "+jsonataddlModule+" v0.1.1") {
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

func TestCheckJSONataDDLVersionAvailableFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the pre-tag checker is a POSIX shell script")
	}

	root := repoRoot(t)
	fakeBin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	curlScript := `#!/bin/sh
set -eu
printf '%s\n' "$*" >>"$FAKE_CURL_LOG"
output=/dev/null
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output) output=$2; shift 2 ;;
    https://*) url=$1; shift ;;
    *) shift ;;
  esac
done
case "$url" in
  *api.github.com*) status=${FAKE_REMOTE_STATUS-404}; body= ;;
  *proxy.golang.org*) status=${FAKE_PROXY_STATUS-200}; body=${FAKE_PROXY_BODY-} ;;
  *sum.golang.org*) status=${FAKE_SUMDB_STATUS-404}; body= ;;
  *) echo 'unexpected endpoint' >&2; exit 90 ;;
esac
if [ "$status" = transport-error ]; then
  exit 7
fi
[ "$output" = /dev/null ] || printf '%s' "$body" >"$output"
printf '%s' "$status"
`
	if err := os.WriteFile(filepath.Join(fakeBin, "curl"), []byte(curlScript), 0o700); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name         string
		remoteStatus string
		proxyStatus  string
		proxyBody    string
		sumdbStatus  string
		wantErr      bool
		wantOutput   string
		wantCurl     int
	}{
		{name: "verified exact absence", remoteStatus: "404", proxyStatus: "200", proxyBody: "v0.1.0\nv0.1.10\n", sumdbStatus: "404", wantOutput: "safe to tag", wantCurl: 3},
		{name: "proxy has no module list", remoteStatus: "404", proxyStatus: "404", sumdbStatus: "404", wantOutput: "safe to tag", wantCurl: 3},
		{name: "remote tag", remoteStatus: "200", wantErr: true, wantOutput: "tag already exists on origin", wantCurl: 1},
		{name: "remote transport failure", remoteStatus: "transport-error", wantErr: true, wantOutput: "request failed", wantCurl: 1},
		{name: "remote unexpected response", remoteStatus: "503", wantErr: true, wantOutput: "HTTP 503", wantCurl: 1},
		{name: "remote malformed status", remoteStatus: "not-a-status", wantErr: true, wantOutput: "malformed HTTP status", wantCurl: 1},
		{name: "proxy record", remoteStatus: "404", proxyStatus: "200", proxyBody: "v0.1.1\n", sumdbStatus: "404", wantErr: true, wantOutput: "already listed by proxy.golang.org", wantCurl: 2},
		{name: "checksum record", remoteStatus: "404", proxyStatus: "200", proxyBody: "v0.1.0\n", sumdbStatus: "200", wantErr: true, wantOutput: "already recorded by sum.golang.org", wantCurl: 3},
		{name: "proxy transport failure", remoteStatus: "404", proxyStatus: "transport-error", sumdbStatus: "404", wantErr: true, wantOutput: "request failed", wantCurl: 2},
		{name: "proxy unexpected response", remoteStatus: "404", proxyStatus: "503", sumdbStatus: "404", wantErr: true, wantOutput: "HTTP 503", wantCurl: 2},
		{name: "proxy gone is not absence", remoteStatus: "404", proxyStatus: "410", sumdbStatus: "404", wantErr: true, wantOutput: "HTTP 410", wantCurl: 2},
		{name: "proxy malformed status", remoteStatus: "404", proxyStatus: "not-a-status", sumdbStatus: "404", wantErr: true, wantOutput: "malformed HTTP status", wantCurl: 2},
		{name: "proxy malformed version", remoteStatus: "404", proxyStatus: "200", proxyBody: "v0.1.0 garbage\n", sumdbStatus: "404", wantErr: true, wantOutput: "malformed version", wantCurl: 2},
		{name: "proxy duplicate version", remoteStatus: "404", proxyStatus: "200", proxyBody: "v0.1.0\nv0.1.0\n", sumdbStatus: "404", wantErr: true, wantOutput: "duplicate version", wantCurl: 2},
		{name: "proxy ambiguous empty line", remoteStatus: "404", proxyStatus: "200", proxyBody: "v0.1.0\n\nv0.1.2\n", sumdbStatus: "404", wantErr: true, wantOutput: "malformed empty version", wantCurl: 2},
		{name: "sumdb transport failure", remoteStatus: "404", proxyStatus: "200", proxyBody: "v0.1.0\n", sumdbStatus: "transport-error", wantErr: true, wantOutput: "request failed", wantCurl: 3},
		{name: "sumdb unexpected response", remoteStatus: "404", proxyStatus: "200", proxyBody: "v0.1.0\n", sumdbStatus: "503", wantErr: true, wantOutput: "HTTP 503", wantCurl: 3},
		{name: "sumdb malformed status", remoteStatus: "404", proxyStatus: "200", proxyBody: "v0.1.0\n", sumdbStatus: "not-a-status", wantErr: true, wantOutput: "malformed HTTP status", wantCurl: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			logDir := t.TempDir()
			curlLog := filepath.Join(logDir, "curl.log")
			env := append(os.Environ(),
				"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"FAKE_CURL_LOG="+curlLog,
				"FAKE_REMOTE_STATUS="+test.remoteStatus,
				"FAKE_PROXY_STATUS="+test.proxyStatus,
				"FAKE_PROXY_BODY="+test.proxyBody,
				"FAKE_SUMDB_STATUS="+test.sumdbStatus,
			)
			output, err := runFailure(t, root, env, "sh", "scripts/check-jsonataddl-version-available.sh", "v0.1.1")
			if test.wantErr == (err == nil) || !strings.Contains(string(output), test.wantOutput) {
				t.Fatalf("pre-tag outcome = %v\n%s", err, output)
			}
			calls, readErr := os.ReadFile(curlLog)
			if os.IsNotExist(readErr) && test.wantCurl == 0 {
				return
			}
			if readErr != nil {
				t.Fatal(readErr)
			}
			lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
			if len(lines) != test.wantCurl {
				t.Fatalf("curl calls = %d, want %d:\n%s", len(lines), test.wantCurl, calls)
			}
			for _, line := range lines {
				if !strings.Contains(line, "--proto =https --proto-redir =https") || !strings.Contains(line, "--connect-timeout 5 --max-time 15") || !strings.Contains(line, "--write-out %{http_code}") {
					t.Fatalf("curl call lacks HTTPS-only redirects or fixed bounds: %q", line)
				}
				if strings.Contains(line, "/@v/v0.1.1.info") {
					t.Fatalf("absence check poisoned the per-version proxy cache: %q", line)
				}
			}
			if test.wantCurl > 0 && !strings.Contains(lines[0], "api.github.com/repos/generalbusiness-ai/tailapps/git/ref/tags/jsonataddl/v0.1.1") {
				t.Fatalf("first check did not use the exact remote-tag surface: %q", lines[0])
			}
			if test.wantCurl > 1 && !strings.Contains(lines[1], "/@v/list") {
				t.Fatalf("second check did not use the version-list surface: %q", lines[1])
			}
		})
	}
}

func TestCheckJSONataDDLVersionAvailableRequiresEveryTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the pre-tag checker is a POSIX shell script")
	}

	root := repoRoot(t)
	tools := []string{"curl", "find", "grep", "mktemp"}
	for _, missing := range tools {
		t.Run(missing, func(t *testing.T) {
			path := t.TempDir()
			for _, tool := range tools {
				if tool == missing {
					continue
				}
				target, err := exec.LookPath(tool)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(path, tool)); err != nil {
					t.Fatal(err)
				}
			}
			env := append(os.Environ(), "PATH="+path)
			output, err := runFailure(t, root, env, "sh", "scripts/check-jsonataddl-version-available.sh", "v0.1.1")
			if err == nil || !strings.Contains(string(output), missing+" is required") {
				t.Fatalf("missing %s outcome = %v\n%s", missing, err, output)
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
  example-pass:list\ -m\ -f*) printf '%s\n' 'github.com/generalbusiness-ai/tailapps/jsonataddl v0.1.1' ;;
  example-missing:list\ -m\ -f*) printf '%s\n' 'github.com/generalbusiness-ai/tailapps/jsonataddl v0.1.1' ;;
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
