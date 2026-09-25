#!/usr/bin/env bash
set -euo pipefail

# All generation orchestration is Kotlin; the image already has Java and Node.
project_dir="$PWD/project"
output_dir="$PWD/benchmark-output"
# The assembled image contains Codex 0.117, predating the Kotlin host's permission profiles.
npm install --no-audit --no-fund --cache "$output_dir/npm-cache" --prefix "$output_dir/tooling" "@openai/codex@${BENCHMARK_CODEX_VERSION:-0.155.1}"
export PATH="$output_dir/tooling/node_modules/.bin:$PATH"
codex --version
exec java -jar "$PWD/benchmark-runtime/benchmark-runner.jar" \
  --project "$project_dir" --output "$output_dir" \
  --model "${BENCHMARK_MODEL:-gpt-5.6-sol}" --minutes "${BENCHMARK_MINUTES:-240}" \
  --limit "${BENCHMARK_LIMIT:-0}" --rules "${BENCHMARK_RULES:-}" \
  --preflight "${BENCHMARK_PREFLIGHT:-false}"
