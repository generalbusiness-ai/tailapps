#!/bin/sh
# Check both paired checkout development and the root's public dependency.
set -eu
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"
module=github.com/generalbusiness-ai/tailapps/jsonataddl

local=$(GOWORK="$repo_root/go.work" go list -m -f '{{.Main}}|{{.Dir}}' "$module")
[ "$local" = "true|$repo_root/jsonataddl" ] || {
  echo 'paired development must use this checkout jsonataddl module' >&2
  exit 1
}

# Ignore ambient workspace, flags and private proxy/sumdb bypasses. Cache
# isolation for an independent release check is supplied by its caller.
unset GOFLAGS GOPRIVATE GONOPROXY GONOSUMDB GOINSECURE
export GOENV=off GOWORK=off GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org
public=$(go list -mod=readonly -m -f '{{.Version}}|{{.Sum}}|{{.GoModSum}}|{{if .Replace}}replacement{{end}}|{{if .Main}}main{{end}}' "$module")
[ "$public" = 'v0.2.0|h1:rD0TyYRPHT+DapFEacbd+jLQKHo6I+mi2ufpcCd+eKY=|h1:fpGrE/1ODSULhyxThhr3TL4JtpfcX3PdA4kJBnKWJRY=||' ] || {
  echo 'root module must select the exact public jsonataddl v0.2.0 and reviewed checksums without replacement' >&2
  exit 1
}
replacements=$(go list -mod=readonly -m -f '{{if .Replace}}{{.Path}}{{end}}' all)
if printf '%s' "$replacements" | grep -q '[^[:space:]]'; then
  echo 'public root-module dependencies must not contain replacements' >&2
  exit 1
fi
go mod verify
go test -mod=readonly ./...
printf '%s\n' 'verified paired development and public root-module dependency'
