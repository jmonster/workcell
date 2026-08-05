#!/usr/bin/env bash
set -euo pipefail

workcell_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repository_root="$(cd "$workcell_root/.." && pwd)"
output_dir="$repository_root/site/public/downloads"

mkdir -p "$output_dir"

targets=(
  "darwin arm64"
  "darwin amd64"
  "linux arm64"
  "linux amd64"
)

for target in "${targets[@]}"; do
  read -r target_os target_arch <<< "$target"
  output_path="$output_dir/workcell-$target_os-$target_arch"
  printf 'building %s/%s\n' "$target_os" "$target_arch"
  (
    cd "$workcell_root"
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" \
      go build -mod=readonly -buildvcs=false -trimpath -ldflags='-s -w' \
      -o "$output_path" ./cmd/workcell
  )
  chmod 0755 "$output_path"
done
