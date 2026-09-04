#!/bin/sh
# Refuse dependencies on Tailapps packages outside the jsonataddl module.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
module_dir=${1:-"$repo_root/jsonataddl"}
module=github.com/generalbusiness-ai/tailapps/jsonataddl
tailapps_prefix=github.com/generalbusiness-ai/tailapps/

dependencies=$(cd "$module_dir" && GOWORK=off go list -deps -test ./...) || {
  echo 'could not enumerate jsonataddl production and test dependencies' >&2
  exit 1
}

violations=$(printf '%s\n' "$dependencies" | awk -v module="$module" -v prefix="$tailapps_prefix" '
  index($1, prefix) != 1 { next }
  $1 == module || $1 == module ".test" || $1 == module "_test" { next }
  index($1, module "/") == 1 { next }
  { print }
')

[ -z "$violations" ] || {
  echo 'jsonataddl tests and production code must not depend on Tailapps packages outside jsonataddl' >&2
  printf '%s\n' "$violations" >&2
  exit 1
}
