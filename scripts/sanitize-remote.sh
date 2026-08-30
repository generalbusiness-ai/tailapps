#!/bin/sh
# Sanitize a git remote URL into a bare public GitHub https URL.
#
# Accepts exactly the GitHub remote forms below, strips user-info,
# credentials, and any .git suffix, and prints https://github.com/OWNER/REPO.
# Anything else - other hosts, local paths, malformed URLs - prints nothing
# and exits 1: build provenance omits rather than fabricates. This is the
# producer half of the buildinfo contract; the binary independently
# re-validates the stamped shape at run time.
set -eu

remote="${1:-}"
owner_repo=""

case "$remote" in
  git@github.com:*)
    owner_repo="${remote#git@github.com:}"
    ;;
  ssh://git@github.com/*)
    owner_repo="${remote#ssh://git@github.com/}"
    ;;
  https://github.com/*)
    owner_repo="${remote#https://github.com/}"
    ;;
  https://*@github.com/*)
    owner_repo="${remote#https://*@github.com/}"
    ;;
  *)
    exit 1
    ;;
esac

owner_repo="${owner_repo%.git}"

# Exactly OWNER/REPO: one slash, both parts non-empty.
case "$owner_repo" in
  */*/*|*/|/*|"")
    exit 1
    ;;
  */*)
    ;;
  *)
    exit 1
    ;;
esac

owner="${owner_repo%%/*}"
repo="${owner_repo#*/}"

valid='^[A-Za-z0-9_.-][A-Za-z0-9_.-]*$'
if ! printf '%s\n' "$owner" | grep -q "$valid"; then exit 1; fi
if ! printf '%s\n' "$repo" | grep -q "$valid"; then exit 1; fi

printf 'https://github.com/%s/%s\n' "$owner" "$repo"
