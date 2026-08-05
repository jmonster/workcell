#!/bin/sh
set -eu

base_url=${STRINGPROOF_BASE_URL:-https://workcell-137.pages.dev/downloads}
install_dir=${STRINGPROOF_INSTALL_DIR:-"$HOME/.local/bin"}

for command_name in curl python3; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "stringproof installer: $command_name is required" >&2
    exit 1
  fi
done

if ! python3 -c 'import sys; raise SystemExit(sys.version_info < (3, 10))'; then
  echo "stringproof installer: Python 3.10 or newer is required" >&2
  exit 1
fi

temporary_dir=$(mktemp -d)
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM

curl -fsSL "$base_url/stringproof.pyz" -o "$temporary_dir/stringproof"

mkdir -p "$install_dir"
install -m 0755 "$temporary_dir/stringproof" "$install_dir/stringproof"

echo "installed stringproof to $install_dir/stringproof"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) echo "add $install_dir to PATH to run stringproof directly" ;;
esac
