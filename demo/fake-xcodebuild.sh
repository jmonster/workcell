#!/usr/bin/env bash
set -euo pipefail

duration="${1:-4}"
step_delay="$(awk -v total="$duration" 'BEGIN { printf "%.3f", total / 4 }')"

printf '[1/4] Resolving packages\n'
sleep "$step_delay"
printf '[2/4] Building\n'
sleep "$step_delay"
printf '[3/4] Testing\n'
sleep "$step_delay"
printf '[4/4] Collecting results\n'
sleep "$step_delay"
