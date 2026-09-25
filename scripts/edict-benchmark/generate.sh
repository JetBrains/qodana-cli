#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
cd "$benchmark_project"
env CODEX_HOME="$benchmark_codex_home" codex exec --dangerously-bypass-hook-trust \
  --skip-git-repo-check --output-last-message "$benchmark_output/trace/last-message.txt" \
  "process inbox and generate new rules. Managed state: $benchmark_state. Private scratch: $benchmark_scratch"
echo 'Codex finished; native SARIF generation and comparison run in the next Gradle step.'
