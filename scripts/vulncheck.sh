#!/bin/sh
# Run the repository's pinned Go vulnerability analyzer. Keeping the version
# here makes local and CI scans use the same tool and advisory behavior.
set -eu

tool_version=v1.7.0
if [ "$#" -eq 0 ]; then
  set -- ./...
fi
exec go run "golang.org/x/vuln/cmd/govulncheck@$tool_version" "$@"
