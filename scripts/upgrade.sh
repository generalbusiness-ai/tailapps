#!/bin/sh
# Rendered into the version-pinned upgrade.sh release asset. It consumes the
# signed release installer only after checking that asset against the signed
# checksum manifest, then restarts the existing user service with rollback.
set -eu

tailapps_version='@TAILAPPS_VERSION@'
template_marker='@TAILAPPS_'VERSION'@'
case "$tailapps_version" in
  "$template_marker"|'')
    echo 'upgrade.sh must be downloaded from a Tailapps release asset' >&2
    exit 64
    ;;
esac

release_root=${TAILAPPS_RELEASE_BASE_URL:-https://github.com/generalbusiness-ai/tailapps/releases}
install_root=${TAILAPPS_INSTALL_ROOT:-"$HOME/.local"}
tailapp_home=${TAILAPP_HOME:-"$install_root/share/tailapp"}
rollback=false

usage() {
  echo 'usage: upgrade.sh [--home ABSOLUTE_PATH] [--rollback]' >&2
  exit 64
}
while [ "$#" -gt 0 ]; do
  case "$1" in
    --home) [ "$#" -ge 2 ] || usage; tailapp_home=$2; shift 2 ;;
    --rollback) rollback=true; shift ;;
    *) usage ;;
  esac
done
case "$install_root:$tailapp_home" in
  /*:/*) ;;
  *) echo 'install root and home must be absolute paths' >&2; exit 64 ;;
esac

bin_link="$install_root/bin/tailapp"
lib_dir="$install_root/lib/tailapp"
previous_link="$lib_dir/tailapp.previous"
[ -L "$bin_link" ] || { echo "expected managed symlink: $bin_link" >&2; exit 65; }
old_target=$(readlink "$bin_link")
case "$old_target" in /*) ;; *) echo 'managed symlink target must be absolute' >&2; exit 65 ;; esac
[ -x "$old_target" ] || { echo "managed target is not executable: $old_target" >&2; exit 65; }

case "$(uname -s)" in
  Darwin)
    service_kind=launchd
    service_label=ai.generalbusiness.tailapp
    service_path="$HOME/Library/LaunchAgents/$service_label.plist"
    service_domain="gui/$(id -u)/$service_label"
    command -v launchctl >/dev/null 2>&1 || { echo 'launchctl is required' >&2; exit 69; }
    command -v plutil >/dev/null 2>&1 || { echo 'plutil is required' >&2; exit 69; }
    [ -f "$service_path" ] || { echo "missing LaunchAgent: $service_path" >&2; exit 66; }
    [ "$(plutil -extract ProgramArguments.0 raw "$service_path")" = "$bin_link" ] || { echo 'LaunchAgent does not use the managed binary link' >&2; exit 65; }
    [ "$(plutil -extract EnvironmentVariables.TAILAPP_HOME raw "$service_path")" = "$tailapp_home" ] || { echo 'LaunchAgent home differs; pass --home with the configured path' >&2; exit 65; }
    launchctl print "$service_domain" >/dev/null 2>&1 || { echo "LaunchAgent is not loaded: $service_domain" >&2; exit 69; }
    restart_service() { launchctl kickstart -k "$service_domain"; }
    status_command="launchctl print $service_domain"
    ;;
  Linux)
    service_kind=systemd-user
    service_path="${XDG_CONFIG_HOME:-"$HOME/.config"}/systemd/user/tailapp.service"
    command -v systemctl >/dev/null 2>&1 || { echo 'systemctl is required' >&2; exit 69; }
    systemctl --user show-environment >/dev/null 2>&1 || { echo 'systemd --user is not available; use a foreground supervisor' >&2; exit 69; }
    [ -f "$service_path" ] || { echo "missing systemd user service: $service_path" >&2; exit 66; }
    [ "$(sed -n 's|^ExecStart=||p' "$service_path")" = "$bin_link serve --otlp-http 127.0.0.1:4318" ] || { echo 'systemd unit does not use the managed binary link' >&2; exit 65; }
    [ "$(sed -n 's|^Environment=TAILAPP_HOME=||p' "$service_path")" = "$tailapp_home" ] || { echo 'systemd unit home differs; pass --home with the configured path' >&2; exit 65; }
    restart_service() { systemctl --user daemon-reload; systemctl --user restart tailapp.service; }
    status_command='systemctl --user status tailapp.service'
    ;;
  *) echo 'upgrade.sh supports macOS and Linux user services' >&2; exit 64 ;;
esac

wait_for_health() {
  attempt=1
  while [ "$attempt" -le 10 ]; do
    if TAILAPP_HOME="$tailapp_home" "$bin_link" health >/dev/null 2>&1; then return 0; fi
    sleep 1
    attempt=$((attempt + 1))
  done
  return 1
}
replace_link() {
  target=$1
  stage=$(mktemp -d "$install_root/bin/.tailapps-upgrade.XXXXXX")
  rmdir "$stage"
  ln -s "$target" "$stage"
  mv "$stage" "$bin_link"
}
report() {
  completed_action=$1
  status=$(TAILAPP_HOME="$tailapp_home" "$bin_link" health)
  if printf '%s' "$status" | grep -Eq '"ingestion_ready"[[:space:]]*:[[:space:]]*true'; then
    printf '%s\n' "{\"version\":\"$tailapps_version\",\"control_plane\":\"healthy\",\"ingestion_ready\":true,\"action\":\"$completed_action\",\"next\":\"$status_command\"}"
  else
    pending_action=$completed_action
    [ "$completed_action" != upgraded ] || pending_action=upgrade_pending
    printf '%s\n' "{\"version\":\"$tailapps_version\",\"control_plane\":\"healthy\",\"ingestion_ready\":false,\"action\":\"$pending_action\",\"next\":\"TAILAPP_HOME=$tailapp_home $bin_link apps status; follow docs/reference/cli.md#upgrading-an-existing-resident\"}"
  fi
}

if [ "$rollback" = true ]; then
  [ -L "$previous_link" ] || { echo "no known-good prior binary: $previous_link" >&2; exit 66; }
  rollback_target=$(readlink "$previous_link")
  [ -x "$rollback_target" ] || { echo "prior binary is not executable: $rollback_target" >&2; exit 65; }
  replace_link "$rollback_target"
  if restart_service && wait_for_health; then
    printf '%s\n' "{\"control_plane\":\"healthy\",\"action\":\"rolled_back\",\"next\":\"$status_command\"}"
    exit 0
  fi
  echo 'rollback service did not become healthy' >&2
  exit 1
fi

if [ "$old_target" = "$lib_dir/tailapp-$tailapps_version" ]; then
  if wait_for_health; then
    report up_to_date
    exit 0
  fi
  echo 'installed resident did not become healthy' >&2
  exit 1
fi

command -v curl >/dev/null 2>&1 || { echo 'curl is required' >&2; exit 69; }
command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 || { echo 'sha256sum or shasum is required' >&2; exit 69; }
mkdir -p "$lib_dir"
release_url="$release_root/download/v$tailapps_version"
checksums=''
signature=''
bundle=''
installer=''
installer_log=''
cleanup_upgrade_files() {
  for upgrade_temp in "$checksums" "$signature" "$bundle" "$installer" "$installer_log"; do
    [ -z "$upgrade_temp" ] || [ ! -e "$upgrade_temp" ] || unlink "$upgrade_temp"
  done
}
trap cleanup_upgrade_files 0
trap 'exit 1' HUP INT TERM
checksums=$(mktemp "$lib_dir/.tailapps-upgrade-checksums.XXXXXX")
signature=$(mktemp "$lib_dir/.tailapps-upgrade-signature.XXXXXX")
bundle=$(mktemp "$lib_dir/.tailapps-upgrade-bundle.XXXXXX")
installer=$(mktemp "$lib_dir/.tailapps-upgrade-installer.XXXXXX")
installer_log=$(mktemp "$lib_dir/.tailapps-upgrade-install.XXXXXX")
curl --fail --silent --show-error --location "$release_url/checksums.txt" --output "$checksums"
curl --fail --silent --show-error --location "$release_url/checksums.txt.sig" --output "$signature"
curl --fail --silent --show-error --location "$release_url/checksums.txt.bundle" --output "$bundle"
curl --fail --silent --show-error --location "$release_url/install.sh" --output "$installer"
expected=$(awk '$2 == "install.sh" {print $1; exit}' "$checksums")
actual=$(command -v sha256sum >/dev/null 2>&1 && sha256sum "$installer" | awk '{print $1}' || shasum -a 256 "$installer" | awk '{print $1}')
[ "$actual" = "$expected" ] || { echo 'release installer checksum mismatch' >&2; exit 65; }
if command -v cosign >/dev/null 2>&1; then
  cosign verify-blob --bundle "$bundle" --certificate-identity-regexp "^https://github.com/generalbusiness-ai/tailapps/.github/workflows/release.yml@refs/tags/v$tailapps_version$" --certificate-oidc-issuer https://token.actions.githubusercontent.com "$checksums"
else
  echo 'cosign is not installed; release installer checksum was verified but keyless signature was not' >&2
fi

if ! TAILAPPS_RELEASE_BASE_URL="$release_root" TAILAPPS_INSTALL_ROOT="$install_root" TAILAPP_HOME="$tailapp_home" sh "$installer" --bundles none --no-service >"$installer_log" 2>&1; then
  cat "$installer_log" >&2
  exit 1
fi
previous_stage=$(mktemp -d "$lib_dir/.tailapps-previous.XXXXXX")
rmdir "$previous_stage"
ln -s "$old_target" "$previous_stage"
mv "$previous_stage" "$previous_link"
if restart_service && wait_for_health; then
  report upgraded
  exit 0
fi
replace_link "$old_target"
if restart_service && wait_for_health; then
  echo 'new resident did not become healthy; restored the known-good prior binary' >&2
  exit 1
fi
echo 'new resident did not become healthy and rollback also failed' >&2
exit 1
