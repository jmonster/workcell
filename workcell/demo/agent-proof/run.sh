#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
workcell_bin="${WORKCELL_BIN:-$repo_root/bin/workcell}"
demo_mode="${WORKCELL_DEMO_MODE:-compare}"

if [[ ! -x "$workcell_bin" ]]; then
  printf 'Workcell binary missing: %s\nRun: make build\n' "$workcell_bin" >&2
  exit 1
fi

cd "$repo_root"
demo_args=(
  --mode "$demo_mode"
  --model gpt-5.6-luna
  --reasoning medium
  --workcell "$workcell_bin"
)

exec go run ./demo/agent-proof "${demo_args[@]}" "$@"
