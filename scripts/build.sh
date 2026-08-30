#!/bin/sh
# Release build: stamps the sanitized GitHub origin into the binary's build
# provenance so serverInfo.websiteUrl reflects the real remote available at
# build time. A repository without a GitHub origin builds unstamped and the
# binary falls back to module-path derivation (see internal/buildinfo).
#
# Usage: scripts/build.sh [OUTPUT]   (default OUTPUT: ./tailapp)
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
output="${1:-$repo_root/tailapp}"

remote=$(git -C "$repo_root" remote get-url origin 2>/dev/null || true)
stamp=$("$repo_root/scripts/sanitize-remote.sh" "$remote" 2>/dev/null || true)

flags=""
if [ -n "$stamp" ]; then
  flags="-X github.com/generalbusiness-ai/tailapps/internal/buildinfo.stampedSourceURL=$stamp"
fi

cd "$repo_root"
go build -ldflags "$flags" -o "$output" ./cmd/tailapp
