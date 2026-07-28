#!/bin/sh
set -eu

base_url=${WORKCELL_BASE_URL:-https://workcell-137.pages.dev/downloads}
install_dir=${WORKCELL_INSTALL_DIR:-"$HOME/.local/bin"}

if ! command -v curl >/dev/null 2>&1; then
  echo "workcell installer: curl is required" >&2
  exit 1
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin|linux) ;;
  *)
    echo "workcell installer: unsupported operating system: $os" >&2
    exit 1
    ;;
esac

machine=$(uname -m)
case "$machine" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64) arch=amd64 ;;
  *)
    echo "workcell installer: unsupported architecture: $machine" >&2
    exit 1
    ;;
esac

asset="workcell-$os-$arch"
temporary_dir=$(mktemp -d)
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM

curl -fsSL "$base_url/$asset" -o "$temporary_dir/workcell"

mkdir -p "$install_dir"
install -m 0755 "$temporary_dir/workcell" "$install_dir/workcell"

echo "installed workcell to $install_dir/workcell"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) echo "add $install_dir to PATH to run workcell directly" ;;
esac
