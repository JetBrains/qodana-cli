#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
cd "$benchmark_project"
mkdir -p "$benchmark_output/trace"
# The shared MCP server sees every worker, including children absent from Codex's console.
# GNU tail drains the lifecycle log and exits when this build-step shell exits.
tasks_log="$(cat "$benchmark_output/edict-process-log")/edict-tasks.log"
test -f "$tasks_log"
tail --pid="$$" --sleep-interval=0.1 -n +1 -f "$tasks_log" &
# Paths come from edict_context, so the request names only the skill and the task.
env CODEX_HOME="$benchmark_codex_home" codex exec --dangerously-bypass-hook-trust \
  --skip-git-repo-check --output-last-message "$benchmark_output/trace/last-message.txt" \
  '$edict_manager process inbox and generate new rules'
echo 'Codex finished; native SARIF generation and comparison run in the next Gradle step.'
