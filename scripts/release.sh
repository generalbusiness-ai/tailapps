#!/bin/sh
# Build deterministic, versioned Tailapps release archives for the supported
# macOS and Linux targets. Publishing and signing happen in GitHub Actions;
# this script only produces local artifacts and their checksums.
#
# Usage: scripts/release.sh vVERSION OUTPUT_DIRECTORY
set -eu

[ "$#" = 2 ] || {
  echo "usage: $0 vVERSION OUTPUT_DIRECTORY" >&2
  exit 64
}

tag=$1
output_dir=$2
case "$tag" in
  v*) version=${tag#v} ;;
  *)
    echo "release tag must start with v" >&2
    exit 64
    ;;
esac
valid_version='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'
if ! printf '%s\n' "$version" | grep -Eq "$valid_version"; then
  echo "release tag must contain a semantic version" >&2
  exit 64
fi

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$output_dir"

archive_for() {
  goos=$1
  goarch=$2
  archive="$output_dir/tailapps_${version}_${goos}_${goarch}.tar.gz"
  [ ! -e "$archive" ] || {
    echo "refusing to overwrite release archive: $archive" >&2
    exit 65
  }
  stage_dir=$(mktemp -d "$output_dir/.tailapps-${goos}-${goarch}.XXXXXX")
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 TAILAPPS_VERSION="$version" \
    "$repo_root/scripts/build.sh" "$stage_dir/tailapp"
  tar -C "$stage_dir" -czf "$archive" tailapp
  unlink "$stage_dir/tailapp"
  rmdir "$stage_dir"
}

archive_for darwin arm64
archive_for darwin amd64
archive_for linux arm64
archive_for linux amd64

checksums="$output_dir/checksums.txt"
[ ! -e "$checksums" ] || {
  echo "refusing to overwrite checksums: $checksums" >&2
  exit 65
}
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$output_dir" && sha256sum tailapps_"$version"_*.tar.gz) >"$checksums"
else
  (cd "$output_dir" && shasum -a 256 tailapps_"$version"_*.tar.gz) >"$checksums"
fi
