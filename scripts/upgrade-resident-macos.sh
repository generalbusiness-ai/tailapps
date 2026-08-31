#!/bin/sh
# Upgrade the per-user macOS Tailapp resident without touching TAILAPP_HOME.
#
# The LaunchAgent always executes ~/.local/bin/tailapp. This command builds a
# versioned binary under ~/.local/lib/tailapp, atomically repoints that link,
# restarts the agent, and restores the old link if its control socket does not
# become healthy. It intentionally never installs, updates, activates, or
# deletes a Tailapp definition or projection.
#
# Usage:
#   scripts/upgrade-resident-macos.sh [--source DIR] [--home DIR] [--label LABEL] [--dry-run]
#   scripts/upgrade-resident-macos.sh [--home DIR] [--label LABEL] --rollback
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source_root=$repo_root
tailapp_home=${TAILAPP_HOME:-"$HOME/.local/share/tailapp"}
label=ai.generalbusiness.tailapp
dry_run=false
rollback=false

usage() {
  echo "usage: $0 [--source DIR] [--home DIR] [--label LABEL] [--dry-run] [--rollback]" >&2
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
    --label)
      [ "$#" -ge 2 ] || usage
      label=$2
      shift 2
      ;;
    --dry-run)
      dry_run=true
      shift
      ;;
    --rollback)
      rollback=true
      shift
      ;;
    *)
      usage
      ;;
  esac
done

[ "$(uname -s)" = "Darwin" ] || {
  echo "this upgrade command is for macOS launchd; use the platform-specific service workflow" >&2
  exit 64
}

case "$tailapp_home" in
  /*) ;;
  *)
    echo "--home must be an absolute path" >&2
    exit 64
    ;;
esac

bin_dir="$HOME/.local/bin"
bin_link="$bin_dir/tailapp"
lib_dir="$HOME/.local/lib/tailapp"
previous_link="$lib_dir/tailapp.previous"
plist="$HOME/Library/LaunchAgents/$label.plist"
domain="gui/$(id -u)/$label"

[ -x "$source_root/scripts/build.sh" ] || {
  echo "Tailapp build script is missing from $source_root" >&2
  exit 66
}
[ -L "$bin_link" ] || {
  echo "$bin_link must be a symlink before upgrading; refusing to replace an untracked binary" >&2
  exit 65
}
[ -f "$plist" ] || {
  echo "LaunchAgent plist is missing: $plist" >&2
  exit 66
}
plist_binary=$(plutil -extract ProgramArguments.0 raw "$plist")
[ "$plist_binary" = "$bin_link" ] || {
  echo "LaunchAgent binary is $plist_binary, not $bin_link; refusing to restart an unexpected service" >&2
  exit 65
}
plist_home=$(plutil -extract EnvironmentVariables.TAILAPP_HOME raw "$plist")
[ "$plist_home" = "$tailapp_home" ] || {
  echo "LaunchAgent TAILAPP_HOME is $plist_home, not $tailapp_home; pass --home with the configured path" >&2
  exit 65
}
launchctl print "$domain" >/dev/null

old_target=$(readlink "$bin_link")
[ -n "$old_target" ] || {
  echo "cannot resolve current binary link: $bin_link" >&2
  exit 65
}
case "$old_target" in
  /*) ;;
  *)
    echo "current binary link must use an absolute target: $old_target" >&2
    exit 65
    ;;
esac
[ -x "$old_target" ] || {
  echo "current binary target is not executable: $old_target" >&2
  exit 65
}

restart_and_check() {
  launchctl kickstart -k "$domain"
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

replace_link() {
  target=$1
  link_stage=$(mktemp -d "$bin_dir/.tailapp-link.XXXXXX")
  rmdir "$link_stage"
  ln -s "$target" "$link_stage"
  mv "$link_stage" "$bin_link"
}

if [ "$rollback" = true ]; then
  [ -L "$previous_link" ] || {
    echo "no previous Tailapp binary is recorded at $previous_link" >&2
    exit 66
  }
  rollback_target=$(readlink "$previous_link")
  [ -x "$rollback_target" ] || {
    echo "recorded previous binary is not executable: $rollback_target" >&2
    exit 65
  }
  if [ "$dry_run" = true ]; then
    printf '%s\n' "would repoint $bin_link to $rollback_target and restart $domain"
    exit 0
  fi
  replace_link "$rollback_target"
  if restart_and_check; then
    printf '%s\n' "rolled back Tailapp resident to $rollback_target"
    exit 0
  fi
  echo "rollback service did not become healthy; $bin_link remains at $rollback_target" >&2
  exit 1
fi

if [ "$dry_run" = true ]; then
  printf '%s\n' "would build $source_root into a new versioned binary under $lib_dir"
  printf '%s\n' "would preserve $old_target at $previous_link, repoint $bin_link, and restart $domain"
  printf '%s\n' "would leave $tailapp_home and all Tailapp definitions and projections untouched"
  exit 0
fi

mkdir -p "$lib_dir"
stage_dir=$(mktemp -d "$lib_dir/.upgrade.XXXXXX")
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

previous_stage=$(mktemp -d "$lib_dir/.tailapp-previous.XXXXXX")
rmdir "$previous_stage"
ln -s "$old_target" "$previous_stage"
mv "$previous_stage" "$previous_link"
replace_link "$installed_binary"

if restart_and_check; then
  printf '%s\n' "upgraded Tailapp resident to $installed_binary"
  printf '%s\n' "rollback with: $0 --home $tailapp_home --label $label --rollback"
  exit 0
fi

replace_link "$old_target"
if restart_and_check; then
  echo "new resident did not become healthy; restored previous binary $old_target" >&2
  exit 1
fi
echo "new resident did not become healthy and automatic rollback also failed; $bin_link points to $old_target" >&2
exit 1
