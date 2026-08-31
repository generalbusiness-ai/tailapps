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
bundles='activity-stats,agent-guard,daily-review,session-cost,signal-counts'
interactive=false
no_service=false

usage() {
  echo "usage: install.sh [--bundles LIST|none] [--interactive] [--no-service]" >&2
  exit 64
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --bundles)
      [ "$#" -ge 2 ] || usage
      bundles=$2
      shift 2
      ;;
    --interactive)
      interactive=true
      shift
      ;;
    --no-service)
      no_service=true
      shift
      ;;
    *) usage ;;
  esac
done

case "$install_root" in
  /*) ;;
  *)
    echo 'TAILAPPS_INSTALL_ROOT must be an absolute path' >&2
    exit 64
    ;;
esac
tailapp_home=${TAILAPP_HOME:-"$install_root/share/tailapp"}
case "$tailapp_home" in
  /*) ;;
  *)
    echo 'TAILAPP_HOME must be an absolute path' >&2
    exit 64
    ;;
esac
release_url="$release_root/download/v$tailapps_version"

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

wait_for_health() {
  attempt=1
  while [ "$attempt" -le 10 ]; do
    if TAILAPP_HOME="$tailapp_home" "$bin_link" health >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
    attempt=$((attempt + 1))
  done
  return 1
}

install_requested_bundles() {
  if [ "$interactive" = true ]; then
    TAILAPP_HOME="$tailapp_home" "$bin_link" setup --bundles "$bundles" --interactive
  else
    TAILAPP_HOME="$tailapp_home" "$bin_link" setup --bundles "$bundles"
  fi
}

state_dir="$install_root/state/tailapp"
service_label='ai.generalbusiness.tailapp'
service_kind=''
service_path=''

configure_user_service() {
  case "$release_os" in
    darwin)
      command -v launchctl >/dev/null 2>&1 || return 1
      command -v plutil >/dev/null 2>&1 || return 1
      service_kind=launchd
      service_path="$HOME/Library/LaunchAgents/$service_label.plist"
      service_domain="gui/$(id -u)/$service_label"
      mkdir -p "$(dirname -- "$service_path")" "$state_dir"
      if [ -f "$service_path" ]; then
        existing_binary=$(plutil -extract ProgramArguments.0 raw "$service_path")
        existing_home=$(plutil -extract EnvironmentVariables.TAILAPP_HOME raw "$service_path")
        existing_address=$(plutil -extract ProgramArguments.3 raw "$service_path")
        [ "$existing_binary" = "$bin_link" ] && [ "$existing_home" = "$tailapp_home" ] && [ "$existing_address" = '127.0.0.1:4318' ] || {
          echo "existing LaunchAgent does not match this Tailapps install; refusing to overwrite it" >&2
          return 1
        }
      else
        plist_stage=$(mktemp "$HOME/Library/LaunchAgents/.tailapps.XXXXXX")
        cat >"$plist_stage" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>$service_label</string>
  <key>ProgramArguments</key><array>
    <string>$bin_link</string><string>serve</string><string>--otlp-http</string><string>127.0.0.1:4318</string>
  </array>
  <key>EnvironmentVariables</key><dict><key>TAILAPP_HOME</key><string>$tailapp_home</string></dict>
  <key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>ThrottleInterval</key><integer>10</integer>
  <key>ProcessType</key><string>Background</string>
  <key>StandardOutPath</key><string>$state_dir/resident.log</string>
  <key>StandardErrorPath</key><string>$state_dir/resident.err.log</string>
</dict></plist>
EOF
        plutil -lint "$plist_stage" >/dev/null
        mv "$plist_stage" "$service_path"
      fi
      if launchctl print "$service_domain" >/dev/null 2>&1; then
        launchctl kickstart -k "$service_domain"
      else
        launchctl bootstrap "gui/$(id -u)" "$service_path"
      fi
      ;;
    linux)
      command -v systemctl >/dev/null 2>&1 || return 1
      systemctl --user show-environment >/dev/null 2>&1 || return 1
      service_kind=systemd-user
      service_path="${XDG_CONFIG_HOME:-"$HOME/.config"}/systemd/user/tailapp.service"
      mkdir -p "$(dirname -- "$service_path")" "$state_dir"
      if [ -f "$service_path" ]; then
        existing_exec=$(sed -n 's|^ExecStart=||p' "$service_path")
        existing_home=$(sed -n 's|^Environment=TAILAPP_HOME=||p' "$service_path")
        [ "$existing_exec" = "$bin_link serve --otlp-http 127.0.0.1:4318" ] && [ "$existing_home" = "$tailapp_home" ] || {
          echo "existing systemd user service does not match this Tailapps install; refusing to overwrite it" >&2
          return 1
        }
      else
        unit_stage=$(mktemp "$(dirname -- "$service_path")/.tailapps.XXXXXX")
        cat >"$unit_stage" <<EOF
[Unit]
Description=Tailapps local telemetry resident

[Service]
Type=simple
Environment=TAILAPP_HOME=$tailapp_home
ExecStart=$bin_link serve --otlp-http 127.0.0.1:4318
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF
        mv "$unit_stage" "$service_path"
      fi
      systemctl --user daemon-reload
      systemctl --user enable --now tailapp.service
      if command -v loginctl >/dev/null 2>&1; then
        login_user=${USER:-"$(id -un)"}
        loginctl enable-linger "$login_user" || {
          echo "Tailapps is running, but it will stop at logout until an administrator allows: loginctl enable-linger $login_user" >&2
        }
      else
        echo 'Tailapps is running, but lingering could not be checked; it may stop at logout. Ask an administrator to enable user-service lingering.' >&2
      fi
      ;;
  esac
}

temporary_pid=''
stop_temporary_resident() {
  if [ -n "$temporary_pid" ]; then
    kill "$temporary_pid" >/dev/null 2>&1 || true
    wait "$temporary_pid" 2>/dev/null || true
    temporary_pid=''
  fi
}
start_temporary_resident() {
  TAILAPP_HOME="$tailapp_home" "$bin_link" serve --otlp-http 127.0.0.1:0 >"$state_dir/temporary-resident.log" 2>"$state_dir/temporary-resident.err.log" &
  temporary_pid=$!
  wait_for_health
}

TAILAPP_HOME="$tailapp_home" "$bin_link" init >/dev/null
mkdir -p "$state_dir"
persistent=false
if [ "$no_service" = false ] && configure_user_service && wait_for_health; then
  persistent=true
fi

if [ "$persistent" = true ]; then
  install_requested_bundles
  unlink "$checksums"
  unlink "$archive"
  unlink "$signature"
  unlink "$bundle"
  printf '%s\n' "installed Tailapps $tailapps_version at $installed_binary"
  printf '%s\n' "resident: $service_kind ($service_path)"
  case "$service_kind" in
    launchd) printf '%s\n' "status: launchctl print gui/$(id -u)/$service_label; logs: $state_dir/resident.err.log" ;;
    systemd-user) printf '%s\n' 'status: systemctl --user status tailapp.service; logs: journalctl --user -u tailapp.service' ;;
  esac
  exit 0
fi

if ! start_temporary_resident; then
  stop_temporary_resident
  echo "could not start a temporary resident for built-in setup; run: TAILAPP_HOME=$tailapp_home $bin_link serve --otlp-http 127.0.0.1:4318" >&2
  exit 1
fi
if ! install_requested_bundles; then
  stop_temporary_resident
  exit 1
fi
stop_temporary_resident
unlink "$checksums"
unlink "$archive"
unlink "$signature"
unlink "$bundle"
printf '%s\n' "installed Tailapps $tailapps_version at $installed_binary"
printf '%s\n' "bundles were installed at $tailapp_home"
printf '%s\n' "foreground remedy: TAILAPP_HOME=$tailapp_home $bin_link serve --otlp-http 127.0.0.1:4318"
if [ "$no_service" = true ]; then
  printf '%s\n' 'no user service was requested; start the foreground remedy above or configure a supervisor explicitly.'
  exit 0
fi
echo 'persistent setup is incomplete because no usable user service manager was available; the binary and requested bundles were retained.' >&2
exit 1
