#!/bin/sh
# Refuse a jsonataddl tag whose version already exists remotely or publicly.
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

for tool in curl find grep mktemp; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "$tool is required to verify module-version absence" >&2
    exit 69
  }
done
printf '%s\n' "${version#v}" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' || {
  echo "version must be semantic vMAJOR.MINOR.PATCH" >&2
  exit 64
}

module=github.com/generalbusiness-ai/tailapps/jsonataddl
tag_ref="refs/tags/jsonataddl/$version"
remote_url="https://api.github.com/repos/generalbusiness-ai/tailapps/git/ref/tags/jsonataddl/$version"
proxy_url="https://proxy.golang.org/$module/@v/list"
sumdb_url="https://sum.golang.org/lookup/$module@$version"

remote_status=$(
  curl --silent --show-error --location \
    --proto '=https' --proto-redir '=https' \
    --connect-timeout 5 --max-time 15 \
    --header 'Accept: application/vnd.github+json' \
    --header 'X-GitHub-Api-Version: 2022-11-28' \
    --output /dev/null --write-out '%{http_code}' \
    "$remote_url"
) || {
  echo "could not verify absence from origin: request failed" >&2
  exit 69
}
case "$remote_status" in
  404) printf 'verified absent from origin: %s\n' "$tag_ref" ;;
  200) echo "refusing $version: tag already exists on origin" >&2; exit 1 ;;
  [0-9][0-9][0-9]) echo "could not verify absence from origin: HTTP $remote_status" >&2; exit 69 ;;
  *) echo "could not verify absence from origin: malformed HTTP status '$remote_status'" >&2; exit 65 ;;
esac

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/jsonataddl-version-check.XXXXXX") || {
  echo "could not create temporary directory for public checks" >&2
  exit 69
}
cleanup() {
  find "$tmp_dir" -depth -delete
}
trap cleanup 0

proxy_body="$tmp_dir/proxy-list"
proxy_status=$(
  curl --silent --show-error --location \
    --proto '=https' --proto-redir '=https' \
    --connect-timeout 5 --max-time 15 \
    --output "$proxy_body" --write-out '%{http_code}' \
    "$proxy_url"
) || {
  echo "could not verify absence from proxy.golang.org: request failed" >&2
  exit 69
}
case "$proxy_status" in
  200)
    seen='|'
    while IFS= read -r listed || [ -n "$listed" ]; do
      [ -n "$listed" ] || {
        echo "could not verify absence from proxy.golang.org: malformed empty version" >&2
        exit 65
      }
      printf '%s\n' "$listed" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' || {
        echo "could not verify absence from proxy.golang.org: malformed version '$listed'" >&2
        exit 65
      }
      case "$seen" in
        *"|$listed|"*)
          echo "could not verify absence from proxy.golang.org: duplicate version '$listed'" >&2
          exit 65
          ;;
      esac
      seen="$seen$listed|"
      if [ "$listed" = "$version" ]; then
        echo "refusing $version: already listed by proxy.golang.org" >&2
        exit 1
      fi
    done <"$proxy_body"
    printf 'verified absent from proxy.golang.org version list: %s\n' "$version"
    ;;
  404)
    printf 'verified proxy.golang.org has no version list: %s\n' "$version"
    ;;
  [0-9][0-9][0-9])
    echo "could not verify absence from proxy.golang.org: HTTP $proxy_status" >&2
    exit 69
    ;;
  *)
    echo "could not verify absence from proxy.golang.org: malformed HTTP status '$proxy_status'" >&2
    exit 65
    ;;
esac

check_sumdb_absent() {
  status=$(
    curl --silent --show-error --location \
      --proto '=https' --proto-redir '=https' \
      --connect-timeout 5 --max-time 15 \
      --output /dev/null --write-out '%{http_code}' \
      "$sumdb_url"
  ) || {
    echo "could not verify absence from sum.golang.org: request failed" >&2
    exit 69
  }

  case "$status" in
    404)
      printf 'verified absent from sum.golang.org: %s\n' "$version"
      ;;
    200)
      echo "refusing $version: already recorded by sum.golang.org" >&2
      exit 1
      ;;
    [0-9][0-9][0-9])
      echo "could not verify absence from sum.golang.org: HTTP $status" >&2
      exit 69
      ;;
    *)
      echo "could not verify absence from sum.golang.org: malformed HTTP status '$status'" >&2
      exit 65
      ;;
  esac
}

check_sumdb_absent
printf 'safe to tag: %s %s has no remote tag, proxy-list entry, or checksum-database record\n' "$module" "$version"
