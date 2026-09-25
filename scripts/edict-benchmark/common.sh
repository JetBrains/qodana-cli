#!/usr/bin/env bash
# Sourced by the three CI Bash steps. TeamCity supplies both VCS checkouts.
set -euo pipefail
benchmark_checkout="${BENCHMARK_CHECKOUT_DIR:-$PWD}"
benchmark_source="$benchmark_checkout/qodana-cli"
benchmark_project="$benchmark_checkout/project"
benchmark_output="$benchmark_checkout/benchmark-output"
benchmark_codex_home="$benchmark_output/codex-home"
benchmark_scratch="$benchmark_output/scratch"
export JAVA_HOME="${JDK_21_0:-${JAVA_HOME:?JDK 21 is required}}"
export PATH="$benchmark_output/tooling/node_modules/.bin:$benchmark_output/tooling/bin:$JAVA_HOME/bin:$PATH"
export QODANA_DIST="${QODANA_DIST:-$benchmark_checkout/native-dist}"
export TMPDIR="$benchmark_scratch"
mkdir -p "$benchmark_output/log" "$benchmark_scratch" "$benchmark_codex_home"

stop_servers() {
  local pid_file pid
  for pid_file in "$benchmark_output"/*.pid; do
    [[ -f "$pid_file" ]] || continue
    read -r pid < "$pid_file"
    [[ "$pid" =~ ^[0-9]+$ ]] || continue
    kill -TERM -- "-$pid" 2>/dev/null || true
  done
  for pid_file in "$benchmark_output"/*.pid; do
    [[ -f "$pid_file" ]] || continue
    read -r pid < "$pid_file"
    [[ "$pid" =~ ^[0-9]+$ ]] || continue
    for attempt in {1..20}; do
      kill -0 -- "-$pid" 2>/dev/null || break
      sleep 1
    done
    kill -KILL -- "-$pid" 2>/dev/null || true
    rm -f "$pid_file"
  done
}
