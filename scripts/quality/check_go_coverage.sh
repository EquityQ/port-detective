#!/usr/bin/env bash
set -euo pipefail

report_dir="${COVERAGE_OUTPUT_DIR:-coverage/go}"
mkdir -p "$report_dir"

printf '| Package | Coverage | Minimum |\n| --- | ---: | ---: |\n' > "$report_dir/summary.md"

while IFS=' ' read -r package minimum; do
  profile="$report_dir/$package.out"
  go test -coverprofile="$profile" "./internal/$package"

  actual="$(go tool cover -func="$profile" | awk '$1 == "total:" { gsub(/%/, "", $3); print $3 }')"
  if [[ -z "$actual" ]]; then
    echo "Could not read coverage for $package" >&2
    exit 1
  fi
  printf '%s: %s%% (minimum %s%%)\n' "$package" "$actual" "$minimum"
  awk -v actual="$actual" -v minimum="$minimum" 'BEGIN { exit !(actual + 0 >= minimum + 0) }' || {
    echo "$package coverage is below the $minimum% minimum" >&2
    exit 1
  }

  printf '| %s | %s%% | %s%% |\n' "$package" "$actual" "$minimum" >> "$report_dir/summary.md"
  go tool cover -html="$profile" -o "$report_dir/$package.html"
done <<'THRESHOLDS'
core 60
cli 45
mcpserver 70
fingerprint 70
THRESHOLDS
