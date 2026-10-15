#!/usr/bin/env bash
# Run on the target macOS machine. Keep the raw JSONL and logs for review.
set -euo pipefail
cd "$(dirname "$0")/.."

out="${STAGE3_OUT:-$PWD/stage3-results-$(date -u +%Y%m%dT%H%M%SZ)}"
mkdir -p "$out"
duration="${STAGE3_DURATION:-60s}"
warmup="${STAGE3_WARMUP:-10s}"
repeats="${STAGE3_REPEATS:-2}"
if [[ ! "$repeats" =~ ^[1-9][0-9]*$ ]]; then
  printf 'STAGE3_REPEATS must be a positive integer\n' >&2
  exit 2
fi
power="${STAGE3_POWER_PROFILE:-unspecified}"
display="${STAGE3_DISPLAY:-unspecified}"
os_version="$(sw_vers -productVersion)"
cpu="$(sysctl -n machdep.cpu.brand_string 2>/dev/null || sysctl -n hw.model)"
gpu="$(system_profiler SPDisplaysDataType 2>/dev/null | awk -F': ' '/Chipset Model:|Chip:/{if (!seen) print $2; seen=1}' || true)"
gpu="${gpu:-unknown}"
failure=0
printf 'run\texit_code\tlog\n' > "$out/run-status.tsv"

{
  date -u
  go version
  git rev-parse HEAD
  git status --short
  sw_vers
  system_profiler SPDisplaysDataType
  pmset -g ps
  printf 'power=%s\ndisplay=%s\n' "$power" "$display"
} > "$out/environment.txt" 2>&1 || true

if ! go test ./... -count=1 > "$out/go-test.txt" 2>&1; then
  failure=1
fi
if ! PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tools -p 'test_*.py' > "$out/python-test.txt" 2>&1; then
  failure=1
fi
if ! go build -o "$out/perftrace" ./cmd/perftrace > "$out/go-build.txt" 2>&1; then
  printf 'perftrace build failed; see %s\n' "$out/go-build.txt"
  exit 1
fi

cases=(
  'widgets 1000 warm'
  'widgets 10000 warm'
  'form 1000 warm'
  'virtual-form 10000 warm'
  'editor 100000 warm'
  'editor 100000 cold'
)
for case in "${cases[@]}"; do
  read -r scenario count pattern <<< "$case"
  for ((repeat=1; repeat<=repeats; repeat++)); do
    modes=(software hardware)
    if (( repeat % 2 == 0 )); then modes=(hardware software); fi
    for graphics in "${modes[@]}"; do
      name="${scenario}-${count}-${pattern}-run${repeat}-${graphics}"
      status=0
      "$out/perftrace" -scenario "$scenario" -count "$count" -pattern "$pattern" \
        -graphics "$graphics" -duration "$duration" -warmup "$warmup" \
        -output "$out/$name.jsonl" -os-version "$os_version" -cpu "$cpu" \
        -gpu "$gpu" -power "$power" -display "$display" > "$out/$name.log" 2>&1 || status=$?
      printf '%s\t%d\t%s\n' "$name" "$status" "$name.log" >> "$out/run-status.tsv"
      if (( status != 0 )); then failure=1; fi
    done
  done
done

status=0
NATIVEFORMS_METAL_FAULT=init "$out/perftrace" -scenario widgets -count 1000 \
    -graphics auto -warmup 0s -duration 8s -output "$out/fault-init.jsonl" \
    -os-version "$os_version" -cpu "$cpu" -gpu "$gpu" -power "$power" -display "$display" \
    > "$out/fault-init.log" 2>&1 || status=$?
printf 'fault-init\t%d\tfault-init.log\n' "$status" >> "$out/run-status.tsv"
if (( status != 0 )); then failure=1; fi
status=0
NATIVEFORMS_METAL_FAIL_AFTER=10 "$out/perftrace" -scenario widgets -count 1000 \
    -graphics auto -warmup 0s -duration 8s -output "$out/fault-render.jsonl" \
    -os-version "$os_version" -cpu "$cpu" -gpu "$gpu" -power "$power" -display "$display" \
    > "$out/fault-render.log" 2>&1 || status=$?
printf 'fault-render\t%d\tfault-render.log\n' "$status" >> "$out/run-status.tsv"
if (( status != 0 )); then failure=1; fi

python3 tools/summarize_perftrace.py "$out"/*.jsonl > "$out/summary.txt" 2>&1 || failure=1
for case in "${cases[@]}"; do
  read -r scenario count pattern <<< "$case"
  for ((repeat=1; repeat<=repeats; repeat++)); do
    prefix="${scenario}-${count}-${pattern}-run${repeat}"
    python3 tools/summarize_perftrace.py --compare \
      "$out/$prefix-software.jsonl" "$out/$prefix-hardware.jsonl" \
      >> "$out/summary.txt" 2>&1 || failure=1
  done
done
python3 tools/summarize_perftrace.py --check-fallback \
  "$out/fault-init.jsonl" "$out/fault-render.jsonl" \
  >> "$out/summary.txt" 2>&1 || failure=1
printf '%s\n' "$failure" > "$out/exit-status.txt"
rm "$out/perftrace"
tar -czf "$out.tar.gz" -C "$(dirname "$out")" "$(basename "$out")"
printf 'Share %s and, if visual differences appear, matching screenshots. Exit status: %s\n' "$out.tar.gz" "$failure"
exit "$failure"
