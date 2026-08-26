#!/bin/sh
set -eu

base_url=${PLAYBOOK_BASE_URL:-https://workcell-137.pages.dev/playbook}
codex_root=${CODEX_HOME:-"$HOME/.codex"}
skill_dir="$codex_root/skills/playbook"

if ! command -v curl >/dev/null 2>&1; then
  echo "playbook installer: curl is required" >&2
  exit 1
fi

temporary_dir=$(mktemp -d)
cleanup() {
  rm -rf -- "$temporary_dir"
}
trap cleanup 0 1 2 15

mkdir -p "$temporary_dir/agents"
curl -fsSL "$base_url/llms.txt" -o "$temporary_dir/SKILL.md"
curl -fsSL "$base_url/openai.yaml" -o "$temporary_dir/agents/openai.yaml"

mkdir -p "$skill_dir/agents"
install -m 0644 "$temporary_dir/SKILL.md" "$skill_dir/SKILL.md"
install -m 0644 "$temporary_dir/agents/openai.yaml" "$skill_dir/agents/openai.yaml"

echo "installed playbook to $skill_dir"
