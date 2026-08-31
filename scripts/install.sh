#!/bin/sh
# This template is rendered into the version-pinned install.sh release asset by
# scripts/release.sh. Do not publish this checkout copy directly: it refuses
# to run until @TAILAPPS_VERSION@ has been replaced by the immutable release
# version.
set -eu

tailapps_version='@TAILAPPS_VERSION@'
template_marker='@TAILAPPS_'VERSION'@'
case "$tailapps_version" in
  "$template_marker"|'')
    echo 'install.sh must be downloaded from a Tailapps release asset' >&2
    exit 64
    ;;
esac

release_root=${TAILAPPS_RELEASE_BASE_URL:-https://github.com/generalbusiness-ai/tailapps/releases}
install_root=${TAILAPPS_INSTALL_ROOT:-"$HOME/.local"}
release_url="$release_root/download/v$tailapps_version"

case "$install_root" in
  /*) ;;
  *)
    echo 'TAILAPPS_INSTALL_ROOT must be an absolute path' >&2
    exit 64
    ;;
esac

case "$(uname -s)" in
  Darwin) release_os=darwin ;;
  Linux) release_os=linux ;;
  *)
    echo "unsupported operating system: $(uname -s)" >&2
    exit 65
    ;;
esac
case "$(uname -m)" in
  arm64|aarch64) release_arch=arm64 ;;
  x86_64|amd64) release_arch=amd64 ;;
  *)
    echo "unsupported CPU architecture: $(uname -m)" >&2
    exit 65
    ;;
esac

command -v curl >/dev/null 2>&1 || { echo 'curl is required' >&2; exit 69; }
command -v tar >/dev/null 2>&1 || { echo 'tar is required' >&2; exit 69; }
if command -v sha256sum >/dev/null 2>&1; then
  sha256_file() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  sha256_file() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  echo 'sha256sum or shasum is required' >&2
  exit 69
fi

download() {
  curl --fail --silent --show-error --location "$1" --output "$2"
}

bin_dir="$install_root/bin"
lib_dir="$install_root/lib/tailapp"
bin_link="$bin_dir/tailapp"
archive_name="tailapps_${tailapps_version}_${release_os}_${release_arch}.tar.gz"
archive_url="$release_url/$archive_name"
mkdir -p "$bin_dir" "$lib_dir"

checksums=$(mktemp "$lib_dir/.tailapps-checksums.XXXXXX")
archive=$(mktemp "$lib_dir/.tailapps-archive.XXXXXX")
signature=$(mktemp "$lib_dir/.tailapps-signature.XXXXXX")
bundle=$(mktemp "$lib_dir/.tailapps-bundle.XXXXXX")
download "$release_url/checksums.txt" "$checksums"
download "$release_url/checksums.txt.sig" "$signature"
download "$release_url/checksums.txt.bundle" "$bundle"
download "$archive_url" "$archive"

expected=$(awk -v name="$archive_name" '$2 == name {print $1; exit}' "$checksums")
printf '%s\n' "$expected" | grep -Eq '^[0-9a-fA-F]{64}$' || {
  echo "checksums.txt has no SHA-256 entry for $archive_name" >&2
  exit 65
}
actual=$(sha256_file "$archive")
[ "$actual" = "$expected" ] || { echo "checksum mismatch for $archive_name" >&2; exit 65; }

if command -v cosign >/dev/null 2>&1; then
  cosign verify-blob \
    --bundle "$bundle" \
    --certificate-identity-regexp "^https://github.com/generalbusiness-ai/tailapps/.github/workflows/release.yml@refs/tags/v$tailapps_version$" \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    "$checksums"
else
  echo 'cosign is not installed; verified the archive checksum but not the keyless checksum signature' >&2
fi

installed_binary="$lib_dir/tailapp-$tailapps_version"
[ ! -e "$installed_binary" ] || { echo "refusing to overwrite $installed_binary" >&2; exit 65; }
candidate=$(mktemp "$lib_dir/.tailapps-binary.XXXXXX")
tar -xOzf "$archive" tailapp >"$candidate"
chmod 755 "$candidate"
"$candidate" version >/dev/null
mv "$candidate" "$installed_binary"

if [ -e "$bin_link" ] && [ ! -L "$bin_link" ]; then
  echo "refusing to replace non-symlink $bin_link" >&2
  exit 65
fi
link_stage=$(mktemp -d "$bin_dir/.tailapps-link.XXXXXX")
rmdir "$link_stage"
ln -s "$installed_binary" "$link_stage"
mv "$link_stage" "$bin_link"

unlink "$checksums"
unlink "$archive"
unlink "$signature"
unlink "$bundle"
printf '%s\n' "installed Tailapps $tailapps_version at $installed_binary"
printf '%s\n' "next: $bin_link serve"
