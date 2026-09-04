#!/bin/sh
# Verify a published jsonataddl version from a clean external consumer.
set -eu

module=github.com/generalbusiness-ai/tailapps/jsonataddl
version=${1:-}
proxy=${2:-https://proxy.golang.org}

case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo 'usage: verify-jsonataddl-module.sh vMAJOR.MINOR.PATCH [GOPROXY]' >&2; exit 64 ;;
esac
printf '%s\n' "${version#v}" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' || {
  echo 'version must be semantic vMAJOR.MINOR.PATCH' >&2
  exit 64
}

verify_tmp=$(mktemp -d "${TMPDIR:-/tmp}/jsonataddl-consumer.XXXXXX")
cleanup() {
  case "$verify_tmp" in
    "${TMPDIR:-/tmp}"/jsonataddl-consumer.*) find "$verify_tmp" -depth -delete ;;
  esac
}
trap cleanup EXIT HUP INT TERM

cd "$verify_tmp"
go_clean() {
  env GOFLAGS= GOPRIVATE= GONOPROXY= GONOSUMDB= GOSUMDB=sum.golang.org GOWORK=off GOPROXY="$proxy" go "$@"
}

go_clean mod init example.invalid/jsonataddl-consumer >/dev/null
go_clean get "$module@$version"

resolved=$(go_clean list -m -f '{{.Path}} {{.Version}}{{if .Replace}} replaced{{end}}' "$module")
if [ "$resolved" != "$module $version" ]; then
  echo "unexpected module resolution: $resolved" >&2
  exit 1
fi

# The dependency's packaged example is the public compile-and-evaluate path.
# Running it here proves a clean consumer can resolve, compile, and execute it
# without maintaining a second copy of the example program.
go_clean test -count=1 -run '^ExampleLoadApplication$' "$module"
printf 'verified %s %s through %s\n' "$module" "$version" "$proxy"
