package scripts_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRootModuleGateChecksSelectionAndCleansPublicEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the root dependency gate is a POSIX shell script")
	}
	root := repoRoot(t)
	fakeBin := t.TempDir()
	script := `#!/bin/sh
set -eu
printf '%s|%s|%s|%s|%s|%s|%s|%s|%s|%s\n' "$*" "${GOFLAGS-}" "${GOPRIVATE-}" "${GONOPROXY-}" "${GONOSUMDB-}" "${GOINSECURE-}" "${GOENV-}" "${GOSUMDB-}" "${GOWORK-}" "${GOPROXY-}" >>"$FAKE_GO_LOG"
case "$*" in
  'list -m -f '*)
    if [ "$FAKE_GO_MODE" = wrong-workspace ]; then
      printf '%s\n' 'false|/foreign/core'
    else
      printf 'true|%s/jsonataddl\n' "$PWD"
    fi ;;
  *'{{.Version}}'*)
    if [ "$FAKE_GO_MODE" = wrong-public-tuple ]; then
      printf '%s\n' 'v0.1.2|wrong|wrong||'
    else
      printf '%s\n' 'v0.2.0|h1:rD0TyYRPHT+DapFEacbd+jLQKHo6I+mi2ufpcCd+eKY=|h1:fpGrE/1ODSULhyxThhr3TL4JtpfcX3PdA4kJBnKWJRY=||'
    fi ;;
  *' all')
    case "$FAKE_GO_MODE" in
      list-failure) echo 'module enumeration failed' >&2; exit 23 ;;
      other-replacement) printf '\ngolang.org/x/time\n\n' ;;
      *) printf '\n\n\n' ;;
    esac ;;
  'mod verify')
    if [ "$FAKE_GO_MODE" = verification-failure ]; then
      echo 'module verification failed' >&2; exit 24
    fi ;;
  'test -mod=readonly ./...')
    if [ "$FAKE_GO_MODE" = tests-failure ]; then
      echo 'root tests failed' >&2; exit 25
    fi ;;
  *) echo "unexpected go call: $*" >&2; exit 90 ;;
esac
`
	if err := os.WriteFile(filepath.Join(fakeBin, "go"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		mode string
		want string
	}{
		{mode: "passing"},
		{mode: "wrong-workspace", want: "paired development must use this checkout"},
		{mode: "wrong-public-tuple", want: "root module must select the exact public"},
		{mode: "other-replacement", want: "dependencies must not contain replacements"},
		{mode: "list-failure", want: "module enumeration failed"},
		{mode: "verification-failure", want: "module verification failed"},
		{mode: "tests-failure", want: "root tests failed"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "go.log")
			env := append(fakeGoEnvironment(fakeBin, logPath, test.mode), "GOENV=/untrusted/go.env")
			output, err := runFailure(t, root, env, "sh", "scripts/check-root-module.sh")
			if test.want != "" {
				if err == nil || !strings.Contains(string(output), test.want) {
					t.Fatalf("gate refusal = %v\n%s", err, output)
				}
				return
			}
			if err != nil || !strings.Contains(string(output), "verified paired development and public root-module dependency") {
				t.Fatalf("gate outcome = %v\n%s", err, output)
			}
			calls, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
			if len(lines) != 5 || !strings.Contains(lines[0], "|"+filepath.Join(root, "go.work")+"|") {
				t.Fatalf("gate did not check paired selection and every public stage:\n%s", calls)
			}
			for _, line := range lines[1:] {
				if !strings.HasSuffix(line, "||||||off|sum.golang.org|off|https://proxy.golang.org") {
					t.Fatalf("public Go subprocess inherited a forbidden setting: %q", line)
				}
			}
		})
	}
}
