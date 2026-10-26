#!/usr/bin/env bash
# Sourced by the benchmark CI Bash steps. TeamCity supplies both VCS checkouts.
set -euo pipefail
benchmark_checkout="${BENCHMARK_CHECKOUT_DIR:-$PWD}"
benchmark_source="$benchmark_checkout/qodana-cli"
benchmark_project="$benchmark_checkout/project"
benchmark_state="$benchmark_project/.edict"
benchmark_output="$benchmark_checkout/benchmark-output"
benchmark_codex_home="$benchmark_output/codex-home"
benchmark_scratch="$benchmark_output/scratch"

# The container steps (Codex, Qodana CLI) have no JDK; the host steps that start the IDE get it from the agent.
tooling_path="$benchmark_output/tooling/node_modules/.bin:$benchmark_output/tooling/bin"
if [[ -n "${JDK_21_0:-${JAVA_HOME:-}}" ]]; then
  export JAVA_HOME="${JDK_21_0:-$JAVA_HOME}"
  tooling_path="$tooling_path:$JAVA_HOME/bin"
fi
export PATH="$tooling_path:$PATH"
export QODANA_DIST="${QODANA_DIST:-$benchmark_checkout/native-dist}"
export TMPDIR="$benchmark_scratch"
# Every Codex call uses the benchmark's Codex home, including the trust check of `qodana edict install`.
export CODEX_HOME="$benchmark_codex_home"
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

# The servers run in their own session (setsid), which TeamCity does not stop when a build ends, so a previous build on a
# reused agent may still hold the MCP port. Stop that build's servers by their pid files, then whatever still listens.
stop_leftover_servers() {
  local port="$1" own_group pid group attempt
  stop_servers
  own_group=$(ps -o pgid= -p "$$" | tr -d ' ')
  for pid in $(listening_pids "$port"); do
    group=$(ps -o pgid= -p "$pid" 2>/dev/null | tr -d ' ' || true)
    echo "Stopping leftover process $pid on port $port"
    # A group other than this build step's own is the leftover server's session: stop all of it, its IDE included.
    if [[ -n "$group" && "$group" != "$own_group" ]]; then
      kill -TERM -- "-$group" 2>/dev/null || true
    else
      kill -TERM "$pid" 2>/dev/null || true
    fi
  done
  for attempt in {1..20}; do
    [[ -z "$(listening_pids "$port")" ]] && return
    sleep 1
  done
  for pid in $(listening_pids "$port"); do kill -KILL "$pid" 2>/dev/null || true; done
}

listening_pids() {
  if command -v ss >/dev/null; then
    ss -ltnpH "sport = :$1" 2>/dev/null | grep -o 'pid=[0-9]*' | cut -d= -f2 | sort -u
  elif command -v lsof >/dev/null; then
    lsof -t -iTCP:"$1" -sTCP:LISTEN 2>/dev/null | sort -u
  fi
}
