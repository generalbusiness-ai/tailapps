#!/bin/sh
# Install a first Tailapps resident for the current user. The script builds a
# versioned binary, links ~/.local/bin/tailapp to it, starts a user service,
# and installs each built-in bundle only when it is missing.
#
# Usage:
#   scripts/setup-resident.sh [--source DIR] [--home DIR] [--otlp ADDR] [--dry-run]
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source_root=$repo_root
tailapp_home=${TAILAPP_HOME:-"$HOME/.local/share/tailapp"}
otlp_addr=127.0.0.1:4318
dry_run=false

usage() {
  echo "usage: $0 [--source DIR] [--home DIR] [--otlp ADDR] [--dry-run]" >&2
  exit 64
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --source)
      [ "$#" -ge 2 ] || usage
      source_root=$2
      shift 2
      ;;
    --home)
      [ "$#" -ge 2 ] || usage
      tailapp_home=$2
      shift 2
      ;;
    --otlp)
      [ "$#" -ge 2 ] || usage
      otlp_addr=$2
      shift 2
      ;;
    --dry-run)
      dry_run=true
      shift
      ;;
    *)
      usage
      ;;
  esac
done

case "$tailapp_home" in
  /*) ;;
  *)
    echo "--home must be an absolute path" >&2
    exit 64
    ;;
esac
case "$otlp_addr" in
  127.0.0.1:*|'[::1]':*) ;;
  *)
    echo "--otlp must use an explicit loopback address (127.0.0.1:PORT or [::1]:PORT)" >&2
    exit 64
    ;;
esac
for configuration_value in "$tailapp_home" "$otlp_addr"; do
  case "$configuration_value" in
    *'&'*|*'<'*|*'>'*|*[[:space:]]*)
      echo "service paths and endpoint may not contain whitespace or XML-special characters" >&2
      exit 64
      ;;
  esac
done

[ -x "$source_root/scripts/build.sh" ] || {
  echo "Tailapp build script is missing from $source_root" >&2
  exit 66
}

bin_dir="$HOME/.local/bin"
bin_link="$bin_dir/tailapp"
lib_dir="$HOME/.local/lib/tailapp"

replace_link() {
  target=$1
  link_stage=$(mktemp -d "$bin_dir/.tailapp-link.XXXXXX")
  rmdir "$link_stage"
  ln -s "$target" "$link_stage"
  mv "$link_stage" "$bin_link"
}

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

install_missing_bundles() {
  installed=$(TAILAPP_HOME="$tailapp_home" "$bin_link" apps list)
  for bundle in activity-stats agent-guard daily-review session-cost signal-counts; do
    case "$installed" in
      *"\"name\": \"$bundle\""*)
        printf '%s\n' "built-in bundle already installed: $bundle"
        ;;
      *)
        TAILAPP_HOME="$tailapp_home" "$bin_link" apps install --bundle "$bundle" \
          --idempotency-key "first-setup-$bundle-v1" "$bundle"
        ;;
    esac
  done
}

operating_system=$(uname -s)
case "$operating_system" in
  Darwin)
    service_kind=launchd
    service_path="$HOME/Library/LaunchAgents/ai.generalbusiness.tailapp.plist"
    service_domain="gui/$(id -u)/ai.generalbusiness.tailapp"
    ;;
  Linux)
    service_kind=systemd-user
    systemd_config=${XDG_CONFIG_HOME:-"$HOME/.config"}
    service_path="$systemd_config/systemd/user/tailapp.service"
    ;;
  *)
    echo "unsupported operating system: $operating_system" >&2
    exit 64
    ;;
esac

validate_existing_service() {
  case "$service_kind" in
    launchd)
      [ -f "$service_path" ] || return 0
      existing_binary=$(plutil -extract ProgramArguments.0 raw "$service_path")
      existing_home=$(plutil -extract EnvironmentVariables.TAILAPP_HOME raw "$service_path")
      existing_address=$(plutil -extract ProgramArguments.3 raw "$service_path")
      [ "$existing_binary" = "$bin_link" ] && [ "$existing_home" = "$tailapp_home" ] && [ "$existing_address" = "$otlp_addr" ] || {
        echo "existing LaunchAgent does not match the requested binary, home, and receiver; refusing to overwrite it" >&2
        exit 65
      }
      ;;
    systemd-user)
      [ -f "$service_path" ] || return 0
      existing_exec=$(sed -n 's|^ExecStart=||p' "$service_path")
      existing_home=$(sed -n 's|^Environment=TAILAPP_HOME=||p' "$service_path")
      [ "$existing_exec" = "$bin_link serve --otlp-http $otlp_addr" ] && [ "$existing_home" = "$tailapp_home" ] || {
        echo "existing systemd user service does not match the requested binary, home, and receiver; refusing to overwrite it" >&2
        exit 65
      }
      ;;
  esac
}

validate_existing_service

if [ "$dry_run" = true ]; then
  printf '%s\n' "would build $source_root into a versioned binary under $lib_dir"
  printf '%s\n' "would atomically link $bin_link to that binary"
  printf '%s\n' "would create or validate $service_path and start its $service_kind user service"
  printf '%s\n' "would initialize $tailapp_home and install any missing built-in bundles"
  exit 0
fi

mkdir -p "$bin_dir" "$lib_dir"
stage_dir=$(mktemp -d "$lib_dir/.setup.XXXXXX")
new_binary="$stage_dir/tailapp"
"$source_root/scripts/build.sh" "$new_binary"
"$new_binary" --help >/dev/null
build_id=$(date -u +%Y%m%dT%H%M%SZ)-$(git -C "$source_root" rev-parse --short=12 HEAD)
installed_binary="$lib_dir/tailapp-$build_id"
[ ! -e "$installed_binary" ] || {
  echo "refusing to overwrite existing versioned binary: $installed_binary" >&2
  exit 65
}
mv "$new_binary" "$installed_binary"
rmdir "$stage_dir"
replace_link "$installed_binary"

TAILAPP_HOME="$tailapp_home" "$bin_link" init

case "$service_kind" in
  launchd)
    mkdir -p "$(dirname -- "$service_path")"
    if [ ! -f "$service_path" ]; then
      plist_stage=$(mktemp "$HOME/Library/LaunchAgents/.tailapp.XXXXXX")
      cat >"$plist_stage" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>ai.generalbusiness.tailapp</string>
  <key>ProgramArguments</key><array>
    <string>$bin_link</string><string>serve</string><string>--otlp-http</string><string>$otlp_addr</string>
  </array>
  <key>EnvironmentVariables</key><dict><key>TAILAPP_HOME</key><string>$tailapp_home</string></dict>
  <key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>ThrottleInterval</key><integer>10</integer>
  <key>ProcessType</key><string>Background</string>
  <key>StandardOutPath</key><string>$HOME/.local/state/tailapp/resident.log</string>
  <key>StandardErrorPath</key><string>$HOME/.local/state/tailapp/resident.err.log</string>
</dict></plist>
EOF
      plutil -lint "$plist_stage" >/dev/null
      mv "$plist_stage" "$service_path"
    fi
    mkdir -p "$HOME/.local/state/tailapp"
    if launchctl print "$service_domain" >/dev/null 2>&1; then
      launchctl kickstart -k "$service_domain"
    else
      launchctl bootstrap "gui/$(id -u)" "$service_path"
    fi
    ;;
  systemd-user)
    mkdir -p "$(dirname -- "$service_path")" "$HOME/.local/state/tailapp"
    if [ ! -f "$service_path" ]; then
      unit_stage=$(mktemp "$(dirname -- "$service_path")/.tailapp.XXXXXX")
      cat >"$unit_stage" <<EOF
[Unit]
Description=Tailapp local telemetry resident

[Service]
Type=simple
Environment=TAILAPP_HOME=$tailapp_home
ExecStart=$bin_link serve --otlp-http $otlp_addr
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
      loginctl enable-linger "$USER" || {
        echo "the systemd user service is running, but loginctl could not enable lingering; ask the host administrator to permit persistent user services" >&2
        exit 1
      }
    fi
    ;;
esac

if ! wait_for_health; then
  echo "resident did not become healthy; inspect the user-service logs and keep the installed binary link for diagnosis" >&2
  exit 1
fi
install_missing_bundles
TAILAPP_HOME="$tailapp_home" "$bin_link" health
TAILAPP_HOME="$tailapp_home" "$bin_link" apps list
