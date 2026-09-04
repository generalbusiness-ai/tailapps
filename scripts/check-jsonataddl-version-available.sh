#!/bin/sh
# Refuse a jsonataddl tag whose module version already has a public record.
# Run this before pushing the corresponding jsonataddl/vVERSION tag.
set -eu

[ "$#" = 1 ] || {
  echo "usage: $0 vMAJOR.MINOR.PATCH" >&2
  exit 64
}

version=$1
case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "version must be semantic vMAJOR.MINOR.PATCH" >&2; exit 64 ;;
esac
printf '%s\n' "${version#v}" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' || {
  echo "version must be semantic vMAJOR.MINOR.PATCH" >&2
  exit 64
}

command -v curl >/dev/null 2>&1 || {
  echo "curl is required to verify public module-version absence" >&2
  exit 69
}

module=github.com/generalbusiness-ai/tailapps/jsonataddl
proxy_url="https://proxy.golang.org/$module/@v/$version.info"
sumdb_url="https://sum.golang.org/lookup/$module@$version"

check_absent() {
  service=$1
  url=$2
  status=$(
    curl --silent --show-error --location \
      --proto '=https' --proto-redir '=https' \
      --connect-timeout 5 --max-time 15 \
      --output /dev/null --write-out '%{http_code}' \
      "$url"
  ) || {
    echo "could not verify absence from $service: request failed" >&2
    exit 69
  }

  case "$status" in
    404)
      printf 'verified absent from %s: %s\n' "$service" "$version"
      ;;
    200)
      echo "refusing $version: already recorded by $service" >&2
      exit 1
      ;;
    [0-9][0-9][0-9])
      echo "could not verify absence from $service: HTTP $status" >&2
      exit 69
      ;;
    *)
      echo "could not verify absence from $service: malformed HTTP status '$status'" >&2
      exit 65
      ;;
  esac
}

check_absent proxy.golang.org "$proxy_url"
check_absent sum.golang.org "$sumdb_url"
printf 'safe to tag: %s %s has no public proxy or checksum-database record\n' "$module" "$version"
