#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
session_name="workcell-multi-$$"
state_dir="$(mktemp -d)"

cleanup() {
  tmux kill-session -t "$session_name" 2>/dev/null || true
  rm -rf "$state_dir"
}
trap cleanup EXIT

tmux new-session -d -s "$session_name" -x 156 -y 42 "/bin/bash --noprofile --norc"
pane_a="$(tmux display-message -p -t "$session_name:0.0" '#{pane_id}')"
pane_b="$(tmux split-window -h -t "$pane_a" -P -F '#{pane_id}' "/bin/bash --noprofile --norc")"
pane_c="$(tmux split-window -v -t "$pane_a" -P -F '#{pane_id}' "/bin/bash --noprofile --norc")"
pane_d="$(tmux split-window -v -t "$pane_b" -P -F '#{pane_id}' "/bin/bash --noprofile --norc")"
tmux select-layout -t "$session_name:0" tiled >/dev/null
tmux set-option -t "$session_name" pane-border-status top
tmux set-option -t "$session_name" pane-border-format ' #{pane_title} '
tmux set-option -t "$session_name" status-style 'bg=#AF8DC3,fg=#090909'
tmux set-option -t "$session_name" pane-active-border-style 'fg=#996FB3'
tmux set-option -t "$session_name" pane-border-style 'fg=#55584F'

tmux select-pane -t "$pane_a" -T 'AGENT A / SHARED GPU'
tmux select-pane -t "$pane_b" -T 'AGENT B / SHARED GPU'
tmux select-pane -t "$pane_c" -T 'AGENT C / SHARED GPU'
tmux select-pane -t "$pane_d" -T 'AGENT D / FIRMWARE RIG'

for pane_id in "$pane_a" "$pane_b" "$pane_c" "$pane_d"; do
  tmux send-keys -t "$pane_id" "cd '$repo_root'; export PS1='> ' PATH='$repo_root/bin':\$PATH WORKCELL_STATE_DIR='$state_dir'; clear" Enter
done

tmux send-keys -t "$pane_a" "WORKCELL_SESSION=agent-a workcell run shared-gpu -- sh -c 'echo training; sleep 6; echo training-done'" Enter

(
  sleep 1
  tmux send-keys -t "$pane_b" "WORKCELL_SESSION=agent-b workcell run shared-gpu --wait -- sh -c 'echo evaluation; sleep 2; echo evaluation-done'" Enter
  sleep 1
  tmux send-keys -t "$pane_c" "WORKCELL_SESSION=agent-c workcell run shared-gpu --wait -- sh -c 'echo benchmark; sleep 1; echo benchmark-done'" Enter
  sleep 1
  tmux send-keys -t "$pane_d" "WORKCELL_SESSION=agent-d workcell run rig:firmware-hil -- sh -c 'echo power-cycle-test; sleep 3; echo power-cycle-done'" Enter
  sleep 9
  tmux send-keys -t "$pane_a" "workcell status shared-gpu" Enter
  tmux send-keys -t "$pane_d" "workcell status rig:firmware-hil" Enter
  sleep 8
  tmux detach-client -s "$session_name"
) &

tmux attach-session -t "$session_name"
