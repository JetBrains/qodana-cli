#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
trap 'stop_servers' EXIT
trap 'exit 130' INT TERM
test -f "$benchmark_output/servers-ready"
printf '%s\n' 'process inbox and generate new rules' > "$benchmark_output/prompt.txt"
cd "$benchmark_project"
# Codex owns its native subagents and connects directly to both MCP servers.
# Keep raw traces private: managed MCP responses contain short-lived capabilities.
setsid timeout --signal=TERM --kill-after=30s "${BENCHMARK_MINUTES:-240}m" \
  env CODEX_HOME="$benchmark_codex_home" codex exec --dangerously-bypass-hook-trust \
  --json --skip-git-repo-check --output-last-message "$benchmark_output/trace/last-message.txt" \
  "$(cat "$benchmark_output/prompt.txt")" \
  > "$benchmark_output/trace/stdout.jsonl" 2> "$benchmark_output/trace/stderr.log" &
codex_pid=$!
echo "$codex_pid" > "$benchmark_output/codex.pid"
shopt -s nullglob
while kill -0 "$codex_pid" 2>/dev/null; do
  clusters=("$benchmark_state"/clusters/*/description.json)
  if ((${#clusters[@]})); then
    jq -rs 'group_by(.status) | map("\(.[0].status)=\(length)") | join(", ")' \
      "${clusters[@]}" | sed 's/^/Generation progress: /'
  else
    echo 'Generation progress: waiting for Edict to create clusters from the inbox'
  fi
  sleep 30
done
wait "$codex_pid"
rm "$benchmark_output/codex.pid"
echo 'Codex finished; native SARIF generation and comparison run in the next Gradle step.'
