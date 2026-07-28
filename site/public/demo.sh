#!/bin/sh
set -eu

usage() {
  cat <<'EOF'
Run the Workcell real-agent proof:

  curl -fsSL https://workcell-137.pages.dev/demo.sh | sh

The demo asks whether to run the shared-resource validation with or without
Workcell. Each run starts three Codex agents.
EOF
}

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
  without|workcell)
    mode=$1
    ;;
  "")
    printf '\nThis starts 3 Luna-medium agents with your local Codex CLI.\n\n  1) Without Workcell\n  2) With Workcell\n\nChoose 1 or 2: ' >/dev/tty
    if ! IFS= read -r choice </dev/tty; then
      echo "workcell demo: an interactive terminal is required" >&2
      exit 1
    fi
    case "$choice" in
      1) mode=without ;;
      2) mode=workcell ;;
      *)
        echo "workcell demo: choose 1 or 2" >&2
        exit 2
        ;;
    esac
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

for command_name in git go codex; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "workcell demo: $command_name is required" >&2
    exit 1
  fi
done

temporary_dir=$(mktemp -d)
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM

echo "Cloning Workcell..." >&2
git clone --depth 1 --quiet https://github.com/jmonster/workcell.git "$temporary_dir/workcell"

cd "$temporary_dir/workcell"
go build -o bin/workcell ./cmd/workcell

echo "Starting three Codex agents in \"$mode\" mode..." >&2
WORKCELL_DEMO_MODE=$mode ./demo/agent-proof/run.sh </dev/tty
