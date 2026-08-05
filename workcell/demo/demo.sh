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

WORKCELL_SESSION=ace/audio-fix \
  "$workcell_bin" run macos-xcode -- "$repo_root/demo/fake-xcodebuild.sh" 6 \
  > "$output_dir/agent-a.txt" 2>&1 &
agent_a_pid=$!

for _ in $(seq 1 100); do
  if grep -q '^ACQUIRED' "$output_dir/agent-a.txt" 2>/dev/null; then
    break
  fi
  sleep 0.02
done

set +e
WORKCELL_SESSION=ace/player-redesign \
  "$workcell_bin" run macos-xcode -- "$repo_root/demo/fake-xcodebuild.sh" 2 \
  > "$output_dir/agent-b-busy.txt" 2>&1
busy_exit_code=$?
set -e

WORKCELL_SESSION=ace/player-redesign \
  "$workcell_bin" run macos-xcode --wait -- "$repo_root/demo/fake-xcodebuild.sh" 2 \
  > "$output_dir/agent-b-wait.txt" 2>&1 &
agent_b_pid=$!

wait "$agent_a_pid"
wait "$agent_b_pid"

printf 'AGENT A\n'
sed -n '1,80p' "$output_dir/agent-a.txt"
printf '\nAGENT B — FIRST ATTEMPT (exit %s)\n' "$busy_exit_code"
sed -n '1,80p' "$output_dir/agent-b-busy.txt"
printf '\nAGENT B — AUTOMATIC HANDOFF\n'
sed -n '1,80p' "$output_dir/agent-b-wait.txt"
