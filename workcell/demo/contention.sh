#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workcell_bin="${WORKCELL_BIN:-$repo_root/bin/workcell}"

exec go run "$repo_root/demo/contention.go" --workcell "$workcell_bin" "$@"
