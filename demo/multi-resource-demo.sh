#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workcell_bin="${WORKCELL_BIN:-$repo_root/bin/workcell}"
state_dir="$(mktemp -d)"
output_dir="$(mktemp -d)"

cleanup() {
  rm -rf "$state_dir" "$output_dir"
}
trap cleanup EXIT

export WORKCELL_STATE_DIR="$state_dir"

"$workcell_bin" run shared-gpu --session agent-a --json -- \
  sh -c 'echo training; sleep 3' > "$output_dir/agent-a.json" &
agent_a_pid=$!

for _ in $(seq 1 100); do
  if "$workcell_bin" status shared-gpu --json 2>/dev/null | grep -q '"decision":"owned"'; then
    break
  fi
  sleep 0.02
done

"$workcell_bin" run shared-gpu --session agent-b --wait --json -- \
  sh -c 'echo evaluation; sleep 2' > "$output_dir/agent-b.json" &
agent_b_pid=$!

sleep 0.1

"$workcell_bin" run shared-gpu --session agent-c --wait --json -- \
  sh -c 'echo benchmark; sleep 1' > "$output_dir/agent-c.json" &
agent_c_pid=$!

"$workcell_bin" run rig:firmware-hil --session agent-d --json -- \
  sh -c 'echo power-cycle-test; sleep 2' > "$output_dir/agent-d.json" &
agent_d_pid=$!

wait "$agent_a_pid"
wait "$agent_b_pid"
wait "$agent_c_pid"
wait "$agent_d_pid"

for agent in a b c d; do
  agent_label="$(printf '%s' "$agent" | tr '[:lower:]' '[:upper:]')"
  printf 'AGENT %s\n' "$agent_label"
  sed -n '1,20p' "$output_dir/agent-$agent.json"
done
